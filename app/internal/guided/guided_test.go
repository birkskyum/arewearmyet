package guided

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

func press(code rune) tea.KeyPressMsg {
	text := ""
	if code >= ' ' && code < 0x7f {
		text = string(code)
	}
	return tea.KeyPressMsg{Code: code, Text: text}
}

// typeAll presses every key the keyboard step waits for.
func typeAll(k *keyboard, except string) {
	for _, row := range layout {
		for _, key := range row {
			if key.needed && !strings.Contains(except, key.label) {
				k.update(press(key.code))
			}
		}
	}
}

func TestKeyboardIsMeasuredOnceEveryNeededKeyWasSeen(t *testing.T) {
	run := &report.Run{}
	k := newKeyboard(run)
	typeAll(k, "")
	if k.done() || len(k.unlit()) != 0 {
		t.Fatalf("done=%v, unlit=%v", k.done(), k.unlit())
	}
	k.update(press(tea.KeyEnter))
	result, _ := run.Get("keyboard")
	if !k.done() || result.State != report.Works || result.How != report.Measured {
		t.Errorf("done=%v, result=%+v", k.done(), result)
	}
}

func TestKeyboardLightsKeysByPlaceWhateverTheLayout(t *testing.T) {
	k := newKeyboard(&report.Run{})
	// A Danish keyboard types æ where a US one has the semicolon, and the terminal says so
	k.update(tea.KeyPressMsg{Code: 'æ', BaseCode: ';', Text: "æ"})
	k.update(tea.KeyPressMsg{Code: 'A', Text: "A"})
	if !k.lit[';'] || !k.lit['a'] {
		t.Errorf("lit: %v", k.lit)
	}
	// A key that isn't in the picture still counts as seen
	k.update(tea.KeyPressMsg{Code: 'ø', Text: "ø"})
	if k.count() != 3 {
		t.Errorf("count %d", k.count())
	}
}

func TestKeyboardAsksAboutKeysThatNeverLit(t *testing.T) {
	run := &report.Run{}
	k := newKeyboard(run)
	typeAll(k, "Q 7")
	for i := 0; i < 3; i++ {
		k.update(press(tea.KeySpace))
	}
	if k.missing == nil || !strings.Contains(k.missing.question, "7 Q") {
		t.Fatalf("missing: %+v", k.missing)
	}
	if k.wantsAllKeys() {
		t.Error("the question still takes every key")
	}
	k.update(press('1')) // "I pressed them and they don't respond"
	result, _ := run.Get("keyboard")
	if !k.done() || result.State != report.Partial || result.Note != "No response from: 7, Q" {
		t.Errorf("result=%+v", result)
	}

	// Going back keeps what was lit
	k = newKeyboard(&report.Run{})
	typeAll(k, "Q")
	for i := 0; i < 3; i++ {
		k.update(press(tea.KeyEscape))
	}
	k.update(press('3'))
	if k.missing != nil || k.done() || len(k.unlit()) != 1 {
		t.Errorf("after going back: missing=%v done=%v unlit=%v", k.missing, k.done(), k.unlit())
	}
}

func TestChoiceMovesAndPicks(t *testing.T) {
	c := choice{options: []string{"a", "b", "c"}}
	if c.update(press(tea.KeyUp)) != -1 || c.cursor != 2 {
		t.Errorf("up from the top: cursor %d", c.cursor)
	}
	c.update(press(tea.KeyDown))
	c.update(press(tea.KeyDown))
	if picked := c.update(press(tea.KeyEnter)); picked != 1 {
		t.Errorf("picked %d", picked)
	}
	if picked := c.update(press('3')); picked != 2 {
		t.Errorf("by number: %d", picked)
	}
	if picked := c.update(press('9')); picked != -1 {
		t.Errorf("a number past the list picked %d", picked)
	}
}

func TestQuestionsAskOnlyWhatIsThereAndUnmeasured(t *testing.T) {
	run := &report.Run{}
	run.Set("charging", report.Works, report.Measured, "PD", "")
	run.Set("bluetooth", report.Works, report.Measured, "hci0 powered", "")
	// The touchpad and the USB ports have steps of their own here; sound has none, as on a
	// machine without PipeWire's tools, so it is asked about
	covered := map[string]bool{"touchpad": true, "usb": true}
	q := newQuestions(run, probe.Facts{Laptop: true, Touchpad: "pad", SoundCard: "card", Bluetooth: "hci0"}, covered)
	var checks []string
	for _, question := range q.list {
		checks = append(checks, question.check)
	}
	if got := strings.Join(checks, " "); got != "speakers microphones external-display installer" {
		t.Errorf("asked: %s", got)
	}

	q.update(press('1')) // speakers: yes
	q.update(press('2')) // microphone: partly, then a note
	for _, c := range "very quiet" {
		q.update(press(c))
	}
	q.update(press(tea.KeyEnter))
	q.update(press('4')) // external screen: haven't tried
	q.update(press('3')) // installer: no
	q.update(press(tea.KeyEnter))
	if !q.done() {
		t.Fatal("not done")
	}
	want := map[string]string{"speakers": report.Works, "microphones": report.Partial, "external-display": "", "installer": report.Broken}
	for check, state := range want {
		if run.State(check) != state {
			t.Errorf("%s: %q, want %q", check, run.State(check), state)
		}
	}
	if microphones, _ := run.Get("microphones"); microphones.Note != "very quiet" || microphones.How != report.Owner {
		t.Errorf("microphones: %+v", microphones)
	}
}

func TestEffectsTurnWhatWasSeenIntoResults(t *testing.T) {
	outcomes := func(list ...string) *effects {
		run := &report.Run{}
		run.Set("keyboard", report.Works, report.Measured, "60 keys registered", "")
		e := &effects{run: run, backlight: "/sys/class/backlight/x", low: 800, high: 2047}
		for i, label := range []string{"Volume up", "Volume down", "Mute", "Mute again, to turn the sound back on", "Brightness down", "Brightness up"} {
			e.items = append(e.items, effect{label: label, outcome: list[i]})
		}
		e.finish()
		return e
	}
	all := outcomes("seen", "seen", "seen", "seen", "seen", "seen")
	if all.run.State("backlight") != report.Works || all.run.State("keyboard") != report.Works {
		t.Errorf("all seen: %+v", all.run.Results)
	}
	some := outcomes("failed", "seen", "failed", "skipped", "seen", "failed")
	keyboard, _ := some.run.Get("keyboard")
	if some.run.State("backlight") != report.Partial || keyboard.State != report.Partial || keyboard.Note != "No effect from: volume up, mute" || keyboard.Evidence != "60 keys registered" {
		t.Errorf("some failed: backlight %q, keyboard %+v", some.run.State("backlight"), keyboard)
	}
	skipped := outcomes("skipped", "skipped", "skipped", "skipped", "skipped", "skipped")
	if skipped.run.State("backlight") != "" || skipped.run.State("keyboard") != report.Works {
		t.Errorf("skipped: %+v", skipped.run.Results)
	}
}

func TestTheTestSkipsStepsWithNothingToDoAndDrawsAtAnySize(t *testing.T) {
	run := &report.Run{Hardware: map[string]string{"maker": "LENOVO", "family": "Yoga Slim 7 14Q8X9"}}
	run.Set("display", report.Works, report.Measured, "eDP-1 connected, 2944x1840", "")
	last := &summary{run: run, options: Options{Send: false}}
	// No volume control and no backlight: the effects step has nothing to ask
	m := &model{run: run, summary: last, failed: map[string]string{}, steps: []step{&welcome{run: run}, &effects{run: run}, newQuestions(run, probe.Facts{}, nil), last}}
	m.Update(press(tea.KeyEnter))
	if m.at != 2 {
		t.Fatalf("at step %d, want the questions", m.at)
	}
	for _, size := range [][2]int{{120, 40}, {60, 20}, {20, 8}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if !strings.Contains(m.View().Content, "USB") {
			t.Errorf("at %v the question is missing", size)
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	for range 3 { // USB ports, an external screen, the installer: haven't tried
		m.Update(press('4'))
	}
	if m.at != 3 || !strings.Contains(m.View().Content, "Nothing is sent") {
		t.Fatalf("at step %d: %s", m.at, m.View().Content)
	}
	_, cmd := m.Update(press(tea.KeyEnter))
	if cmd == nil || last.farewell() != "Not sent." {
		t.Errorf("farewell %q", last.farewell())
	}
}

func TestCameraCountsDownBeforeItTakesAPicture(t *testing.T) {
	c := newCamera(&report.Run{}, t.TempDir())
	if c.start() != nil || c.phase != "intro" {
		t.Fatalf("it started at %q", c.phase)
	}
	if !strings.Contains(c.view(80, 30), "Nothing is taken until you press Enter") || !strings.Contains(c.view(80, 30), "never leaves this computer") {
		t.Errorf("the intro doesn't say what will happen:\n%s", c.view(80, 30))
	}
	c.update(press(tea.KeyEnter))
	for want := 5; want >= 1; want-- {
		if c.phase != "countdown" || c.count != want || !strings.Contains(c.view(80, 30), "Taking a picture in") {
			t.Fatalf("at %d: phase %q, count %d", want, c.phase, c.count)
		}
		c.update(countMsg{})
	}
	if c.phase != "capturing" {
		t.Errorf("after the countdown it is %q", c.phase)
	}

	// Skipping during the countdown takes no picture
	c = newCamera(&report.Run{}, t.TempDir())
	c.update(press(tea.KeyEnter))
	c.update(press('s'))
	if !c.done() || c.update(countMsg{}) != nil {
		t.Error("skipping didn't stop the countdown")
	}
}

// faulty is a step whose own code fails, as any step might on a machine nobody has seen.
type faulty struct{ in string }

func (f faulty) name() string   { return "Faulty" }
func (f faulty) start() tea.Cmd { return nil }
func (f faulty) keys() string   { return "" }
func (f faulty) done() bool     { return false }
func (f faulty) update(tea.Msg) tea.Cmd {
	if f.in == "update" {
		var none map[string]int
		none["x"] = 1
	}
	return nil
}
func (f faulty) view(width, height int) string {
	if f.in == "view" {
		return []string{}[3]
	}
	return "fine"
}

func TestAStepThatFailsIsPassedOverAndTheTestGoesOn(t *testing.T) {
	for _, in := range []string{"update", "view"} {
		run := &report.Run{}
		last := &summary{run: run, options: Options{Send: false}}
		m := &model{run: run, summary: last, failed: map[string]string{}, width: 100, height: 40}
		last.failed = m.failed
		m.steps = []step{&welcome{run: run}, faulty{in: in}, newQuestions(run, probe.Facts{}, map[string]bool{"usb": true, "external-display": true}), last}
		m.Update(press(tea.KeyEnter))
		m.View()
		m.Update(press('x'))
		if m.at != 2 || m.failed["Faulty"] == "" {
			t.Fatalf("a fault in %s: at step %d, failed %v", in, m.at, m.failed)
		}
		// The owner goes on answering, and the summary says what was passed over
		m.Update(press('1'))
		if run.State("installer") != report.Works || m.at != 3 {
			t.Fatalf("after the fault: step %d, results %+v", m.at, run.Results)
		}
		if drawn := m.View().Content; !strings.Contains(drawn, "Faulty step ran into a problem") {
			t.Errorf("the summary doesn't mention the step:\n%s", drawn)
		}
	}
}

func TestTheTestIsSavedAsEachStepBegins(t *testing.T) {
	run := &report.Run{}
	var saved []string
	last := &summary{run: run}
	m := &model{run: run, summary: last, failed: map[string]string{}, save: func(step string) { saved = append(saved, step) }}
	m.steps = []step{&welcome{run: run}, newTouchscreen(run, "screen"), newQuestions(run, probe.Facts{}, map[string]bool{"usb": true, "external-display": true}), last}
	m.Update(press(tea.KeyEnter))
	m.Update(press('s'))
	if got := strings.Join(saved, ", "); got != "Touchscreen, Questions" {
		t.Errorf("saved at: %s", got)
	}
}

func TestAStoppedTestGoesOnFromItsStepAndACrashIsThatStepsResult(t *testing.T) {
	dir := t.TempDir()
	build := func(run *report.Run) []step {
		return []step{newKeyboard(run), newCamera(run, dir), newSleep(run, probe.Facts{}), newQuestions(run, probe.Facts{}, nil)}
	}
	saved := report.Pending{Boot: "an-earlier-boot", Step: "Camera", Results: []report.Result{{Check: "keyboard", State: report.Works, How: report.Measured, Evidence: "60 keys"}}}

	// The computer restarted, and the owner says it went down by itself
	run := &report.Run{}
	opening, steps := resume(run, saved, "this-boot", &welcome{run: run}, build(run))
	back, ok := opening.(*resumed)
	if !ok || !back.restarted || len(steps) != 3 || run.State("keyboard") != report.Works {
		t.Fatalf("resumed=%v, %d steps, results %+v", ok, len(steps), run.Results)
	}
	back.update(press('1'))
	camera, _ := run.Get("camera")
	if camera.State != report.Broken || !strings.Contains(camera.Evidence, "restarted by itself during the test's Camera step") {
		t.Errorf("camera: %+v", camera)
	}
	if !steps[0].done() || steps[0].start() != nil {
		t.Error("the step it crashed in would run again")
	}

	// The owner restarted it themselves: no blame, and the step runs again
	run = &report.Run{}
	opening, steps = resume(run, saved, "this-boot", &welcome{run: run}, build(run))
	opening.update(press('2'))
	if run.State("camera") != "" || steps[0].done() {
		t.Errorf("camera %q, step done %v", run.State("camera"), steps[0].done())
	}

	// A saved step this run doesn't have leaves the run as it is
	run = &report.Run{}
	opening, steps = resume(run, report.Pending{Step: "USB ports"}, "this-boot", &welcome{run: run}, build(run))
	if _, isWelcome := opening.(*welcome); !isWelcome || len(steps) != 4 || len(run.Results) != 0 {
		t.Error("a test for another set of steps was resumed")
	}
}

func TestChosenStepsRunAlone(t *testing.T) {
	run := &report.Run{}
	dir := t.TempDir()
	all := []step{newKeyboard(run), &effects{run: run}, newSpeakers(run, dir), newMicrophones(run, dir), newHeadphones(run, dir), newCamera(run, dir), newQuestions(run, probe.Facts{}, nil)}
	names := func(steps []step) string {
		var out []string
		for _, s := range steps {
			out = append(out, s.name())
		}
		return strings.Join(out, ", ")
	}
	if got := names(choose(all, []string{"sound"})); got != "Speakers, Microphone, Headphones" {
		t.Errorf("sound: %s", got)
	}
	if got := names(choose(all, []string{"camera", "keyboard"})); got != "Keyboard, Volume and brightness keys, Camera" {
		t.Errorf("camera and keyboard: %s", got)
	}
	if len(choose(all, []string{"usb"})) != 0 || !KnownStep("usb") || KnownStep("nonsense") {
		t.Error("step words are off")
	}
}

func TestUSBPlugInsAreSeenWithTheirSpeed(t *testing.T) {
	root := t.TempDir()
	device := func(name, speed, product string) {
		dir := filepath.Join(root, "sys/bus/usb/devices", name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "speed"), []byte(speed+"\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "idVendor"), []byte("0781\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "idProduct"), []byte(product+"\n"), 0o644)
	}
	unplug := func(name string) { os.RemoveAll(filepath.Join(root, "sys/bus/usb/devices", name)) }
	device("usb1", "480", "0002")
	device("1-4", "480", "9999") // the built-in camera or fingerprint reader, there from the start
	os.MkdirAll(filepath.Join(root, "sys/bus/usb/devices/1-4:1.0"), 0o755)

	run := &report.Run{}
	u := newUSB(run, machine.Fixture(root))
	u.start()
	seen := func() { u.update(usbMsg{u.devices()}) }
	seen()
	if len(u.plugs) != 0 {
		t.Fatalf("what was already there counted as a plug-in: %+v", u.plugs)
	}
	device("2-1", "5000", "5581")
	seen()
	unplug("2-1")
	seen()
	// A hub comes with the devices on it; it is one plug-in
	device("1-2", "480", "0608")
	device("1-2.1", "480", "c52b")
	seen()
	if len(u.plugs) != 2 || u.plugs[0].speed != 5000 || u.slowed() {
		t.Fatalf("plugs %+v, slowed %v", u.plugs, u.slowed())
	}
	u.update(press(tea.KeyEnter))
	result, _ := run.Get("usb")
	if result.State != report.Works || result.How != report.Measured || !strings.Contains(result.Evidence, "2-1 at 5 Gb/s") || strings.Contains(result.Evidence, "0781") {
		t.Errorf("result %+v", result)
	}

	// The same stick at full speed one way round and USB 2 speed the other
	run = &report.Run{}
	u = newUSB(run, machine.Fixture(root))
	u.start()
	device("2-2", "5000", "5581")
	u.update(usbMsg{u.devices()})
	unplug("2-2")
	u.update(usbMsg{u.devices()})
	device("1-3", "480", "5581")
	u.update(usbMsg{u.devices()})
	u.update(press(tea.KeyEnter))
	if result, _ := run.Get("usb"); result.State != report.Partial || !strings.Contains(result.Note, "one orientation") {
		t.Errorf("one orientation only: %+v", result)
	}

	// Plugged in, and nothing ever appeared
	run = &report.Run{}
	u = newUSB(run, machine.Fixture(root))
	u.start()
	u.update(press('n'))
	u.update(press(tea.KeyEnter))
	if run.State("usb") != report.Broken {
		t.Errorf("nothing appeared: %q", run.State("usb"))
	}
}

func TestAnExternalScreenIsReadWhenItIsPluggedIn(t *testing.T) {
	root := t.TempDir()
	set := func(path, text string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
	}
	set("sys/class/drm/card0-eDP-1/status", "connected\n")
	set("sys/class/drm/card0-DP-2/status", "disconnected\n")
	sys := machine.Fixture(root)
	sys.Command = func(name string, args ...string) (string, error) {
		if name == "hyprctl" {
			return `[{"name":"eDP-1","width":2944,"height":1840,"refreshRate":90.0},{"name":"DP-2","width":3840,"height":2160,"refreshRate":59.997}]`, nil
		}
		return "", os.ErrNotExist
	}
	run := &report.Run{}
	s := newScreen(run, sys)
	s.start()
	s.update(screenMsg{s.external()})
	if s.phase != "waiting" {
		t.Fatalf("the built-in screen counted: %q", s.phase)
	}
	// Plugged into a USB-C port
	set("sys/class/drm/card0-DP-2/status", "connected\n")
	os.MkdirAll(filepath.Join(root, "sys/class/typec/port0-partner"), 0o755)
	s.update(screenMsg{s.external()})
	if s.phase != "picture" || s.details != "DP-2 (DisplayPort over USB-C), 3840x2160 at 60 Hz" {
		t.Fatalf("phase %q, details %q", s.phase, s.details)
	}
	s.update(press('1'))
	set("sys/class/drm/card0-DP-2/status", "disconnected\n")
	s.update(screenMsg{s.external()})
	set("sys/class/drm/card0-DP-2/status", "connected\n")
	s.update(screenMsg{s.external()})
	result, _ := run.Get("external-display")
	if !s.done() || result.State != report.Works || !strings.Contains(result.Evidence, "DP-2 (DisplayPort over USB-C), 3840x2160 at 60 Hz; it came back") {
		t.Errorf("result %+v", result)
	}

	// It never comes back after unplugging
	run = &report.Run{}
	s = newScreen(run, sys)
	s.start() // already connected
	s.update(press('1'))
	set("sys/class/drm/card0-DP-2/status", "disconnected\n")
	s.update(screenMsg{s.external()})
	s.update(press('n'))
	if result, _ := run.Get("external-display"); result.State != report.Partial {
		t.Errorf("no hotplug: %+v", result)
	}
}

func TestTouchpadIsMeasuredFromThePointerInTheTerminal(t *testing.T) {
	run := &report.Run{}
	pad := newTouchpad(run, "hid-over-i2c 0488:1054 Touchpad")
	if !pad.wantsMouse() {
		t.Fatal("it doesn't ask for the pointer")
	}
	for x := 0; x < 8; x++ {
		pad.update(tea.MouseMotionMsg{X: x, Y: 3})
	}
	pad.update(tea.MouseClickMsg{Button: tea.MouseLeft})
	pad.update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	pad.update(press(tea.KeyEnter))
	if pad.done() {
		t.Fatal("done with only one scroll direction")
	}
	// Tried, and scrolling the other way never registered
	pad.update(press('n'))
	result, _ := run.Get("touchpad")
	if !pad.done() || result.State != report.Partial || result.Note != "No response to: scrolling the other way" {
		t.Errorf("result %+v", result)
	}

	run = &report.Run{}
	pad = newTouchpad(run, "pad")
	for x := 0; x < 8; x++ {
		pad.update(tea.MouseMotionMsg{X: x, Y: 3})
	}
	pad.update(tea.MouseClickMsg{Button: tea.MouseLeft})
	pad.update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	pad.update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	pad.update(press(tea.KeyEnter))
	if result, _ := run.Get("touchpad"); !pad.done() || result.State != report.Works || result.How != report.Measured {
		t.Errorf("all seen: %+v", result)
	}

	// Nothing seen could be the terminal's doing, so the owner is asked about the desktop
	run = &report.Run{}
	pad = newTouchpad(run, "pad")
	pad.update(press('n'))
	if pad.done() || pad.nothing == nil || pad.wantsMouse() {
		t.Fatal("it didn't ask about the desktop")
	}
	pad.update(press('1'))
	if result, _ := run.Get("touchpad"); result.State != report.Works || result.How != report.Owner {
		t.Errorf("nothing seen, works on the desktop: %+v", result)
	}
}

func TestTouchscreenCountsTapsInDifferentPlaces(t *testing.T) {
	run := &report.Run{}
	screen := newTouchscreen(run, "hid-over-i2c 27C6:0123")
	screen.update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: 4})
	screen.update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: 4}) // the same place twice counts once
	screen.update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 30, Y: 9})
	screen.update(press(tea.KeyEnter))
	if screen.done() {
		t.Fatal("done after two places")
	}
	screen.update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 60, Y: 2})
	screen.update(press(tea.KeyEnter))
	if result, _ := run.Get("touchscreen"); !screen.done() || result.State != report.Works || result.How != report.Measured {
		t.Errorf("result %+v", result)
	}

	run = &report.Run{}
	screen = newTouchscreen(run, "x")
	screen.update(press('a'))
	if run.State("touchscreen") != report.Absent {
		t.Error("no touchscreen wasn't recorded")
	}
}

func TestHeadphonesAskOnlyAfterPlaying(t *testing.T) {
	run := &report.Run{}
	jack := newHeadphones(run, t.TempDir())
	jack.update(press('1'))
	if jack.done() || jack.phase != "intro" {
		t.Fatal("an answer was taken before anything played")
	}
	jack.update(playedMsg{})
	jack.update(press('3'))
	if result, _ := run.Get("headphones"); !jack.done() || result.State != report.Broken || result.Note != "With headphones plugged in, the sound stays on the speakers." {
		t.Errorf("result %+v", result)
	}
	skipped := newHeadphones(&report.Run{}, t.TempDir())
	skipped.update(press('s'))
	if !skipped.done() || len(skipped.run.Results) != 0 {
		t.Error("skipping left a result")
	}
}

// A theme's "bright black" can be nearly its background, so nothing that's read may use it,
// nor the faint attribute.
func TestNothingIsDrawnInAFaintColour(t *testing.T) {
	run := &report.Run{Hardware: map[string]string{"maker": "LENOVO", "product": "83ED"}}
	run.Set("wifi", report.Works, report.Measured, "wlan0 connected", "")
	run.Set("hibernate", report.Broken, report.Measured, "the kernel offers no hibernation", "")
	dir := t.TempDir()
	last := &summary{run: run, options: Options{Send: true}}
	last.start()
	sys := machine.Fixture(dir)
	back := &resumed{run: run, pending: report.Pending{Step: "Camera"}, restarted: true}
	back.ask = back.question()
	steps := []step{&welcome{run: run}, back, newKeyboard(run), &effects{run: run, items: []effect{{label: "Volume up"}}}, newTouchpad(run, "pad"), newTouchscreen(run, "screen"),
		newSpeakers(run, dir), newMicrophones(run, dir), newHeadphones(run, dir), newCamera(run, dir), newUSB(run, sys), newScreen(run, sys), newSleep(run, probe.Facts{}),
		newQuestions(run, probe.Facts{Laptop: true}, nil), last}
	for at, s := range steps {
		m := &model{run: run, summary: last, failed: map[string]string{}, steps: steps, at: at, width: 100, height: 40}
		drawn := m.View().Content
		for _, code := range []string{"\x1b[90m", "\x1b[2m", ";90m", "\x1b[97m"} {
			if strings.Contains(drawn, code) {
				t.Errorf("%s is drawn with %q", s.name(), code)
			}
		}
	}
}
