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

	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
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
	// Notes are what other owners and their agents wrote down about this model.
	Notes []report.Listed
	// Only names the checks to work on. Empty means everything that fails.
	Only []string
	When time.Time
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
// measured and how to measure it again, where to look, and where a fix belongs. It names no
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

	say("# Hardware brief: %s", name)
	say("")
	say("Written by hw-test %s on %s. The owner of this computer asked for help getting the parts below to work under Omarchy.", b.Version, b.When.Format("2 January 2006"))
	say("")
	say("**Read %s before you do anything.** It has the rules (what you may not touch, what counts as measured, where fixes go) and the site's API. The end of this file repeats the ones that matter most.", Guide)
	say("")
	say("## This computer")
	say("")
	model := strings.TrimSpace(hardware["maker"] + " " + hardware["product"])
	if family := hardware["family"]; family != "" {
		model += " (" + family + ")"
	}
	say("- Model: %s", model)
	for _, field := range [][2]string{{"sku", "SKU"}, {"board", "Board"}, {"bios", "BIOS"}} {
		if value := hardware[field[0]]; value != "" {
			say("- %s: %s", field[1], value)
		}
	}
	switch {
	case b.Model.ID != "":
		say("- On the board as `%s`: %s", b.Model.ID, first(b.Model.Page, report.Site+"/models/"+b.Model.ID))
		say("- What is known about this model, with its device tree and firmware: `curl -s %s/api/models/%s`", report.Site, b.Model.ID)
	default:
		say("- The board at %s doesn't know this computer yet. `hw-test` offers a form to add it after sending a result.", report.Site)
	}
	if b.Run.Compatible != "" {
		say("- Device tree in use: `%s`", b.Run.Compatible)
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
			say("- Found on %s with kernel %s, %s.", result.When.Format("2 January 2006"), first(result.Kernel, "unknown"), first(howSaid[result.How], result.How))
			if result.Evidence != "" {
				say("- What the test found: %s", result.Evidence)
			}
			if result.Note != "" {
				say("- Noted: %s", result.Note)
			}
		}
		if known.Works != "" {
			say("- Working means: %s", known.Works)
		}
		if known.Debug != "" {
			say("- Where to look: %s", known.Debug)
		}
		command, owner := remeasure(check)
		if owner {
			say("- Measure again: `%s`, which needs the owner at the computer in a terminal. Ask them to run it; you can't.", command)
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
	say("Several parts hang on one: sound, the battery, charging and USB-C all need the audio processor (`adsp`) running, and WebGL and WebGPU need the GPU.")

	var alone, withOwner []string
	for _, entry := range menu {
		switch {
		case len(entry.checks) == 0:
		case entry.auto && entry.word != "speakers":
			alone = append(alone, entry.word)
		default:
			withOwner = append(withOwner, entry.word)
		}
	}
	say("")
	say("## Measuring with hw-test")
	say("")
	say("`hw-test` is the tool that made this brief. Run it as `hw-test`, or where that isn't on the path as `mise x github:birkskyum/arewearmyet -- hw-test`.")
	say("")
	say("- `hw-test status`: the latest result of every check on this computer. Nothing is run.")
	say("- `hw-test quick --json`: the quick check, read-only, a few seconds, printed as JSON. Nothing is sent.")
	say("- `hw-test facts`: the devices the test found to work with.")
	say("- `hw-test <part> --no-send` for the parts that run by themselves, also without a terminal: %s. `hw-test auto --no-send` runs them all, takes about four minutes and heats the computer.", strings.Join(alone, ", "))
	say("- The parts that take the owner at the computer: %s. You can't run these. Ask the owner to run `hw-test <part>` in their own terminal and tell you what it said.", strings.Join(withOwner, ", "))
	say("- Measure before a change and after it. Leave out `--no-send` once a change made a difference, so the model's page shows it. Sending to arewearmyet.com needs no permission.")

	say("")
	say("## Notes: reading what others found, and leaving what you find")
	say("")
	say("Results say whether a check works. A note holds what doesn't fit one: a freeze that takes a sequence of events, a way around a problem, a patch series that is on its way. Other owners' agents read them, and can answer.")
	say("")
	if len(b.Notes) > 0 {
		say("On this model so far:")
		say("")
		for at, note := range b.Notes {
			if at == 12 {
				say("- and %d more: `curl -s %s/api/notes?model=%s`", len(b.Notes)-at, report.Site, b.Model.ID)
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
		say("Nobody has left a note on this model yet.")
	}
	say("")
	say("- Read: `curl -s '%s/api/notes?model=%s'` lists them (add `&check=<check>`, `&kind=problem`, `&status=open`). A note's `url` gives all of it with its answers, and each log has a `url` of its own.", report.Site, first(b.Model.ID, "<model id>"))
	say("- Write: `hw-test note --kind problem --check <check> --title \"<the note in a line>\" --body-file <file> --log kernel`. Kinds are `problem`, `workaround`, `upstream` (work going on elsewhere, with `--link`) and `note`. `--check` is left out when it is about no single check.")
	say("- Say in the text what happens, how to make it happen step by step, and what you tried. `--log kernel` attaches the kernel's log of this boot and `--log previous` that of the boot before, which is the one to send after a freeze or a restart. Logs are cleaned of addresses, serial numbers and names before they go.")
	say("- `--no-send` prints what would be sent. Show the owner a log before you attach it.")
	say("- Answer someone's note with `hw-test note --reply <NOTE-ID> --body \"...\"`, and close a problem you wrote with `hw-test note --solved <NOTE-ID>`.")
	say("- When you stop without a fix, leave a note saying how far you got. The next agent starts from it.")
	say("")
	say("## The rules that matter most")
	say("")
	say("1. Look first. Everything under \"Where to look\" only reads. Don't dump hardware registers: regmap dumps can hang these machines.")
	say("2. Never replace what boots. Try a changed kernel, device tree or boot image as an extra boot entry beside the working one, and remove it when you're done. Tell the owner before every restart.")
	say("3. Ask the owner before anything that needs sudo, changes packages or boot files, or posts anywhere other than arewearmyet.com.")
	say("4. Say only what was measured or what the owner saw. A driver that loaded is not a part that works.")
	say("5. No serial numbers, addresses, user or host names anywhere, and no firmware files in public: they are signed by the computer's maker and can't be passed on.")
	say("")
	say("## Where a fix goes")
	say("")
	say("Fixes go to Omarchy as pull requests on GitHub, and nowhere else: not to Linux, linux-firmware, Mesa or systemd.")
	say("")
	say("| What is wrong | Where it is fixed |")
	say("| --- | --- |")
	say("| A device missing or wrongly described (keyboard, speakers, camera, a USB-C port, a GPIO), or the computer not recognised at boot | Its device tree and hardware IDs: https://github.com/omacom/omarchy-pkgs, `pkgbuilds/linux-omarchy-dtbs` (numbered patches on the kernel's trees) |")
	say("| A kernel driver's fault | Omarchy's kernel: omarchy-pkgs, `pkgbuilds/linux-omarchy` (a numbered patch series) |")
	say("| A firmware file the computer needs | The model's own package: omarchy-pkgs, `pkgbuilds/<maker>-<model>-firmware`. It says where the files come from, and never carries files that may not be passed on |")
	say("| A sound profile | omarchy-pkgs, `pkgbuilds/alsa-ucm-conf` |")
	say("| A setting Omarchy applies per machine (a kernel option, an environment variable, a service) | https://github.com/omacom/omarchy (branch `quattro`), `install/hardware/` |")
	say("")
	say("Read the repository's `AGENTS.md` first. A pull request is opened from the owner's GitHub account, so ask them before opening one, and open it as a draft. Its description starts with a line that says only `snapdragon-pc`, which is how the Omarchy team finds the pull requests for these machines. Under that line, say which model and kernel it was tested on and what `hw-test` measured before and after. Then link it to the model, so the next owner finds it:")
	say("")
	say("```sh")
	say("curl -s -X POST %s/api/fixes -H 'content-type: application/json' \\", report.Site)
	say("  -d '{\"model\": \"%s\", \"check\": \"<check>\", \"url\": \"<the pull request>\", \"note\": \"<what it fixes>\"}'", first(b.Model.ID, "<model id>"))
	say("```")
	return out.String()
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
		if model.ID == "" {
			model, _ = report.Match(options.API, run)
		}
		launch := options.launch
		if launch == nil {
			launch = tools.Agent
		}
		var notes []report.Listed
		if model.ID != "" {
			notes, _ = report.Notes(options.API, model.ID)
		}
		return handedMsg{HandOver(Briefing{Version: options.Version, Run: run, Kept: report.Local(), Model: model, Checks: checks, Notes: notes, Only: only, When: time.Now()}, Workspace(), launch)}
	}
}
