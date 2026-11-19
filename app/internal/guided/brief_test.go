package guided

import (
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
		"# Hardware brief: LENOVO Yoga Slim 7 14Q8X9",
		// The guide is named once, with which parts of it to read
		"The guide for this work is https://arewearmyet.com/agents.md", "Read its Words and Rules, then step 5 (Fix it) and step 6 (Notes)",
		"Model: LENOVO 83ED (Yoga Slim 7 14Q8X9)", "On arewearmyet.com as `lenovo-yoga-slim-7x-14q8x9`", "/api/models/lenovo-yoga-slim-7x-14q8x9",
		"Family, which is the first line of a pull request or an issue for it: `arm-dragon-msft`",
		"Device tree in use: `lenovo,yoga-slim7x`, built from `arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts`",
		"Kernel: 7.3.0-59-omarchy, on Omarchy", "linux-omarchy 7.3-59, mesa 26.2.3",
		// Whether it is up to date, which decides whether a fault is worth chasing
		"Omarchy's channels have linux-omarchy at: stable 7.3-3.1, edge 7.3-71. This computer has 7.3-59.", "ask the owner to run `omarchy update`",
		// What fails, worst first, each with what was found and how
		"### Speakers (check `speakers`): partly", "Measured on 19 November 2026 with kernel 7.3.0-59-omarchy, seen by the owner", "left speaker heard at -31 dB", "the left is louder than the right",
		"Working means: Both speakers play at a usable volume.", "Where to look: Sound runs through the audio DSP.",
		// What the site has written up goes beside what was measured, with its source
		"### Hibernation (check `hibernate`): broken", "Already known on the site, on Omarchy (broken, 2026-10): Waking from hibernation restarts the laptop. Source: https://github.com/omacom/omarchy/discussions/11823",
		// A part that takes the owner is said to, and one that runs by itself has its command
		"Measure again: `hw-test speakers`, run by the owner in their own terminal", "Measure again: `hw-test graphics --no-send`, which runs by itself", "Measure again: `hw-test quick --no-send`",
		"- works: wifi", "- not tested:",
		// No notes yet isn't the same as nothing known
		"Nobody has left a note on this model yet. That means nobody wrote one, not that nothing is known", "api/notes?q=<words>",
		// One way to go about it, in order, with the commands
		"## How to go about it", "1. **Check it is worth chasing.**", "`hw-test verify before`", "`hw-test tree start`, edit, `hw-test tree build`, then the owner runs `hw-test tree boot`",
		"Never install a rebuilt `linux-omarchy-dtbs`, write under `/boot/dtbs` or edit `/boot/limine.conf` by hand", "`hw-test resume", "Open it as a draft.",
		"starts with a line that says only `arm-dragon-msft`, then the block", "`hw-test verify after --pr <its address>", "mark it ready",
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
	if with := Brief(other); !strings.Contains(with, "Reported Ubuntu 25.04, kernel 6.14, not on Omarchy, so a hint and no more (works, 2025-05): All four speakers play.") || strings.Contains(with, "Already known on the site, on Omarchy (works") {
		t.Errorf("a report from another distro isn't told apart:\n%s", with)
	}
	// What works isn't something to work on, and its finding isn't quoted
	if strings.Contains(brief, "### Wi-Fi") || strings.Contains(brief, "Fine.") {
		t.Error("a part that works is listed as work")
	}
	// The how-to lives in the guide: the brief doesn't repeat its tables and rule lists
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
	if with := Brief(noted); !strings.Contains(with, "- Freezes on wake with a dock (problem, open, suspend): `curl -s https://arewearmyet.com/api/notes/NOTE-AAAAAAAAAA`, 2 answers") || strings.Contains(with, "Nobody has left a note") {
		t.Errorf("the notes on the model aren't listed:\n%s", with)
	}

	// Asked for one check, the brief is about that one, also when it was never tested here
	b = yoga()
	b.Only = []string{"camera"}
	if brief = Brief(b); !strings.Contains(brief, "### Webcam (check `camera`): not tested yet") || strings.Contains(brief, "### Speakers") || !strings.Contains(brief, "- partly: speakers, webgpu") {
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
	// With no agent chosen, or not on Omarchy, the brief is still written and the owner told what to do
	none := HandOver(yoga(), dir, func(string, string) error { return tools.ErrNoAgent })
	if none.Opened || !strings.Contains(none.Said, "omarchy default agent") || !strings.Contains(none.Said, "BRIEF.md") {
		t.Errorf("no agent chosen: %+v", none)
	}
	elsewhere := HandOver(yoga(), dir, func(string, string) error { return tools.ErrNotOmarchy })
	if elsewhere.Opened || !strings.Contains(elsewhere.Said, "Give it to your agent") {
		t.Errorf("not on Omarchy: %+v", elsewhere)
	}
	// Nothing failing: no agent is opened for nothing
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
	if strings.Contains(strings.Join(s.ask.options, "|"), askAgent) {
		t.Errorf("the agent is offered with nothing failing: %v", s.ask.options)
	}
	// A round with a failure offers it, between going back and leaving, and choosing it keeps the choice up
	run.Set("webgpu", report.Partial, report.Measured, "WebGPU only with switches", "")
	s = &summary{run: run, options: options}
	s.start()
	if got := strings.Join(s.ask.options, " | "); got != "Back to the start | "+askAgent+" | Quit" || s.ask.cursor != 0 {
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

	// The start screen: a for the part under the cursor, or for everything that fails
	when := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)
	kept := map[string]report.Kept{
		"camera": {Result: report.Result{Check: "camera", State: report.Partial, Note: "Too dark."}, When: when},
		"usb4":   {Result: report.Result{Check: "usb4", State: report.Broken}, When: when},
		"wifi":   {Result: report.Result{Check: "wifi", State: report.Works}, When: when},
	}
	start := &welcome{run: run, parts: menu, kept: kept, options: options}
	start.ask = start.question()
	if !strings.Contains(start.keys(), "a to let an agent look into what fails") || strings.Join(start.failing(), " ") != "usb4 camera" {
		t.Errorf("keys %q, failing %v", start.keys(), start.failing())
	}
	for at, part := range start.parts {
		if part.word == "camera" {
			start.ask.cursor = at + 1
		}
	}
	if strings.Join(start.failing(), " ") != "camera" {
		t.Errorf("on the camera: %v", start.failing())
	}
	if start.update(press('a')) == nil || start.started {
		t.Error("a didn't hand the camera over")
	}
	// On a part that works there is nothing to hand over, and no hint to
	for at, part := range start.parts {
		if part.word == "wireless" {
			start.ask.cursor = at + 1
		}
	}
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
