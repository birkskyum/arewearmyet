package guided

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
	"github.com/birkskyum/arewearmyet/app/internal/tree"
)

// Guide is where an agent reads how to work on these machines.
const Guide = "https://arewearmyet.com/agents.md"

// Briefing is what goes into a brief: the computer, what the tests found on it, and what the
// site knows about each check.
type Briefing struct {
	Version string
	Run     *report.Run
	// Kept is the machine's latest result per check.
	Kept  map[string]report.Kept
	Model report.Model
	// Checks is the site's list, with where to look when a check fails. It may be empty when
	// the site couldn't be reached.
	Checks map[string]report.Check
	// Family is the family the machine names itself as, for where the site doesn't know it.
	Family string
	// TreeSource is the name of this machine's device tree source, when it is installed.
	TreeSource string
	// Findings are what is written up on the site about this model, by check.
	Findings map[string]report.Finding
	// Notes are what other owners and their agents wrote down about this model.
	Notes []report.Listed
	// Only names the checks to work on. Empty means everything that fails.
	Only []string
	When time.Time
}

// Fill adds what the machine and the site know beyond the results: the family, the device
// tree's source, what is written up about the model and the notes left on it. Whatever can't
// be had is left out.
func (b *Briefing) Fill(api string) {
	b.Family = probe.Family(machine.Real())
	if source, err := tree.Real().Machine(); err == nil {
		b.TreeSource = source
	}
	if b.Model.ID != "" {
		b.Notes, _ = report.Notes(api, b.Model.ID)
		b.Findings, _ = report.Findings(api, b.Model.ID)
	}
}

// ChecksFor turns words into checks: a check's own id, or a part of the test, which stands
// for the checks it tests.
func ChecksFor(words []string) (checks []string, unknown []string) {
	seen := map[string]bool{}
	add := func(check string) {
		if !seen[check] {
			seen[check] = true
			checks = append(checks, check)
		}
	}
	for _, word := range words {
		if _, ok := checkTitle[word]; ok {
			add(word)
			continue
		}
		found := false
		for _, entry := range menu {
			if entry.word == word && len(entry.checks) > 0 {
				found = true
				for _, check := range entry.checks {
					add(check)
				}
			}
		}
		if !found {
			unknown = append(unknown, word)
		}
	}
	return checks, unknown
}

// remeasure is the command that tests a check again, and whether that takes the owner.
func remeasure(check string) (command string, owner bool) {
	for _, entry := range menu {
		for _, tested := range entry.checks {
			if tested == check {
				return "hw-test " + entry.word, !entry.auto || entry.word == "speakers"
			}
		}
	}
	return "hw-test quick --json", false
}

// Failing lists the checks that are partly working or broken in the machine's latest results.
func Failing(kept map[string]report.Kept) []string {
	var failing []string
	for check, result := range kept {
		if severity[result.State] > 1 {
			failing = append(failing, check)
		}
	}
	sort.Slice(failing, func(a, b int) bool {
		if severity[kept[failing[a]].State] != severity[kept[failing[b]].State] {
			return severity[kept[failing[a]].State] > severity[kept[failing[b]].State]
		}
		return failing[a] < failing[b]
	})
	return failing
}

var howSaid = map[string]string{report.Measured: "measured by the test", report.Owner: "seen by the owner", "agent": "read by an agent"}

// Brief writes what an agent needs to pick a failing part up: the computer, what was
// measured and what is already known of each part, and the order to go about it in. The
// guide has the rules and the how-to in full, so the brief only names them. It holds no
// serial number, address, user or host.
func Brief(b Briefing) string {
	var out strings.Builder
	say := func(format string, args ...any) { fmt.Fprintf(&out, format+"\n", args...) }
	hardware := b.Run.Hardware
	name := strings.TrimSpace(hardware["maker"] + " " + first(hardware["family"], hardware["product"]))
	work := b.Only
	if len(work) == 0 {
		work = Failing(b.Kept)
	}
	family := first(b.Model.Family, b.Family)

	say("# Hardware brief: %s", name)
	say("")
	say("Written by hw-test %s on %s. The owner of this computer asked for help getting the parts below to work under Omarchy.", b.Version, b.When.Format("2 January 2006"))
	say("")
	say("The guide for this work is %s. Its steps 1 to 4 are done for you: the computer is identified and measured. **Read its Words and Rules, then step 5 (Fix it) and step 6 (Notes), before you change anything.** This file has what is particular to this computer.", Guide)
	say("")
	say("## This computer")
	say("")
	model := strings.TrimSpace(hardware["maker"] + " " + hardware["product"])
	if text := hardware["family"]; text != "" {
		model += " (" + text + ")"
	}
	say("- Model: %s", model)
	for _, field := range [][2]string{{"sku", "SKU"}, {"board", "Mainboard"}, {"bios", "BIOS"}} {
		if value := hardware[field[0]]; value != "" {
			say("- %s: %s", field[1], value)
		}
	}
	switch {
	case b.Model.ID != "":
		say("- On arewearmyet.com as `%s`: %s", b.Model.ID, first(b.Model.Page, report.Site+"/models/"+b.Model.ID))
		say("- Everything known about this model, with its device tree and firmware: `curl -s %s/api/models/%s`", report.Site, b.Model.ID)
	default:
		say("- arewearmyet.com doesn't know this computer yet. `hw-test` offers a form to add it after sending a result.")
	}
	if family != "" {
		say("- Family, which is the first line of a pull request or an issue for it: `%s`", family)
	}
	if b.Run.Compatible != "" {
		line := fmt.Sprintf("- Device tree in use: `%s`", b.Run.Compatible)
		if b.TreeSource != "" {
			line += fmt.Sprintf(", built from `arch/arm64/boot/dts/qcom/%s.dts`", b.TreeSource)
		}
		say("%s", line)
	}
	say("- Kernel: %s, on %s", b.Run.Kernel, b.Run.OS)
	var packages []string
	for name, version := range b.Run.Packages {
		packages = append(packages, name+" "+version)
	}
	sort.Strings(packages)
	if len(packages) > 0 {
		say("- Packages: %s", strings.Join(packages, ", "))
	}
	// Whether the computer is up to date decides whether a fault is still worth chasing
	if len(b.Model.Kernels) > 0 {
		var channels []string
		for _, channel := range []string{"stable", "rc", "edge"} {
			if version := b.Model.Kernels[channel]; version != "" {
				channels = append(channels, channel+" "+version)
			}
		}
		say("- Omarchy's channels have linux-omarchy at: %s. This computer has %s. If it is behind its channel, ask the owner to run `omarchy update`, restart and run `hw-test fix` again before you debug: the fault may be fixed already.", strings.Join(channels, ", "), first(b.Run.Packages["linux-omarchy"], "an unknown version"))
	}

	say("")
	say("## What to work on")
	if len(work) == 0 {
		say("")
		say("Nothing fails in this computer's latest results. Run `hw-test` to test what hasn't been tested yet.")
	}
	for _, check := range work {
		known := b.Checks[check]
		say("")
		result, tested := b.Kept[check]
		state := "not tested yet"
		if tested {
			state = stateLabel[result.State]
		}
		say("### %s (check `%s`): %s", first(checkTitle[check], known.Title, check), check, state)
		say("")
		if tested {
			say("- Measured on %s with kernel %s, %s.", result.When.Format("2 January 2006"), first(result.Kernel, "unknown"), first(howSaid[result.How], result.How))
			if result.Evidence != "" {
				say("- What the test found: %s", result.Evidence)
			}
			if result.Note != "" {
				say("- Noted: %s", result.Note)
			}
		}
		// What is written up on the site about this part, which often names the cause
		if finding, ok := b.Findings[check]; ok && finding.Note != "" {
			line := fmt.Sprintf("- Already known on the site (%s", stateLabel[finding.State])
			if finding.Checked != "" {
				line += ", " + finding.Checked
			}
			line += "): " + finding.Note
			if finding.Source != "" {
				line += " Source: " + finding.Source
			}
			say("%s", line)
		}
		if known.Works != "" {
			say("- Working means: %s", known.Works)
		}
		if known.Debug != "" {
			say("- Where to look: %s", known.Debug)
		}
		command, owner := remeasure(check)
		if owner {
			say("- Measure again: `%s`, run by the owner in their own terminal, since it needs them at the computer. Ask them to run it and tell you what it said.", command)
		} else {
			say("- Measure again: `%s --no-send`, which runs by itself, also without a terminal.", strings.TrimSuffix(command, " --json"))
		}
	}

	// Where everything else stands, since one part's fault is often another's
	working := map[string][]string{}
	covered := map[string]bool{}
	for _, check := range work {
		covered[check] = true
	}
	for check := range checkTitle {
		if covered[check] {
			continue
		}
		state := "not tested"
		if result, ok := b.Kept[check]; ok {
			state = stateLabel[result.State]
		}
		working[state] = append(working[state], check)
	}
	say("")
	say("## The rest of this computer")
	say("")
	for _, state := range []string{"broken", "partly", "works", "not on this model", "not tested"} {
		if checks := working[state]; len(checks) > 0 {
			sort.Strings(checks)
			say("- %s: %s", state, strings.Join(checks, ", "))
		}
	}
	say("")
	say("Several parts hang on one: sound, the battery, charging and USB-C all need the audio processor (`adsp`) running, and WebGL and WebGPU need the GPU. `hw-test status` shows all of it at any time.")

	say("")
	say("## Notes on this model")
	say("")
	if len(b.Notes) > 0 {
		for at, note := range b.Notes {
			if at == 12 {
				say("- and %d more: `curl -s '%s/api/notes?model=%s'`", len(b.Notes)-at, report.Site, b.Model.ID)
				break
			}
			about := note.Kind
			if note.Kind == "problem" && note.Status != "" {
				about += ", " + note.Status
			}
			if note.Check != "" {
				about += ", " + note.Check
			}
			line := fmt.Sprintf("- %s (%s): `curl -s %s`", note.Title, about, note.URL)
			if note.Replies > 0 {
				line += fmt.Sprintf(", %d answers", note.Replies)
			}
			say("%s", line)
		}
		say("")
		say("Read the ones that touch your work before you start: someone may have been here.")
	} else {
		say("Nobody has left a note on this model yet. That means nobody wrote one, not that nothing is known: see \"Already known on the site\" above, and search every model's notes with `curl -s '%s/api/notes?q=<words>'`.", report.Site)
	}

	say("")
	say("## How to go about it")
	say("")
	say("Each step is explained in the guide's step 5 and 6. In short:")
	say("")
	say("1. **Check it is worth chasing.** Up to date (above)? Already known, with a cause? Some parts can't be fixed from here: a missing kernel feature, or something Omarchy has no place for. Then say so to the owner, leave a note (step 8) and go on to the next part.")
	say("2. **Snapshot.** `hw-test verify before`, before you change anything.")
	say("3. **Look.** The commands under \"Where to look\" only read. `dtc -I fs -O dts /proc/device-tree` shows the device tree the computer is running.")
	say("4. **Change.** A setting: try it on this computer, keeping a copy of what you change. The device tree: `hw-test tree start`, edit, `hw-test tree build`, then the owner runs `hw-test tree boot`, which makes a boot entry of its own. Never install a rebuilt `linux-omarchy-dtbs`, write under `/boot/dtbs` or edit `/boot/limine.conf` by hand: each can stop the computer from starting.")
	say("5. **Before a restart, or a test that may freeze the computer:** write where you are into `PROGRESS.md`, then `hw-test resume \"<what to go on with>\"`. You are opened again at the owner's next login.")
	say("6. **Measure again.** The \"Measure again\" command of each part you touched, and a neighbouring part. Then `hw-test verify after`, which prints a before-and-after block.")
	say("7. **Pull request.** Open it as a draft. Its description starts with a line that says only %s, then the block. Then `hw-test verify after --pr <its address> --title \"<the change in a line>\"`, which records it on the model's page, and mark it ready. Tell the owner.", familyLine(family))
	say("8. **Not fixed?** Leave it for the next agent: `hw-test note --kind problem --check <check> --title \"...\" --body-file <file> --log kernel`. What the owner told you about a part goes in with `hw-test report --check <check> --state partial --note \"...\"`.")
	say("9. **Clean up.** `hw-test tree remove` (run by the owner), restore the settings you changed, and say what is left.")
	say("")
	say("The owner's data is the line: never their files, the disk's partitions or its encryption, and no firmware flashing or BIOS changes. Commands that need root go through `sudo`. When it asks for a password you can't give, hand the owner the exact command to run.")
	return out.String()
}

// familyLine is how the brief names the line a pull request opens with: the machine's
// family when it is known, and the choice of them when it isn't.
func familyLine(family string) string {
	switch family {
	case "":
		return "the computer's family (`" + probe.DragonMsft + "` for a Snapdragon machine that came with Windows, `" + probe.DragonGoogle + "` for a Chromebook or Googlebook, `" + probe.Nvidia + "` or `" + probe.Apple + "`)"
	case probe.Apple:
		return "`" + probe.Apple + "` (Apple Silicon Macs belong to the Omarchy Mac team, so ask the owner where that team takes changes before opening anything)"
	}
	return "`" + family + "`"
}

// Workspace is the folder the brief is written to and the agent starts in, where what it
// clones stays in view of the owner.
func Workspace() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "hw-bringup")
	}
	if info, err := os.Stat(filepath.Join(home, "Work")); err == nil && info.IsDir() {
		return filepath.Join(home, "Work", "hw-bringup")
	}
	return filepath.Join(home, "hw-bringup")
}

// Handed is how handing the failing parts to an agent went.
type Handed struct {
	// File is where the brief was written, and Said what to tell the owner.
	File, Said string
	// Opened is set when the agent's window opened.
	Opened bool
}

// HandOver writes the brief for the given checks, or for everything that fails, and opens
// the owner's agent on it. launch is what opens the agent, which tests replace.
func HandOver(b Briefing, dir string, launch func(dir, prompt string) error) Handed {
	work := b.Only
	if len(work) == 0 {
		work = Failing(b.Kept)
	}
	if len(work) == 0 {
		return Handed{Said: "Nothing fails in this computer's latest results, so there is nothing to hand over."}
	}
	file := filepath.Join(dir, "BRIEF.md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Handed{Said: "The brief couldn't be written: " + err.Error()}
	}
	if err := os.WriteFile(file, []byte(Brief(b)), 0o644); err != nil {
		return Handed{Said: "The brief couldn't be written: " + err.Error()}
	}
	var titles []string
	for _, check := range work {
		titles = append(titles, strings.ToLower(first(checkTitle[check], check)))
	}
	prompt := fmt.Sprintf("Read BRIEF.md in this folder and help me get this working on my computer: %s. It says what was measured, how to measure again with hw-test, where to look and where a fix goes. Start by reading the guide it points to, %s.", strings.Join(titles, ", "), Guide)
	err := launch(dir, prompt)
	switch {
	case err == nil:
		return Handed{File: file, Opened: true, Said: "Your agent opened in a window of its own, with the brief in " + file + "."}
	case errors.Is(err, tools.ErrNoAgent):
		return Handed{File: file, Said: "No agent is chosen on this computer yet. Pick one in the Omarchy menu under Setup, Default, Agent, or with omarchy default agent claude, and choose this again. The brief is in " + file + "."}
	case errors.Is(err, tools.ErrNotOmarchy):
		return Handed{File: file, Said: "The brief is in " + file + ". Give it to your agent: \"Read " + file + " and help me get this working.\""}
	}
	return Handed{File: file, Said: "The agent couldn't be opened (" + err.Error() + "). The brief is in " + file + "."}
}

// handedMsg carries how handing over went back to the screen that asked.
type handedMsg struct{ handed Handed }

// handOver hands the given checks to the agent without holding the screen up: the site is
// asked for its notes on the checks first, and the brief is written without them when it
// doesn't answer.
func handOver(run *report.Run, options Options, model report.Model, only []string) tea.Cmd {
	return func() tea.Msg {
		// What this round found counts as the machine's latest before the brief is written from it
		_ = report.Remember(run, time.Now())
		checks, _ := report.Checks(options.API)
		// The site says which model this is and which family its pull requests name
		if model.ID == "" || model.Family == "" {
			if matched, err := report.Match(options.API, run); err == nil && matched.ID != "" {
				model = matched
			}
		}
		launch := options.launch
		if launch == nil {
			launch = tools.Agent
		}
		briefing := Briefing{Version: options.Version, Run: run, Kept: report.Local(), Model: model, Checks: checks, Only: only, When: time.Now()}
		briefing.Fill(options.API)
		return handedMsg{HandOver(briefing, Workspace(), launch)}
	}
}
