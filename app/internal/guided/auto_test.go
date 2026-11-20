package guided

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/birkskyum/arewearmyet/app/internal/deep"
	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

func measured(check, state, evidence, note string) report.Result {
	return report.Result{Check: check, State: state, How: report.Measured, Evidence: evidence, Note: note}
}

// autos is an automatic step in each of its phases, for the tests that draw every step.
func autos(run *report.Run) []step {
	test := func(name string) deep.Test {
		return deep.Test{Name: name, Doing: "Opening a test page in Chromium, without a window", About: "Chromium opens a page that asks for WebGL and for WebGPU, which maps and 3D pages draw with.",
			Nothing: "Chromium didn't answer the test page, so there is nothing to report.", Run: func(func(string)) []report.Result { return nil }}
	}
	running := newAuto(run, test("Running"))
	running.start()
	shown := newAuto(run, test("Shown"))
	shown.start()
	shown.update(autoMsg{name: "Shown", results: []report.Result{measured("webgpu", report.Partial, "WebGPU on qualcomm adreno-7xx with --enable-unsafe-webgpu --enable-features=Vulkan; without them: no adapter", "Chromium doesn't offer WebGPU by itself here.")}})
	empty := newAuto(run, test("Empty"))
	empty.start()
	empty.update(autoMsg{name: "Empty"})
	return append([]step{running, shown, empty}, heats(run)...)
}

func TestAnAutomaticStepMeasuresShowsAndGoesOnByItself(t *testing.T) {
	run := &report.Run{}
	ran := 0
	a := newAuto(run, deep.Test{Name: "Graphics", Doing: "Asking Vulkan which device it draws on", About: "Programs draw through Vulkan.", Run: func(func(string)) []report.Result {
		ran++
		return []report.Result{measured("gpu", report.Works, "Vulkan draws on Adreno X1-85 (turnip Mesa driver)", "")}
	}})
	if cmd := a.start(); cmd == nil || a.done() {
		t.Fatal("it didn't start by itself")
	}
	if drawn := a.view(90, 30); !strings.Contains(drawn, "Asking Vulkan") || !strings.Contains(a.keys(), "nothing for you to do") {
		t.Errorf("while it runs:\n%s\n%s", drawn, a.keys())
	}
	// Another step's result isn't taken for this one's
	a.update(autoMsg{name: "Browser", results: []report.Result{measured("webgl", report.Broken, "", "")}})
	if a.phase != "running" || len(run.Results) != 0 {
		t.Fatalf("phase %q, results %+v", a.phase, run.Results)
	}
	next := a.update(autoMsg{name: "Graphics", results: []report.Result{measured("gpu", report.Works, "Vulkan draws on Adreno X1-85 (turnip Mesa driver)", "")}})
	result, _ := run.Get("gpu")
	if a.phase != "shown" || next == nil || result.State != report.Works || result.How != report.Measured || a.done() {
		t.Fatalf("phase %q, result %+v", a.phase, result)
	}
	if drawn := a.view(90, 30); !strings.Contains(drawn, "✓ Graphics acceleration: works") || !strings.Contains(drawn, "Adreno X1-85") {
		t.Errorf("the result:\n%s", drawn)
	}
	// It goes on with nobody pressing anything
	a.update(goOnMsg{"Graphics"})
	if !a.done() {
		t.Error("it waits for a key")
	}
}

func TestAnAutomaticStepKeepsAFailureTheQuickCheckFound(t *testing.T) {
	run := &report.Run{}
	run.Set("gpu", report.Broken, report.Measured, "the GPU's firmware didn't load", "")
	run.Set("wifi", report.Works, report.Measured, "wlan0 connected", "")
	a := newAuto(run, deep.Test{Name: "Graphics"})
	a.start()
	a.update(autoMsg{name: "Graphics", results: []report.Result{
		measured("gpu", report.Partial, "Vulkan draws on llvmpipe", "Programs get a renderer that runs on the processor, not the GPU."),
		measured("wifi", report.Works, "connected at 5180 MHz, signal -48 dBm", "")}})
	if gpu, _ := run.Get("gpu"); gpu.State != report.Broken || gpu.Evidence != "the GPU's firmware didn't load" {
		t.Errorf("the failure was softened: %+v", gpu)
	}
	// What the deeper look found as well or worse replaces the quick check's line
	if wifi, _ := run.Get("wifi"); wifi.Evidence != "connected at 5180 MHz, signal -48 dBm" {
		t.Errorf("wifi: %+v", wifi)
	}
}

func TestAnAutomaticStepThatFallsOverReportsNothing(t *testing.T) {
	run := &report.Run{}
	a := newAuto(run, deep.Test{Name: "Video decoding", Run: func(func(string)) []report.Result { panic("no such device") }})
	cmd := a.start()
	// The measuring half of the step's command, run as the program would
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("start gave %T", cmd())
	}
	msg, ok := batch[0]().(autoMsg)
	if !ok || msg.fault != "no such device" {
		t.Fatalf("the fault wasn't caught: %+v", msg)
	}
	a.update(msg)
	if a.phase != "shown" || len(run.Results) != 0 || !strings.Contains(a.view(90, 30), "problem of its own") {
		t.Errorf("phase %q, results %+v:\n%s", a.phase, run.Results, a.view(90, 30))
	}
}

func TestTheAutomaticRoundRunsTheSoundWithoutAKey(t *testing.T) {
	run := &report.Run{}
	dir := t.TempDir()
	sound, camera := newSpeakers(run, dir), newCamera(run, dir)
	graphics := newAuto(run, deep.Test{Name: "Graphics"})
	steps := []step{newKeyboard(run), sound, newHeadphones(run, dir), camera, graphics}
	names := func(steps []step) string {
		var out []string
		for _, s := range steps {
			out = append(out, s.name())
		}
		return strings.Join(out, ", ")
	}
	// The automatic round is the sound and the tests that measure themselves, nothing done with the owner
	if got := names(choose(steps, []string{"auto"})); got != "Sound, Graphics" {
		t.Errorf("auto: %s", got)
	}
	handsOff(steps, []string{"auto"})
	if !sound.handsOff || !graphics.handsOff {
		t.Fatal("the steps weren't told nobody is there")
	}
	if sound.start() == nil || sound.phase != "playing" {
		t.Fatalf("the tones wait for a key: %q", sound.phase)
	}
	sound.phase = "intro"
	played(sound, 900, 800, 700, 700, 700)
	if sound.phase != "result" || sound.done() {
		t.Fatalf("phase %q", sound.phase)
	}
	sound.update(goOnMsg{"Sound"})
	if !sound.done() || run.State("speakers") != report.Works {
		t.Errorf("done %v, speakers %q", sound.done(), run.State("speakers"))
	}
	// Run as one part or in the whole test, the sound waits for its Enter as before
	handsOff(steps, []string{"speakers"})
	again := newSpeakers(run, dir)
	if sound.handsOff || again.start() != nil || again.phase != "intro" {
		t.Errorf("handsOff %v, phase %q", sound.handsOff, again.phase)
	}
	if !Automatic([]string{"graphics", "video"}, false) || !Automatic([]string{"auto"}, false) || Automatic([]string{"graphics", "camera"}, false) || Automatic([]string{"sound"}, false) || Automatic(nil, false) {
		t.Error("which words need nobody is off")
	}
	// The sound test runs for an agent only on the owner's yes, which --play says was given
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if Automatic([]string{"speakers"}, false) || !Automatic([]string{"speakers"}, true) || !Automatic([]string{"microphone", "wifi"}, true) || Automatic([]string{"camera"}, true) {
		t.Error("when the sound test runs without a terminal is off")
	}
	for _, word := range append(AutomaticWords(), "auto") {
		if !KnownStep(word) {
			t.Errorf("%q isn't a step word", word)
		}
	}
}

func TestWithoutATerminalTheAutomaticTestsReportOnlyThemselves(t *testing.T) {
	run := &report.Run{}
	run.Set("display", report.Works, report.Measured, "eDP-1 connected", "")
	var said []string
	tests := []deep.Test{
		{Name: "Graphics", Doing: "Asking Vulkan", Run: func(func(string)) []report.Result {
			return []report.Result{measured("gpu", report.Works, "Vulkan draws on Adreno X1-85", "")}
		}},
		{Name: "Browser", Doing: "Opening a test page", Run: func(func(string)) []report.Result { panic("chromium fell over") }},
		{Name: "Temperatures", Doing: "Keeping every core busy", Run: func(say func(string)) []report.Result {
			say("   60 s  66 °C  working at 100%")
			return []report.Result{measured("fans", report.Works, "the fan sped up", "")}
		}},
	}
	Unattended(run, tests, []string{"graphics"}, func(line string) { said = append(said, line) })
	if len(run.Results) != 1 || run.State("gpu") != report.Works {
		t.Errorf("results %+v", run.Results)
	}
	// One test falling over is said, and the rest still run
	if text := strings.Join(said, "\n"); !strings.Contains(text, "Asking Vulkan") || !strings.Contains(text, "chromium fell over") || strings.Contains(text, "every core") {
		t.Errorf("said:\n%s", text)
	}
	said = nil
	Unattended(run, tests, []string{"auto"}, func(line string) { said = append(said, line) })
	if len(run.Results) != 2 || run.State("fans") != report.Works {
		t.Errorf("auto: %+v", run.Results)
	}
	// A test that takes a while tells what it reads as it goes
	if !strings.Contains(strings.Join(said, "\n"), "66 °C") {
		t.Errorf("the readings aren't said: %q", said)
	}
}

func TestAMenuTooTallForTheWindowScrollsWithTheCursor(t *testing.T) {
	c := choice{question: "What would you like to test?", heads: map[int]string{1: "Runs by itself", 4: "Needs you"}}
	for _, name := range []string{"Everything", "Sound", "Graphics", "Video", "Keyboard", "Touchpad", "Camera", "USB ports", "Sleep", "Quit"} {
		c.options = append(c.options, name)
		c.sides = append(c.sides, side{mark: "◐", text: "19 Nov · There is a picture, but too dark, pale or off in colour.", style: close_})
	}
	whole := ansi.ReplaceAllString(c.view(80), "")
	// The mark in front, the words after in one column, and no numbers
	for _, text := range []string{"› ◐ Everything   19 Nov · There is a picture", "  ◐ USB ports    19 Nov", "Runs by itself", "Needs you"} {
		if !strings.Contains(whole, text) {
			t.Fatalf("the whole menu lacks %q:\n%s", text, whole)
		}
	}
	// A number picks nothing in a menu, where the parts have none to show
	if c.update(press('3')) != -1 || c.cursor != 0 {
		t.Errorf("a number moved the cursor to %d", c.cursor)
	}
	for cursor := range c.options {
		c.cursor = cursor
		drawn := c.viewIn(80, 9)
		lines := strings.Split(drawn, "\n")
		if len(lines) > 9 || !strings.Contains(drawn, "› ") || !strings.Contains(drawn, c.options[cursor]) {
			t.Errorf("with the cursor on %s, %d lines:\n%s", c.options[cursor], len(lines), drawn)
		}
	}
	c.cursor = 0
	if drawn := c.viewIn(80, 9); !strings.Contains(drawn, "↓ more") || strings.Contains(drawn, "↑ more") {
		t.Errorf("at the top:\n%s", drawn)
	}
	c.cursor = len(c.options) - 1
	if drawn := c.viewIn(80, 9); !strings.Contains(drawn, "↑ more") || strings.Contains(drawn, "↓ more") {
		t.Errorf("at the bottom:\n%s", drawn)
	}
	// The words after a part are cut to the window, and left out where there is no room for them, so
	// every part stays on one line, with nothing past the edge
	for _, width := range []int{60, 40, 24} {
		drawn := c.view(width)
		for _, line := range strings.Split(drawn, "\n") {
			if lipgloss.Width(line) > width {
				t.Errorf("a line of %d in %d columns: %q", lipgloss.Width(line), width, line)
			}
		}
		if rows := strings.Count(drawn, "◐"); rows != len(c.options) {
			t.Errorf("at %d columns the parts take %d rows:\n%s", width, rows, drawn)
		}
	}
}

func TestTheStartScreenFitsAShortWindow(t *testing.T) {
	run := &report.Run{Hardware: map[string]string{"maker": "LENOVO", "product": "83ED"}}
	last := &summary{run: run}
	for _, height := range []int{60, 40, 30, 24, 16} {
		start := &welcome{run: run, parts: menu, kept: map[string]report.Kept{}}
		m := &model{run: run, summary: last, failed: map[string]string{}, steps: []step{start, last}, width: 100, height: height}
		drawn := m.View().Content
		if lines := strings.Count(drawn, "\n") + 1; lines > height {
			t.Errorf("in %d rows it draws %d", height, lines)
		}
		// The menu and its cursor are what stays
		if !strings.Contains(drawn, "What would you like to test?") || !strings.Contains(drawn, "› ") || !strings.Contains(drawn, "Everything") {
			t.Errorf("in %d rows the menu is gone:\n%s", height, drawn)
		}
		// Every entry can still be reached
		for range len(menu) + 1 {
			m.Update(press(tea.KeyDown))
		}
		if drawn = m.View().Content; !strings.Contains(drawn, "Quit") || strings.Count(drawn, "\n")+1 > height {
			t.Errorf("in %d rows the last entry can't be reached:\n%s", height, drawn)
		}
	}
}

func TestTheMenuSaysWhenANewerReleaseIsOut(t *testing.T) {
	run := &report.Run{Hardware: map[string]string{"maker": "LENOVO", "product": "83ED"}}
	start := &welcome{run: run, parts: menu, kept: map[string]report.Kept{}, options: Options{Version: "0.12.0"}}
	if strings.Contains(start.view(100, 60), "is out") {
		t.Error("a newer release is announced before anyone said so")
	}
	start.update(releaseMsg{"0.12.1"})
	if drawn := start.view(100, 60); !strings.Contains(drawn, "hw-test 0.12.1 is out, and this is 0.12.0") || !strings.Contains(drawn, "hw-test update") {
		t.Errorf("the newer release isn't said:\n%s", drawn)
	}
	// On the newest release there is nothing to say
	start.update(releaseMsg{""})
	if strings.Contains(start.view(100, 60), "is out") {
		t.Error("the notice stayed after it no longer held")
	}
	// A step named on the command line shows no menu, so nothing is asked from it
	named := &welcome{run: run, only: true, options: Options{Version: "0.12.0"}}
	if named.start() != nil {
		t.Error("a named step asks about releases from the menu it never shows")
	}
}

func TestAMiniPCIsNotAskedToUnplugItsOnlyScreen(t *testing.T) {
	root := t.TempDir()
	set := func(path, text string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
	}
	// No panel of its own, and one monitor on HDMI
	set("sys/class/drm/card0-HDMI-A-1/status", "connected\n")
	set("sys/class/drm/card0-HDMI-A-1/modes", "3840x2160\n")
	set("sys/class/drm/card0-DP-1/status", "disconnected\n")
	sys := machine.Fixture(root)
	sys.Command = func(string, ...string) (string, error) { return "", os.ErrNotExist }
	run := &report.Run{}
	s := newScreen(run, sys)
	s.start()
	if !s.only || s.phase != "picture" || !strings.Contains(s.view(90, 30), "This computer's screen is the one on HDMI-A-1") {
		t.Fatalf("only %v, phase %q:\n%s", s.only, s.phase, s.view(90, 30))
	}
	s.update(press('1'))
	result, _ := run.Get("external-display")
	if !s.done() || result.State != report.Works || strings.Contains(result.Evidence, "came back") {
		t.Errorf("done %v, result %+v", s.done(), result)
	}

	// With a second monitor there is one to read on while the other is unplugged
	set("sys/class/drm/card0-DP-1/status", "connected\n")
	s = newScreen(&report.Run{}, sys)
	s.start()
	if s.only {
		t.Error("two monitors counted as one")
	}

	// And its questions don't speak of a laptop's keyboard or touchpad
	q := newQuestions(&report.Run{}, probe.Facts{Encrypted: true}, map[string]bool{"usb": true, "external-display": true})
	for _, asked := range q.list {
		if strings.Contains(asked.text, "laptop") || strings.Contains(asked.text, "touchpad") || strings.Contains(asked.text, "built-in") {
			t.Errorf("a mini PC is asked %q", asked.text)
		}
	}
	// The disk password, the installer and what the owner changed
	if len(q.list) != 3 {
		t.Errorf("asked %d questions", len(q.list))
	}
}
