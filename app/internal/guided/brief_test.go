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

func TestTheBriefSaysWhatFailsHowToMeasureItAndWhereAFixGoes(t *testing.T) {
	brief := Brief(yoga())
	for _, text := range []string{
		"# Hardware brief: LENOVO Yoga Slim 7 14Q8X9", "Read https://arewearmyet.com/agents.md before you do anything",
		"Model: LENOVO 83ED (Yoga Slim 7 14Q8X9)", "`lenovo-yoga-slim-7x-14q8x9`", "/api/models/lenovo-yoga-slim-7x-14q8x9", "Device tree in use: `lenovo,yoga-slim7x`",
		"Kernel: 7.3.0-59-omarchy, on Omarchy", "linux-omarchy 7.3-59, mesa 26.2.3",
		// What fails, worst first, each with what was found and how
		"### Speakers (check `speakers`): partly", "Found on 19 November 2026 with kernel 7.3.0-59-omarchy, seen by the owner", "left speaker heard at -31 dB", "the left is louder than the right",
		"Working means: Both speakers play at a usable volume.", "Where to look: Sound runs through the audio DSP.",
		// A part that takes the owner is said to, and one that runs by itself has its command
		"Measure again: `hw-test speakers`, which needs the owner at the computer", "Measure again: `hw-test graphics --no-send`, which runs by itself",
		"### Hibernation (check `hibernate`): broken", "Measure again: `hw-test quick --no-send`",
		// Where the rest stands
		"- works: wifi", "- not tested:",
		// How to measure, the rules, and where a fix belongs
		"`hw-test status`", "hw-test auto --no-send", "The parts that take the owner at the computer:", "Never replace what boots", "Ask the owner before anything that needs sudo",
		"pkgbuilds/linux-omarchy-dtbs", "pkgbuilds/linux-omarchy`", "https://github.com/omacom/omarchy ", "open it as a draft", "starts with a line that says only `arm-dragon-msft`", `"model": "lenovo-yoga-slim-7x-14q8x9"`,
	} {
		if !strings.Contains(brief, text) {
			t.Errorf("the brief lacks %q", text)
		}
	}
	// How to leave what was found for the next agent, and that nobody has yet
	for _, text := range []string{"Nobody has left a note on this model yet.", "hw-test note --kind problem --check <check>", "--log previous", "api/notes?model=lenovo-yoga-slim-7x-14q8x9", "leave a note saying how far you got"} {
		if !strings.Contains(brief, text) {
			t.Errorf("the brief lacks %q", text)
		}
	}
	// What others wrote down about this model is listed, to read before starting
	noted := yoga()
	noted.Notes = []report.Listed{{ID: "NOTE-AAAAAAAAAA", Check: "suspend", Kind: "problem", Status: "open", Title: "Freezes on wake with a dock", Replies: 2, URL: "https://arewearmyet.com/api/notes/NOTE-AAAAAAAAAA"}}
	if with := Brief(noted); !strings.Contains(with, "- Freezes on wake with a dock (problem, open, suspend): `curl -s https://arewearmyet.com/api/notes/NOTE-AAAAAAAAAA`, 2 answers") || strings.Contains(with, "Nobody has left a note") {
		t.Errorf("the notes on the model aren't listed:\n%s", with)
	}
	if broken, partly := strings.Index(brief, "### Hibernation"), strings.Index(brief, "### Speakers"); broken > partly {
		t.Error("what is broken doesn't come before what partly works")
	}
	// What works isn't something to work on
	if strings.Contains(brief, "### Wi-Fi") {
		t.Error("a part that works is listed as work")
	}

	// Asked for one check, the brief is about that one, also when it was never tested here
	b := yoga()
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
	// A machine the board doesn't know is named by what it says of itself
	b = yoga()
	b.Model, b.Family = report.Model{}, "arm-nvidia"
	if brief = Brief(b); !strings.Contains(brief, "says only `arm-nvidia`") {
		t.Error("an NVIDIA machine's brief doesn't name its family")
	}
	// A Mac's brief says whose it is
	b.Family = "arm-apple"
	if brief = Brief(b); !strings.Contains(brief, "says only `arm-apple` (Apple Silicon Macs belong to the Omarchy Mac team") {
		t.Error("a Mac's brief doesn't say it is the Mac team's")
	}
	// And where nothing tells, the brief gives the choice
	b.Family = ""
	if brief = Brief(b); !strings.Contains(brief, "says only the machine's family (`arm-dragon-msft` for a Snapdragon machine that came with Windows, `arm-dragon-google`") {
		t.Error("an unknown machine's brief doesn't list the families")
	}
	// A computer the board doesn't know, with nothing failing
	b = yoga()
	b.Model, b.Kept = report.Model{}, map[string]report.Kept{}
	if brief = Brief(b); !strings.Contains(brief, "doesn't know this computer yet") || !strings.Contains(brief, "Nothing fails in this computer's latest results") || !strings.Contains(brief, `"model": "<model id>"`) {
		t.Errorf("an unknown computer:\n%s", brief)
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
