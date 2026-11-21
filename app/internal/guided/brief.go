package guided

import (
	"errors"
	"fmt"
	"io"
	"net/http"
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

// Briefing is what goes into a brief, which is the computer, what the tests found on it, and what
// the site knows about each check.
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
	// Binary is where the hw-test that wrote the brief is, for an agent whose surroundings
	// don't find it by name.
	Binary string
	// Trusted is until when the owner has said the agent works on its own (hw-test trust). Zero
	// when they haven't.
	Trusted time.Time
	// ByOwner is set when the owner themselves asked for the handing over, at a terminal. It
	// is what lets the agent run the handed parts' tests itself.
	ByOwner bool
	// InThread hands the work to a conversation that keeps, where the owner has a program for
	// that, and says how it went. Nil, or an error from it, opens the agent the usual way.
	InThread func(dir, title, message string) (string, error)
	// GetGuide fetches the guide's text, to put a copy beside the brief. Nil leaves it out.
	GetGuide func() ([]byte, error)
	// guideCopy is set once that copy is written.
	guideCopy bool
}

// Command is where an agent finds hw-test when its name alone isn't found. It is the command the
// owner types, which follows updates, and only without that the file that is running now. A path
// with a version in it would keep an agent on that version after an update.
func Command() string {
	if home, err := os.UserHomeDir(); err == nil {
		if installed := filepath.Join(home, ".local", "bin", "hw-test"); tools.Have(installed) {
			return installed
		}
	}
	running, _ := os.Executable()
	return running
}

// InThread hands work to the conversation the owner's agent already has for this folder, so nothing
// the agent learnt is lost and the owner has one place to look. That is the thread hw-test started
// in T3 Code, or the conversation of an agent in a terminal, which says which it is whenever it
// calls hw-test. Without one, and where T3 Code is installed, a new thread is started there, since
// T3 Code runs whichever agent the owner has, and keeps the thread.
func InThread(dir, title, message string) (string, error) {
	if os.Getenv("HW_TEST_AGENT") != "" {
		return "", tools.ErrNoSession
	}
	if kept, ok := report.ThreadFor(dir); ok {
		err := tools.Continue(tools.Session{Kind: kept.Kind, ID: kept.ID}, dir, message)
		switch {
		case err == nil && kept.Kind == "t3":
			tools.T3Show(dir)
			which := "the same thread as before"
			if kept.Title != "" {
				which += ", \"" + kept.Title + "\""
			}
			return "Your agent has it in T3 Code, in " + which + ". It is in the list on the left, under " + filepath.Base(dir) + ".", nil
		case err == nil:
			return "Your agent has it, in the same conversation as before.", nil
		case errors.Is(err, tools.ErrAgentOpen):
			tools.FocusAgentWindow()
			return "Your agent's window is open, and it can't be written to from here. Tell it to read BRIEF.md again, which is about " + strings.TrimPrefix(title, "Fix ") + " now.", nil
		}
	}
	// In T3 Code's own window first, typed into the thread it opens, so the thread in front of the
	// owner is the one at work. Where that can't be done, a thread is started behind the window and
	// the owner is told which one to click
	prior, _ := report.ThreadFor(dir)
	if id, named, err := tools.T3Type(dir, message); err == nil {
		if id != "" {
			_ = report.KeepThread(dir, report.Thread{Kind: "t3", ID: id, When: time.Now(), Title: named, Checks: prior.Checks})
		}
		return "Your agent has it in T3 Code, in the thread that just opened.", nil
	}
	id, err := tools.T3Start(dir, title, message)
	if err != nil {
		return "", err
	}
	_ = report.KeepThread(dir, report.Thread{Kind: "t3", ID: id, When: time.Now(), Title: title, Checks: prior.Checks})
	tools.T3Show(dir)
	return "Your agent has it in T3 Code, in a new thread called \"" + title + "\". Click it in the list on the left, under " + filepath.Base(dir) + ", to watch.", nil
}

// FetchGuide reads the guide from the site.
func FetchGuide() ([]byte, error) {
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(Guide)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the site answered %s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 1<<20))
}

// sounds are the checks the sound test measures, with tones played and listened to.
var sounds = map[string]bool{"speakers": true, "microphones": true}

// worthALook says whether a result works but measured something an agent could look into.
func worthALook(result report.Result) bool {
	return result.State == report.Works && result.Look != ""
}

// summed says a result in a line for a list to pick from, with the part, how it stands and why.
func summed(check string, result report.Result) string {
	state := stateLabel[result.State]
	if worthALook(result) {
		state = "works, with something to look into"
	}
	line := first(checkTitle[check], check) + " (" + state + ")"
	if why := first(result.Look, result.Note, result.Evidence); why != "" {
		if len(why) > 90 {
			why = why[:90]
			if cut := strings.LastIndexByte(why, ' '); cut > 50 {
				why = why[:cut]
			}
			why += "…"
		}
		line += ". " + strings.TrimRight(why, ".")
	}
	return line
}

// Summed says each of the given checks in a line, from the machine's latest results.
func Summed(kept map[string]report.Kept, checks []string) []string {
	var lines []string
	for _, check := range checks {
		lines = append(lines, summed(check, kept[check].Result))
	}
	return lines
}

// Fill adds what the machine and the site know beyond the results, which is the family, the device
// tree's source, what is written up about the model and the notes left on it. Whatever can't be had
// is left out.
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

// ChecksFor turns words into checks. A word is a check's own id, or a part of the test, which
// stands for the checks it tests.
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

// Failing lists what an agent can be asked to look into in the machine's latest results. Those are
// the checks that are broken, those partly working, and those that work with something worth a
// second look, in that order.
func Failing(kept map[string]report.Kept) []string {
	rank := func(check string) int {
		if worthALook(kept[check].Result) {
			return 1
		}
		if severity[kept[check].State] > 1 {
			return severity[kept[check].State]
		}
		return 0
	}
	var failing []string
	for check := range kept {
		if rank(check) > 0 {
			failing = append(failing, check)
		}
	}
	sort.Slice(failing, func(a, b int) bool {
		if rank(failing[a]) != rank(failing[b]) {
			return rank(failing[a]) > rank(failing[b])
		}
		return failing[a] < failing[b]
	})
	return failing
}

var howSaid = map[string]string{report.Measured: "measured by the test", report.Owner: "seen by the owner", "agent": "read by an agent"}

// Brief writes what an agent needs to pick a failing part up, which is the computer, what was
// measured and what is already known of each part, and the order to go about it in. The guide has
// the rules and the how-to in full, so the brief only names them. It holds no serial number,
// address, user or host.
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

	say("# Hardware brief for %s", name)
	say("")
	say("Written by hw-test %s on %s. The owner of this computer asked for help getting the parts below to work under Omarchy.", b.Version, b.When.Format("2 January 2006"))
	say("")
	say("The guide for this work is %s. Its steps 1 to 4 are done for you, since the computer is identified and measured. **Read its Words and Rules, then step 5 (Fix it) and step 6 (Notes), before you change anything.** This file has what is particular to this computer.%s", Guide, map[bool]string{true: " A copy of the guide is in GUIDE.md beside this file, for where the network is closed to you.", false: ""}[b.guideCopy])
	say("")
	say("## This computer")
	say("")
	model := strings.TrimSpace(hardware["maker"] + " " + hardware["product"])
	if text := hardware["family"]; text != "" {
		model += " (" + text + ")"
	}
	say("- The model is %s.", model)
	for _, field := range [][2]string{{"sku", "SKU"}, {"board", "mainboard"}, {"bios", "BIOS"}} {
		if value := hardware[field[0]]; value != "" {
			say("- Its %s is %s.", field[1], value)
		}
	}
	switch {
	case b.Model.ID != "":
		say("- It is on arewearmyet.com as `%s`, at %s", b.Model.ID, first(b.Model.Page, report.Site+"/models/"+b.Model.ID))
		say("- Everything known about this model, with its device tree and firmware, comes from `curl -s %s/api/models/%s`.", report.Site, b.Model.ID)
	default:
		say("- arewearmyet.com doesn't know this computer yet. `hw-test` offers a form to add it after sending a result.")
	}
	if family != "" {
		say("- Its family, which is the first line of a pull request or an issue for it, is `%s`.", family)
	}
	if b.Run.Compatible != "" {
		line := fmt.Sprintf("- The device tree in use is `%s`", b.Run.Compatible)
		if b.TreeSource != "" {
			line += fmt.Sprintf(", built from `arch/arm64/boot/dts/qcom/%s.dts`", b.TreeSource)
		}
		say("%s", line)
	}
	say("- It runs kernel %s, on %s.", b.Run.Kernel, b.Run.OS)
	if b.Binary != "" {
		say("- hw-test %s is at `%s`. If plain `hw-test` doesn't start where you run, as in a sandbox, call it by that path. It follows updates, so don't call a copy by its version.", b.Version, b.Binary)
	}
	var packages []string
	for name, version := range b.Run.Packages {
		packages = append(packages, name+" "+version)
	}
	sort.Strings(packages)
	if len(packages) > 0 {
		say("- Its packages are %s.", strings.Join(packages, ", "))
	}
	// Whether the computer is up to date decides whether a fault is still worth chasing
	if len(b.Model.Kernels) > 0 {
		var channels []string
		for _, channel := range []string{"stable", "rc", "edge"} {
			if version := b.Model.Kernels[channel]; version != "" {
				channels = append(channels, channel+" "+version)
			}
		}
		say("- Omarchy's channels have linux-omarchy at %s. This computer has %s. If it is behind its channel, ask the owner whether to update, then run `hw-test ask update`, which opens Omarchy's update in a window for them and gives you how it ended, so they relay nothing. It refuses, and says why, where an update would do harm (a test boot entry stands, the kernel is not Omarchy's own, a kernel or device tree package was built on this computer). Restart after it before you debug (leave word with `hw-test resume` first, as before any restart), since the fault may be fixed already.", strings.Join(channels, ", "), first(b.Run.Packages["linux-omarchy"], "an unknown version"))
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
			if worthALook(result.Result) {
				state = "works, with something to look into"
			}
		}
		say("### %s (check `%s`), %s", first(checkTitle[check], known.Title, check), check, state)
		say("")
		if tested {
			say("- Measured on %s with kernel %s, %s.", result.When.Format("2 January 2006"), first(result.Kernel, "unknown"), first(howSaid[result.How], result.How))
			if result.Evidence != "" {
				say("- **What the test found.** %s", result.Evidence)
			}
			if result.Note != "" {
				say("- **Noted.** %s", result.Note)
			}
			if worthALook(result.Result) {
				say("- **Worth a second look.** %s Find out whether it is a fault, and say so to the owner either way.", result.Look)
			}
		}
		// What is written up on the site about this part, which often names the cause
		if finding, ok := b.Findings[check]; ok && finding.Note != "" {
			// A report from another distro or from the tree's author is a hint, and is said to be one
			line := fmt.Sprintf("- **Already known on the site, on Omarchy** (%s", stateLabel[finding.State])
			if !finding.OnOmarchy() {
				from := map[string]string{"other-distro": "on another distro", "tree-author": "by the device tree's author"}[finding.Basis]
				line = fmt.Sprintf("- **Reported %s, not on Omarchy, so a hint and no more** (%s", first(finding.Where, from), stateLabel[finding.State])
			}
			if finding.Checked != "" {
				line += ", " + finding.Checked
			}
			line += "). " + finding.Note
			if finding.Source != "" {
				line += " Source: " + finding.Source
			}
			say("%s", line)
		}
		if known.Works != "" {
			say("- **What working means.** %s", known.Works)
		}
		if known.Debug != "" {
			say("- **Where to look.** %s", known.Debug)
		}
		command, owner := remeasure(check)
		if sounds[check] && b.ByOwner {
			say("- **Measure again.** `%s --no-send`, which you run yourself, also without a terminal. The owner handed you this part, and that lets the test play its tones at half volume for about twenty seconds and listen to them with the microphones. It keeps only how loud each tone was. The owner hears it, so say a word before the first run.", command)
		} else if sounds[check] {
			say("- **Measure again.** `%s --no-send --play`, which you run yourself, also without a terminal, once the owner has said yes to it. It plays tones at half volume for about twenty seconds and listens to them with the microphones, and keeps only how loud each tone was. Ask them once.", command)
		} else if owner {
			say("- **Measure again.** `%s`, which opens the test in a window for the owner, since it needs them at the computer, waits until they are through and prints the result. Tell them a window is coming.", strings.Replace(command, "hw-test ", "hw-test ask ", 1))
		} else {
			say("- **Measure again.** `%s --no-send`, which runs by itself, also without a terminal.", strings.TrimSuffix(command, " --json"))
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
			say("- **%s.** %s", strings.ToUpper(state[:1])+state[1:], strings.Join(checks, ", "))
		}
	}
	say("")
	say("Several parts hang on one. Sound, the battery, charging and USB-C all need the audio processor (`adsp`) running, and WebGL and WebGPU need the GPU. `hw-test status` shows all of it at any time.")

	// One part at a time. A part that can't be fixed must not hold up the others
	var others []string
	for _, check := range Failing(b.Kept) {
		if !covered[check] {
			others = append(others, "`"+check+"`")
		}
	}
	say("")
	say("## One part at a time")
	say("")
	say("This brief is for what is under \"What to work on\" and nothing else. Stay on it until it works. When the first thing you try isn't the fix, the fix lives somewhere else, in a setting, a kernel patch that exists elsewhere, or a driver nobody has written yet. The guide's \"Keep going\" has the order to try them in and the few reasons to hand the part back. A part you hand back comes with what you tried at each step and the next thing worth trying.")
	if len(others) > 0 {
		say("")
		say("Also failing or worth a look on this computer are %s. They are for later. `hw-test brief <check>` prints the brief for one of them from the results already here, without testing anything again, and the owner can hand you one with `hw-test fix`.", strings.Join(others, ", "))
	}

	say("")
	say("## Notes on this model")
	say("")
	if len(b.Notes) > 0 {
		for at, note := range b.Notes {
			if at == 12 {
				say("- and %d more, from `curl -s '%s/api/notes?model=%s'`", len(b.Notes)-at, report.Site, b.Model.ID)
				break
			}
			about := note.Kind
			if note.Kind == "problem" && note.Status != "" {
				about += ", " + note.Status
			}
			if note.Check != "" {
				about += ", " + note.Check
			}
			line := fmt.Sprintf("- %s (%s), read with `curl -s %s`", note.Title, about, note.URL)
			if note.Replies > 0 {
				line += fmt.Sprintf(", %d answers", note.Replies)
			}
			say("%s", line)
		}
		say("")
		say("Read the ones that touch your work before you start, since someone may have been here.")
	} else {
		say("Nobody has left a note on this model yet. That means nobody wrote one, not that nothing is known. See \"Already known on the site\" above, and search every model's notes with `curl -s '%s/api/notes?q=<words>'`.", report.Site)
	}

	say("")
	say("## How to go about it")
	say("")
	say("Each step is explained in the guide's step 5 and 6. In short:")
	say("")
	say("1. **Find where the fix lives.** Up to date (above)? Already known, with a cause? Then it is the device tree, a setting, the kernel or another Omarchy package, and the guide's table says which holds what. Very little is out of reach, like firmware only the maker can sign.")
	// One part's work is measured against a snapshot of its own, named after it, so that it
	// isn't compared with where the computer stood before some other part's change
	before, after := "hw-test verify before <part>", "hw-test verify after <part>"
	if len(work) == 1 {
		before, after = "hw-test verify before "+work[0], "hw-test verify after "+work[0]
	}
	say("2. **Snapshot.** `%s`, once, before you change anything. The name keeps this part's snapshot apart from another part's. When the owner hears or sees a part differently than the test says, record that first with `hw-test report`.", before)
	// An NVIDIA machine describes its own hardware through ACPI, so it has no device tree to read or
	// change
	acpi := family == probe.Nvidia
	if acpi {
		say("3. **Look.** The commands under \"Where to look\" only read. This computer describes its own hardware through ACPI, so there is no device tree. `sudo dmesg` and `journalctl -k -b` say what each driver found.")
		say("4. **Change.** Try a setting on this computer, keeping a copy of what you change. There is no device tree here, so `hw-test tree` is not for this computer. A fault is in a kernel driver, a system setting or an application's setting, and the guide's table says where each of those is fixed.")
	} else {
		say("3. **Look.** The commands under \"Where to look\" only read. `dtc -I fs -O dts /proc/device-tree` shows the device tree the computer is running.")
		say("4. **Change.** Try a setting on this computer, keeping a copy of what you change. For the device tree, run `hw-test tree start`, edit, `hw-test tree build`, then the owner runs `hw-test tree boot`, which makes a boot entry of its own, and picks it at the next start. When it works, `hw-test tree patch` writes the change as the patch Omarchy's package takes. Never install a rebuilt `linux-omarchy-dtbs`, write under `/boot/dtbs` or edit `/boot/limine.conf` by hand. Each can stop the computer from starting.")
	}
	say("5. **Before a restart, or a test that may freeze the computer:** write where you are into `PROGRESS.md`, then `hw-test resume \"<what to go on with>\"`. You are opened again at the owner's next login.")
	say("6. **Measure again.** The \"Measure again\" command of each part you touched, and a neighbouring part. Then `" + after + "`, which prints a before-and-after block (`--check <check>` gives a row to a part that works on both sides but measures better).")
	say("7. **Pull request.** Open it as a draft. Its description starts with a line that says only %s, then the block. Then `"+after+" --pr <its address> --title \"<the change in a line>\"`, which records it on the model's page, and mark it ready. Tell the owner.", familyLine(family))
	say("8. **Not fixed yet?** Go to the next step of the guide's \"Keep going\", which is a setting, then a patch from elsewhere, then a driver of your own. Only when that list is done, or one of its reasons to hand back holds, leave it for the next agent with `hw-test note --kind problem --check <check> --title \"...\" --body-file <file> --log kernel`. What the owner told you about a part goes in with `hw-test report --check <check> --state partial --note \"...\"`.")
	if acpi {
		say("9. **Clean up.** Restore the settings you changed, remove test packages, and say what is left. A change you leave on the computer because the part works only with it is recorded with `hw-test report --check <check> --how agent --needs \"<the change and what it does for the part, in a sentence>\"`, so the model's page says the part works with a change made on this computer.")
	} else {
		say("9. **Clean up.** `hw-test tree remove` (run by the owner), restore the settings you changed, and say what is left. A change you leave on the computer because the part works only with it is recorded with `hw-test report --check <check> --how agent --needs \"<the change and what it does for the part, in a sentence>\"`, so the model's page says the part works with a change made on this computer.")
	}
	say("")
	say("The owner's data is the line. Never touch their files, the disk's partitions or its encryption, and do no firmware flashing or BIOS changes. Commands that need root go through `sudo`. When it asks for a password, run `hw-test trust`, which asks the owner once to let you work on your own, or hand the owner the exact command to run.")
	if !b.Trusted.IsZero() {
		say("")
		say("**The owner has said to work on your own until " + b.Trusted.Local().Format("15:04 on 2 January") + "** (`hw-test trust`). Until then a restart, a system file, a settings file of theirs and a stock package need no asking, and `sudo` asks for no password. Keep a copy of what you change, as always. Go from step to step without asking whether to go on, and don't end your turn to check in. Stop only when the part works, when the next thing takes the owner (the power button, the disk password, their eyes or ears), or when the guide's \"Keep going\" list is done. `hw-test trust --show` says whether it still holds.")
	}
	return out.String()
}

// familyLine is how the brief names the line a pull request opens with. That is the machine's
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
		return Handed{Said: "The brief couldn't be written (" + err.Error() + ")."}
	}
	// A copy of the guide goes beside the brief, since an agent's surroundings often have no network
	where := Guide
	if b.GetGuide != nil {
		if text, err := b.GetGuide(); err == nil && len(text) > 0 && os.WriteFile(filepath.Join(dir, "GUIDE.md"), text, 0o644) == nil {
			b.guideCopy = true
			where = "GUIDE.md beside it (a copy of " + Guide + ")"
		}
	}
	if err := os.WriteFile(file, []byte(Brief(b)), 0o644); err != nil {
		return Handed{Said: "The brief couldn't be written (" + err.Error() + ")."}
	}
	// The owner's handing a part over is what lets the agent test that part itself
	if b.ByOwner {
		_ = report.Hand(work, b.When)
		_ = report.NoteWork(dir, work, time.Now())
	}
	var titles []string
	for _, check := range work {
		titles = append(titles, strings.ToLower(first(checkTitle[check], check)))
	}
	prompt := fmt.Sprintf("Read BRIEF.md in this folder and help me get this working on my computer. It is about %s, and says what was measured, how to measure again with hw-test and where to look. The guide it points to, %s, has the rules and says where a fix goes, so read that first.", strings.Join(titles, ", "), where)
	// A conversation that keeps comes first. The next part and every new result then go into the same
	// one, whichever agent runs in it
	if b.InThread != nil {
		if said, err := b.InThread(dir, "Fix "+strings.Join(titles, ", "), prompt); err == nil {
			return Handed{File: file, Opened: true, Said: said + " The brief is in " + file + "."}
		}
	}
	err := launch(dir, prompt)
	switch {
	case err == nil:
		return Handed{File: file, Opened: true, Said: "Your agent opened in a window of its own, with the brief in " + file + "."}
	case errors.Is(err, tools.ErrNoAgent):
		return Handed{File: file, Said: "No agent is chosen on this computer yet. Pick one in the Omarchy menu under Setup, Default, Agent, or with omarchy default agent claude, and choose this again. The brief is in " + file + "."}
	case errors.Is(err, tools.ErrNotOmarchy):
		return Handed{File: file, Said: "The brief is in " + file + ". Give it to your agent with the words \"Read " + file + " and help me get this working.\""}
	}
	return Handed{File: file, Said: "The agent couldn't be opened (" + err.Error() + "). The brief is in " + file + "."}
}

// handedMsg carries how handing over went back to the screen that asked.
type handedMsg struct{ handed Handed }

// handOver hands the given checks to the agent without holding the screen up. The site is asked for
// its notes on the checks first, and the brief is written without them when it doesn't answer.
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
		launch, guide := options.launch, options.guide
		if launch == nil {
			launch, guide = tools.Agent, FetchGuide
		}
		trusted, _ := report.Trusted(time.Now())
		briefing := Briefing{Version: options.Version, Run: run, Kept: report.Local(), Model: model, Checks: checks, Only: only, When: time.Now(), GetGuide: guide, Trusted: trusted, ByOwner: true}
		if options.launch == nil {
			briefing.InThread = InThread
		}
		briefing.Binary = Command()
		briefing.Fill(options.API)
		return handedMsg{HandOver(briefing, Workspace(), launch)}
	}
}
