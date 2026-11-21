package guided

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

// yoga is a briefing for a machine with one part partly working, one broken and one fine.
func yoga() Briefing {
	when := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)
	kept := func(check, state, how, evidence, note string) report.Kept {
		return report.Kept{Result: report.Result{Check: check, State: state, How: how, Evidence: evidence, Note: note}, Kernel: "7.3.0-59-omarchy", When: when}
	}
	return Briefing{
		Version: "0.9.0",
		Run: &report.Run{Compatible: "lenovo,yoga-slim7x", Kernel: "7.3.0-59-omarchy", OS: "Omarchy",
			Hardware: map[string]string{"maker": "LENOVO", "product": "83ED", "family": "Yoga Slim 7 14Q8X9", "bios": "NHCN60WW"},
			Packages: map[string]string{"linux-omarchy": "7.3-59", "mesa": "26.2.3"}},
		Kept: map[string]report.Kept{
			"speakers":  kept("speakers", report.Partial, report.Owner, "by the microphones at half volume: left speaker heard at -31 dB, right speaker heard at -38 dB", "Both speakers play, but the left is louder than the right."),
			"hibernate": kept("hibernate", report.Broken, report.Measured, "the kernel offers no hibernation", ""),
			"wifi":      kept("wifi", report.Works, report.Measured, "connected at 5180 MHz", ""),
			"webgpu":    kept("webgpu", report.Partial, report.Measured, "WebGPU on qualcomm adreno-7xx with --enable-unsafe-webgpu", ""),
		},
		Model: report.Model{ID: "lenovo-yoga-slim-7x-14q8x9", Page: "https://arewearmyet.com/models/lenovo-yoga-slim-7x-14q8x9", Family: "arm-dragon-msft"},
		Checks: map[string]report.Check{
			"speakers": {ID: "speakers", Works: "Both speakers play at a usable volume.", Debug: "Sound runs through the audio DSP. The speaker amplifiers need their reset GPIOs in the tree."},
		},
		When: time.Date(2026, 11, 19, 5, 0, 0, 0, time.UTC),
	}
}

func TestTheBriefSaysWhatFailsWhatIsKnownAndHowToGoAboutIt(t *testing.T) {
	b := yoga()
	b.TreeSource = "x1e80100-lenovo-yoga-slim7x"
	b.Model.Kernels = map[string]string{"stable": "7.3-3.1", "edge": "7.3-71"}
	b.Findings = map[string]report.Finding{
		"hibernate": {State: report.Broken, Note: "Waking from hibernation restarts the laptop.", Checked: "2026-10", Source: "https://github.com/omacom/omarchy/discussions/11823"},
		"wifi":      {State: report.Works, Note: "Fine."},
	}
	brief := Brief(b)
	for _, text := range []string{
		"# Hardware brief for LENOVO Yoga Slim 7 14Q8X9",
		// The guide is named once, with which parts of it to read
		"The guide for this work is https://arewearmyet.com/agents.md", "Read its Words and Rules, then step 5 (Fix it) and step 6 (Notes)",
		"The model is LENOVO 83ED (Yoga Slim 7 14Q8X9).", "It is on arewearmyet.com as `lenovo-yoga-slim-7x-14q8x9`", "/api/models/lenovo-yoga-slim-7x-14q8x9",
		"Its family, which is the first line of a pull request or an issue for it, is `arm-dragon-msft`.",
		"The device tree in use is `lenovo,yoga-slim7x`, built from `arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts`",
		"It runs kernel 7.3.0-59-omarchy, on Omarchy", "linux-omarchy 7.3-59, mesa 26.2.3",
		// Whether it is up to date, which decides whether a fault is worth chasing
		"Omarchy's channels have linux-omarchy at stable 7.3-3.1, edge 7.3-71. This computer has 7.3-59.", "ask the owner whether to update, then run `hw-test ask update`", "It refuses, and says why, where an update would do harm",
		// What fails, worst first, each with what was found and how
		"### Speakers (check `speakers`), partly", "Measured on 19 November 2026 with kernel 7.3.0-59-omarchy, seen by the owner", "left speaker heard at -31 dB", "the left is louder than the right",
		"**What working means.** Both speakers play at a usable volume.", "**Where to look.** Sound runs through the audio DSP.",
		// What the site has written up goes beside what was measured, with its source
		"### Hibernation (check `hibernate`), broken", "**Already known on the site, on Omarchy** (broken, 2026-10). Waking from hibernation restarts the laptop. Source: https://github.com/omacom/omarchy/discussions/11823",
		// A part that takes the owner is said to, and one that runs by itself has its command
		"**Measure again.** `hw-test speakers --no-send --play`, which you run yourself, also without a terminal, once the owner has said yes", "**Measure again.** `hw-test graphics --no-send`, which runs by itself", "**Measure again.** `hw-test quick --no-send`",
		"- **Works.** wifi", "- **Not tested.**",
		// No notes yet isn't the same as nothing known
		"Nobody has left a note on this model yet. That means nobody wrote one, not that nothing is known", "api/notes?q=<words>",
		// One way to go about it, in order, with the commands
		"## How to go about it", "1. **Find where the fix lives.**", "`hw-test verify before <part>`", "`hw-test tree start`, edit, `hw-test tree build`, then the owner runs `hw-test tree boot`",
		"Never install a rebuilt `linux-omarchy-dtbs`, write under `/boot/dtbs` or edit `/boot/limine.conf` by hand", "`hw-test resume", "Open it as a draft.",
		"starts with a line that says only `arm-dragon-msft`, then the block", "`hw-test verify after <part> --pr <its address>", "mark it ready",
		"`hw-test note --kind problem --check <check>", "`hw-test report --check <check> --state partial", "`hw-test tree remove`",
		"The owner's data is the line", "hand the owner the exact command to run",
	} {
		if !strings.Contains(brief, text) {
			t.Errorf("the brief lacks %q", text)
		}
	}
	if broken, partly := strings.Index(brief, "### Hibernation"), strings.Index(brief, "### Speakers"); broken > partly {
		t.Error("what is broken doesn't come before what partly works")
	}
	// What was seen on another distro is told apart from what was seen on Omarchy
	other := yoga()
	other.Findings = map[string]report.Finding{"speakers": {State: report.Works, Basis: "other-distro", Where: "Ubuntu 25.04, kernel 6.14", Note: "All four speakers play.", Checked: "2025-05", Source: "https://bugs.launchpad.net/x"}}
	if with := Brief(other); !strings.Contains(with, "**Reported Ubuntu 25.04, kernel 6.14, not on Omarchy, so a hint and no more** (works, 2025-05). All four speakers play.") || strings.Contains(with, "Already known on the site, on Omarchy** (works") {
		t.Errorf("a report from another distro isn't told apart:\n%s", with)
	}
	// What works isn't something to work on, and its finding isn't quoted
	if strings.Contains(brief, "### Wi-Fi") || strings.Contains(brief, "Fine.") {
		t.Error("a part that works is listed as work")
	}
	// The how-to lives in the guide, so the brief doesn't repeat its tables and rule lists
	for _, repeated := range []string{"| What is wrong |", "## Measuring with hw-test", "## The rules that matter most", "curl -s -X POST"} {
		if strings.Contains(brief, repeated) {
			t.Errorf("the brief repeats the guide's %q", repeated)
		}
	}
	if words := len(strings.Fields(brief)); words > 1300 {
		t.Errorf("the brief has grown to %d words", words)
	}

	// What others wrote down about this model is listed, to read before starting
	noted := yoga()
	noted.Notes = []report.Listed{{ID: "NOTE-AAAAAAAAAA", Check: "suspend", Kind: "problem", Status: "open", Title: "Freezes on wake with a dock", Replies: 2, URL: "https://arewearmyet.com/api/notes/NOTE-AAAAAAAAAA"}}
	if with := Brief(noted); !strings.Contains(with, "- Freezes on wake with a dock (problem, open, suspend), read with `curl -s https://arewearmyet.com/api/notes/NOTE-AAAAAAAAAA`, 2 answers") || strings.Contains(with, "Nobody has left a note") {
		t.Errorf("the notes on the model aren't listed:\n%s", with)
	}

	// Asked for one check, the brief is about that one, also when it was never tested here
	b = yoga()
	b.Only = []string{"camera"}
	if brief = Brief(b); !strings.Contains(brief, "### Webcam (check `camera`), not tested yet") || strings.Contains(brief, "### Speakers") || !strings.Contains(brief, "- **Partly.** speakers, webgpu") {
		t.Errorf("a brief for the camera alone:\n%s", brief)
	}
	// A Chromebook or a Googlebook is Google's family, which its pull requests name
	b = yoga()
	b.Model.Family = "arm-dragon-google"
	if brief = Brief(b); !strings.Contains(brief, "says only `arm-dragon-google`") || strings.Contains(brief, "arm-dragon-msft") {
		t.Error("a Chromebook's brief doesn't name Google's family")
	}
	// A machine the site doesn't know is named by what it says of itself
	b = yoga()
	b.Model, b.Family = report.Model{}, "arm-nvidia"
	if brief = Brief(b); !strings.Contains(brief, "says only `arm-nvidia`") || !strings.Contains(brief, "doesn't know this computer yet") {
		t.Error("an NVIDIA machine's brief doesn't name its family")
	}
	// A Mac's brief says whose it is
	b.Family = "arm-apple"
	if brief = Brief(b); !strings.Contains(brief, "says only `arm-apple` (Apple Silicon Macs belong to the Omarchy Mac team") {
		t.Error("a Mac's brief doesn't say it is the Mac team's")
	}
	// And where nothing tells, the brief gives the choice
	b.Family = ""
	if brief = Brief(b); !strings.Contains(brief, "says only the computer's family (`arm-dragon-msft` for a Snapdragon machine that came with Windows, `arm-dragon-google`") {
		t.Error("an unknown machine's brief doesn't list the families")
	}
	// With nothing failing there is nothing to work on, and it says so
	b = yoga()
	b.Kept = map[string]report.Kept{}
	if brief = Brief(b); !strings.Contains(brief, "Nothing fails in this computer's latest results") {
		t.Errorf("nothing failing:\n%s", brief)
	}
}

// With one part to work on, the snapshot commands carry its name, so the part gets a snapshot
// of its own and isn't measured against another part's.
func TestTheBriefNamesThePartInItsSnapshotCommands(t *testing.T) {
	b := yoga()
	b.Only = []string{"webgpu"}
	brief := Brief(b)
	for _, text := range []string{"`hw-test verify before webgpu`", "`hw-test verify after webgpu`", "`hw-test verify after webgpu --pr <its address>"} {
		if !strings.Contains(brief, text) {
			t.Errorf("the brief lacks %q", text)
		}
	}
	if strings.Contains(brief, "<part>") {
		t.Error("the brief for one part still leaves the part's name open")
	}
}

// An NVIDIA machine describes its own hardware through ACPI, so its brief names no device tree
// commands, as there is no tree to read, build or boot.
func TestTheBriefForAnNvidiaMachineLeavesTheDeviceTreeOut(t *testing.T) {
	b := yoga()
	b.Run.Compatible = ""
	b.Run.Hardware = map[string]string{"maker": "ASUSTeK COMPUTER INC.", "product": "ProArt P16 H7607BA"}
	b.Model = report.Model{ID: "asus-proart-p16-h7607ba", Page: "https://arewearmyet.com/models/asus-proart-p16-h7607ba", Family: "arm-nvidia"}
	brief := Brief(b)
	for _, text := range []string{
		"Its family, which is the first line of a pull request or an issue for it, is `arm-nvidia`.",
		"describes its own hardware through ACPI, so there is no device tree",
		"`hw-test tree` is not for this computer",
		"9. **Clean up.** Restore the settings you changed",
	} {
		if !strings.Contains(brief, text) {
			t.Errorf("the brief lacks %q", text)
		}
	}
	for _, text := range []string{"hw-test tree start", "hw-test tree remove", "/proc/device-tree"} {
		if strings.Contains(brief, text) {
			t.Errorf("the brief for an NVIDIA machine names %q", text)
		}
	}
	// A Snapdragon PC's brief keeps them
	if pc := Brief(yoga()); !strings.Contains(pc, "hw-test tree start") || !strings.Contains(pc, "hw-test tree remove") {
		t.Error("the brief for a Snapdragon PC lost its device tree steps")
	}
}

func TestWordsNameChecksOrPartsOfTheTest(t *testing.T) {
	checks, unknown := ChecksFor([]string{"speakers", "gpu", "graphics", "nonsense", "sleep"})
	if got := strings.Join(checks, " "); got != "speakers gpu webgl webgpu suspend" || strings.Join(unknown, " ") != "nonsense" {
		t.Errorf("checks %q, unknown %q", got, unknown)
	}
	// A word that is both a check and a part is the check
	if checks, _ = ChecksFor([]string{"camera"}); len(checks) != 1 || checks[0] != "camera" {
		t.Errorf("camera: %v", checks)
	}
}

func TestWhatFailsIsHandedToTheOwnersAgentWithTheBrief(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hw-bringup")
	var where, prompt string
	launch := func(dir, text string) error {
		where, prompt = dir, text
		return nil
	}
	handed := HandOver(yoga(), dir, launch)
	written, _ := os.ReadFile(filepath.Join(dir, "BRIEF.md"))
	if !handed.Opened || handed.File != filepath.Join(dir, "BRIEF.md") || !strings.Contains(string(written), "### Speakers") || where != dir {
		t.Fatalf("handed %+v, started in %q", handed, where)
	}
	for _, text := range []string{"Read BRIEF.md in this folder", "hibernation, speakers, webgpu in the browser", "https://arewearmyet.com/agents.md"} {
		if !strings.Contains(prompt, text) {
			t.Errorf("the agent's first message lacks %q: %s", text, prompt)
		}
	}
	// One part alone, as the owner picks it. The brief is for that part, says so, names what else is
	// left for later, and has what an agent without a network or a PATH needs
	one := yoga()
	one.Only, one.Binary = []string{"speakers"}, "/home/owner/.local/share/mise/installs/hw/0.16.0/hw-test"
	one.GetGuide = func() ([]byte, error) { return []byte("# Are we ARM yet? Guide for AI agents\n"), nil }
	one.Kept["camera"] = report.Kept{Result: report.Result{Check: "camera", State: report.Works, How: report.Owner, Evidence: "a 1920x1092 frame", Look: "The picture is 30 of 255 bright."}, Kernel: "7.3.0-59-omarchy", When: one.When}
	handed = HandOver(one, dir, launch)
	written, _ = os.ReadFile(filepath.Join(dir, "BRIEF.md"))
	guide, _ := os.ReadFile(filepath.Join(dir, "GUIDE.md"))
	if !handed.Opened || !strings.HasPrefix(string(guide), "# Are we ARM yet? Guide for AI agents") {
		t.Fatalf("handed %+v, guide %q", handed, guide)
	}
	if !strings.Contains(prompt, "on my computer. It is about speakers, and says") || !strings.Contains(prompt, "GUIDE.md beside it (a copy of https://arewearmyet.com/agents.md)") {
		t.Errorf("the agent's first message: %s", prompt)
	}
	for _, text := range []string{
		"A copy of the guide is in GUIDE.md beside this file", "hw-test 0.9.0 is at `/home/owner/.local/share/mise/installs/hw/0.16.0/hw-test`. If plain `hw-test` doesn't start",
		"## One part at a time", "Stay on it until it works", "Also failing or worth a look on this computer are `hibernate`, `webgpu`, `camera`.", "`hw-test brief <check>` prints the brief for one of them from the results already here, without testing anything again",
	} {
		if !strings.Contains(string(written), text) {
			t.Errorf("the brief for one part lacks %q:\n%s", text, written)
		}
	}
	if strings.Contains(string(written), "### Hibernation") || strings.Contains(string(written), "### Webcam") {
		t.Error("the brief for one part goes into the others")
	}
	// A part that works with something worth a look is briefed as that
	one.Only = []string{"camera"}
	if brief := Brief(one); !strings.Contains(brief, "### Webcam (check `camera`), works, with something to look into") || !strings.Contains(brief, "- **Worth a second look.** The picture is 30 of 255 bright. Find out whether it is a fault") {
		t.Errorf("a part worth a look:\n%s", brief)
	}
	// Handed over by the owner, the speakers are the agent's to measure. The brief says how, and the
	// computer remembers the owner's yes for the sound test
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if Automatic([]string{"speakers"}, false) || MaySound(time.Now()) {
		t.Error("the sound test would run for an agent before the owner handed the speakers over")
	}
	one.Only, one.ByOwner, one.When = []string{"speakers"}, true, time.Now()
	HandOver(one, dir, launch)
	written, _ = os.ReadFile(filepath.Join(dir, "BRIEF.md"))
	if !strings.Contains(string(written), "**Measure again.** `hw-test speakers --no-send`, which you run yourself, also without a terminal") {
		t.Errorf("the brief doesn't give the agent the sound test:\n%s", written)
	}
	if !Automatic([]string{"speakers"}, false) || !Automatic([]string{"microphone", "graphics"}, false) || Automatic([]string{"camera"}, true) || !WantsSound([]string{"auto"}) || WantsSound([]string{"graphics"}) {
		t.Error("after the owner handed the speakers over, the sound test still doesn't run for the agent, or more than it does")
	}
	// An agent that asks for the brief itself is given no such yes
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	one.ByOwner = false
	HandOver(one, dir, launch)
	written, _ = os.ReadFile(filepath.Join(dir, "BRIEF.md"))
	if MaySound(time.Now()) || !strings.Contains(string(written), "`hw-test speakers --no-send --play`, which you run yourself, also without a terminal, once the owner has said yes") {
		t.Error("an agent gave itself the owner's yes to the sound test")
	}
	// A test that takes the owner is opened for them, and nothing is retyped
	one.Only = []string{"camera"}
	if brief := Brief(one); !strings.Contains(brief, "**Measure again.** `hw-test ask camera`, which opens the test in a window for the owner") {
		t.Errorf("a test with the owner:\n%s", brief)
	}

	// Without the guide to copy, the brief and the message point at the site
	one.GetGuide = func() ([]byte, error) { return nil, fmt.Errorf("no network") }
	os.Remove(filepath.Join(dir, "GUIDE.md"))
	HandOver(one, dir, launch)
	if strings.Contains(prompt, "GUIDE.md") || !strings.Contains(prompt, "https://arewearmyet.com/agents.md, has the rules") {
		t.Errorf("without a copy: %s", prompt)
	}

	// Where the owner has a program that keeps conversations, the work goes into one of those,
	// and no terminal agent is opened. When it can't take it, the usual agent opens
	one.Only = []string{"speakers"}
	var threaded []string
	one.InThread = func(dir, title, message string) (string, error) {
		threaded = append(threaded, title+" | "+message[:28])
		return "Your agent has it in T3 Code, in a new thread called \"" + title + "\".", nil
	}
	prompt = ""
	handed = HandOver(one, dir, launch)
	if !handed.Opened || prompt != "" || len(threaded) != 1 || threaded[0] != "Fix speakers | Read BRIEF.md in this folder" || !strings.Contains(handed.Said, "in T3 Code, in a new thread") || !strings.Contains(handed.Said, "BRIEF.md") {
		t.Errorf("into a thread: %+v, threaded %v, terminal %q", handed, threaded, prompt)
	}
	one.InThread = func(string, string, string) (string, error) { return "", fmt.Errorf("T3 Code isn't running") }
	if handed = HandOver(one, dir, launch); !handed.Opened || !strings.Contains(prompt, "Read BRIEF.md") || !strings.Contains(handed.Said, "window of its own") {
		t.Errorf("when the thread can't take it: %+v, terminal %q", handed, prompt)
	}
	one.InThread = nil

	// With no agent chosen, or not on Omarchy, the brief is still written and the owner told what to do
	none := HandOver(yoga(), dir, func(string, string) error { return tools.ErrNoAgent })
	if none.Opened || !strings.Contains(none.Said, "omarchy default agent") || !strings.Contains(none.Said, "BRIEF.md") {
		t.Errorf("no agent chosen: %+v", none)
	}
	elsewhere := HandOver(yoga(), dir, func(string, string) error { return tools.ErrNotOmarchy })
	if elsewhere.Opened || !strings.Contains(elsewhere.Said, "Give it to your agent") {
		t.Errorf("not on Omarchy: %+v", elsewhere)
	}
	// Nothing failing, so no agent is opened for nothing
	fine := yoga()
	fine.Kept = map[string]report.Kept{"wifi": fine.Kept["wifi"]}
	opened := false
	if handed = HandOver(fine, dir, func(string, string) error { opened = true; return nil }); opened || handed.Opened || !strings.Contains(handed.Said, "Nothing fails") {
		t.Errorf("nothing failing: %+v, opened %v", handed, opened)
	}
}

func TestTheScreensOfferTheAgentOnlyWhenSomethingFails(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	run := &report.Run{Hardware: map[string]string{"maker": "LENOVO", "product": "83ED"}}
	run.Set("video-decode", report.Works, report.Measured, "60 frames", "")
	var asked string
	options := Options{Send: false, API: "http://127.0.0.1:1", Version: "0.9.0", launch: func(dir, prompt string) error {
		asked = prompt
		return nil
	}}

	// A round where everything works ends without the offer
	s := &summary{run: run, options: options}
	s.start()
	if strings.Contains(strings.Join(s.ask.options, "|"), "agent") {
		t.Errorf("the agent is offered with nothing failing: %v", s.ask.options)
	}
	// A round with a failure offers it, between going back and leaving, and choosing it keeps the choice up
	run.Set("webgpu", report.Partial, report.Measured, "WebGPU only with switches", "")
	s = &summary{run: run, options: options}
	s.start()
	if got := strings.Join(s.ask.options, " | "); got != "Back to the start | Let an agent look into the webgpu in the browser | Quit" || s.ask.cursor != 0 {
		t.Fatalf("the last choice: %s", got)
	}
	cmd := s.update(press('2'))
	if cmd == nil || s.done() || !strings.Contains(s.view(90, 30), "opening your agent") {
		t.Fatalf("choosing the agent: done %v\n%s", s.done(), s.view(90, 30))
	}
	s.update(cmd())
	if !strings.Contains(asked, "webgpu in the browser") || strings.Contains(asked, "video") || !strings.Contains(s.view(90, 30), "Your agent opened in a window of its own") {
		t.Errorf("asked %q\n%s", asked, s.view(90, 30))
	}

	// With several to look into, the owner picks one, and the offer is there again for the next, so a
	// part that can't be fixed, like hibernation here, holds up nothing
	run.Set("hibernate", report.Broken, report.Measured, "the kernel offers no hibernation", "")
	run.Set("speakers", report.Works, report.Measured, "left -47 dB, right -55 dB", "")
	run.Flag("speakers", "The right speaker measures 8 dB quieter than the other.")
	s = &summary{run: run, options: options}
	s.start()
	if got := strings.Join(s.ask.options, " | "); got != "Back to the start | Let an agent look into one of them | Quit" {
		t.Fatalf("the last choice with several: %s", got)
	}
	if s.update(press('2')) != nil || s.phase != "pick" {
		t.Fatalf("choosing the agent with several: phase %q", s.phase)
	}
	if got := strings.Join(s.ask.options, " | "); got != "WebGPU in the browser (partly). WebGPU only with switches | Hibernation (broken). the kernel offers no hibernation | Speakers (works, with something to look into). The right speaker measures 8 dB quieter than the other | Back" ||
		!strings.Contains(s.ask.question, "one part at a time") {
		t.Fatalf("what to pick from: %s\n%s", got, s.ask.question)
	}
	asked = ""
	cmd = s.update(press('3'))
	if cmd == nil || s.phase != "end" || s.done() {
		t.Fatalf("after picking: phase %q", s.phase)
	}
	s.update(cmd())
	if !strings.Contains(asked, "on my computer. It is about speakers, and says") || strings.Contains(asked, "hibernation") || !strings.Contains(strings.Join(s.ask.options, "|"), "Let an agent look into one of them") {
		t.Errorf("asked %q, then offered %v", asked, s.ask.options)
	}
	// Back from the list leaves everything as it was
	s.update(press('2'))
	if s.update(press('4')) != nil || s.phase != "end" {
		t.Errorf("Back: phase %q", s.phase)
	}

	// The start screen. An agent takes one part at a time, the one under the cursor. On "Everything"
	// there is nothing to hand over, so a part that can't be fixed holds up nothing
	when := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)
	kept := map[string]report.Kept{
		"camera":   {Result: report.Result{Check: "camera", State: report.Partial, Note: "Too dark."}, When: when},
		"usb4":     {Result: report.Result{Check: "usb4", State: report.Broken}, When: when},
		"wifi":     {Result: report.Result{Check: "wifi", State: report.Works}, When: when},
		"speakers": {Result: report.Result{Check: "speakers", State: report.Works, Look: "The right speaker measures 8 dB quieter than the other."}, When: when},
	}
	// What fails comes first, then what works with something worth a look
	if got := strings.Join(Failing(kept), " "); got != "usb4 camera speakers" {
		t.Errorf("what an agent can look into: %s", got)
	}
	start := &welcome{run: run, parts: menu, kept: kept, options: options}
	start.ask = start.question()
	if strings.Contains(start.keys(), "agent") || len(start.failing()) != 0 || start.update(press('a')) != nil {
		t.Errorf("on Everything: keys %q, failing %v", start.keys(), start.failing())
	}
	at := func(word string) {
		for i, part := range start.parts {
			if part.word == word {
				start.ask.cursor = i + 1
			}
		}
	}
	at("camera")
	if strings.Join(start.failing(), " ") != "camera" || !strings.Contains(start.keys(), "a to let an agent look into it") {
		t.Errorf("on the camera: %v, keys %q", start.failing(), start.keys())
	}
	if start.update(press('a')) == nil || start.started {
		t.Error("a didn't hand the camera over")
	}
	// Enter on such a part asks first whether to test it again, or have an agent look into it as it
	// stands
	picked := ""
	start.pick = func(word string) { picked = word }
	start.handed = ""
	if start.update(press(tea.KeyEnter)) != nil || start.offer == nil || start.started || picked != "" {
		t.Fatalf("Enter on a failing part: offer %v, started %v", start.offer != nil, start.started)
	}
	if view := start.view(90, 30); !strings.Contains(view, "Webcam (partly). Too dark") || !strings.Contains(view, "Test it again") || !strings.Contains(view, "Let an agent look into it") {
		t.Errorf("the offer:\n%s", view)
	}
	handing := start.update(press('2'))
	if handing != nil {
		handing()
	}
	if handing == nil || start.offer != nil || start.started || picked != "" || !strings.Contains(asked, "webcam") {
		t.Errorf("the agent wasn't handed the camera: started %v, picked %q, asked %q", start.started, picked, asked)
	}
	start.update(press(tea.KeyEnter))
	start.update(press('1'))
	if !start.started || picked != "camera" || start.offer != nil {
		t.Errorf("testing it again: started %v, picked %q", start.started, picked)
	}
	// A part that works with something worth a look is offered the same way, and says so in the list
	start.started, picked = false, ""
	at("speakers")
	if list := start.question(); !strings.Contains(list.view(110), "worth a look. The right speaker measures 8 dB quieter") {
		t.Errorf("the list doesn't say what is worth a look:\n%s", list.view(110))
	}
	start.update(press(tea.KeyEnter))
	if start.offer == nil || !strings.Contains(start.view(120, 30), "Speakers (works, with something to look into). The right speaker measures 8 dB quieter than the other") {
		t.Errorf("the speakers' offer:\n%s", start.view(90, 30))
	}
	start.update(press(tea.KeyEscape))
	if start.offer != nil || start.started {
		t.Error("Esc didn't go back to the list")
	}
	// On a part that works there is nothing to hand over, and no hint to
	at("wireless")
	// With a conversation going, the screen says what the agent is on, and g goes to it
	if strings.Contains(start.view(110, 60), "Your agent is on") {
		t.Error("an agent is said to be at work where none is")
	}
	start.thread = report.Thread{Kind: "codex", ID: "01a1f404", When: when, Checks: []string{"speakers"}}
	if view := start.view(110, 60); !strings.Contains(view, "Your agent is on speakers, since 02:00 on 19 Nov. Press g to go to it.") {
		t.Errorf("what the agent is on:\n%s", view)
	}
	gone := ""
	start.show = func(thread report.Thread) error { gone = thread.ID; return nil }
	start.handed = ""
	if start.update(press('g')) != nil || gone != "01a1f404" || start.started || !strings.Contains(start.view(110, 60), "Your agent's conversation is open.") {
		t.Errorf("g: gone to %q, started %v", gone, start.started)
	}
	start.thread, start.handed = report.Thread{Kind: "t3", ID: "mcp:7", When: when}, ""
	if view := start.view(110, 60); !strings.Contains(view, "Your agent is on the hardware work in T3 Code") {
		t.Errorf("a T3 Code thread:\n%s", view)
	}
	start.thread, start.handed = report.Thread{}, ""
	if strings.Contains(start.keys(), "agent") || start.update(press('a')) != nil {
		t.Errorf("an agent is offered for a part that works: %q", start.keys())
	}
	// And on a computer where nothing fails, nowhere
	fine := &welcome{run: run, parts: menu, kept: map[string]report.Kept{"wifi": kept["wifi"]}}
	fine.ask = fine.question()
	if strings.Contains(fine.keys(), "agent") || fine.update(press('a')) != nil {
		t.Errorf("an agent is offered with nothing failing: %q", fine.keys())
	}
	_ = tea.KeyEnter
}

func TestTheBriefSaysWhenTheOwnerLetsTheAgentWorkOnItsOwn(t *testing.T) {
	b := yoga()
	asked := Brief(b)
	if strings.Contains(asked, "work on your own until") || !strings.Contains(asked, "run `hw-test trust`") {
		t.Errorf("without the owner's word the brief should point at hw-test trust and claim nothing:\n%s", asked[len(asked)-700:])
	}
	b.Trusted = time.Date(2026, 11, 21, 9, 30, 0, 0, time.Local)
	trusted := Brief(b)
	for _, want := range []string{"The owner has said to work on your own until 09:30 on 21 November", "need no asking", "don't end your turn to check in", "the power button, the disk password"} {
		if !strings.Contains(trusted, want) {
			t.Errorf("the brief with the owner's word lacks %q", want)
		}
	}
}

func TestNewWorkOpensTheOwnersAgentAndT3CodeOnlyWhereAskedFor(t *testing.T) {
	dir, bin, work := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HW_TEST_AGENT", "")
	t.Setenv("HW_TEST_T3_MODEL", "codex/gpt-6-astra")
	// T3 Code is installed and running. Its command notes how it was called
	os.WriteFile(filepath.Join(bin, "t3"), []byte("#!/bin/sh\nprintf '%s|' \"$@\" >> "+dir+"/called\necho >> "+dir+"/called\n[ \"$1\" = thread ] && [ \"$2\" = start ] && echo '{\"threadId\":\"mcp:77\"}'\nexit 0\n"), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	called := func() string {
		data, _ := os.ReadFile(filepath.Join(dir, "called"))
		return string(data)
	}

	// By itself hw-test starts nothing there, and the caller opens the agent Omarchy has set
	t.Setenv("HW_TEST_T3", "")
	if said, err := InThread(work, "Fix speakers", "read the brief"); !errors.Is(err, tools.ErrNoSession) || said != "" {
		t.Fatalf("without being asked for T3 Code: %q, %v", said, err)
	}
	if strings.Contains(called(), "start") {
		t.Errorf("a thread was started in T3 Code unasked: %s", called())
	}

	// A thread that is going there keeps the work
	_ = report.KeepThread(work, report.Thread{Kind: "t3", ID: "mcp:12", When: time.Now(), Title: "Speakers"})
	if said, err := InThread(work, "Fix speakers", "go on"); err != nil || !strings.Contains(said, "the same thread as before") {
		t.Errorf("a thread that is going: %q, %v", said, err)
	}
	if !strings.Contains(called(), "thread|send|mcp:12|go on|") {
		t.Errorf("the going thread wasn't written to: %s", called())
	}

	// Unless the owner leaves T3 Code out, which also forgets that thread
	t.Setenv("HW_TEST_T3", "0")
	if _, err := InThread(work, "Fix speakers", "go on"); !errors.Is(err, tools.ErrNoSession) {
		t.Errorf("with T3 Code left out: %v", err)
	}
	if _, kept := report.ThreadFor(work); kept {
		t.Error("the T3 Code thread is still kept for the folder")
	}

	// And asked for, new work starts there
	t.Setenv("HW_TEST_T3", "1")
	if said, err := InThread(work, "Fix speakers", "read the brief"); err != nil || !strings.Contains(said, "T3 Code") {
		t.Errorf("asked for T3 Code: %q, %v", said, err)
	}
	if kept, ok := report.ThreadFor(work); !ok || kept.Kind != "t3" || kept.ID != "mcp:77" {
		t.Errorf("the new thread isn't kept: %+v, %v", kept, ok)
	}
}
