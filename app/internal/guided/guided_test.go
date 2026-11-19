package guided

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/birkskyum/arewearmyet/app/internal/audio"
	"github.com/birkskyum/arewearmyet/app/internal/deep"
	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/picture"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

// ansi matches the colour and style codes, to read a drawn screen as its text.
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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
	k.update(press('2')) // "I pressed them and they don't respond"
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
	// A skipped key isn't called working: the keyboard's result says which weren't checked
	if keyboard, _ = skipped.run.Get("keyboard"); keyboard.Note != "Not checked: volume up, volume down, mute" || keyboard.How != report.Measured {
		t.Errorf("skipped keys aren't named: %+v", keyboard)
	}
	one := outcomes("skipped", "seen", "failed", "seen", "seen", "seen")
	if keyboard, _ = one.run.Get("keyboard"); keyboard.State != report.Partial || keyboard.Note != "No effect from: mute. Not checked: volume up" {
		t.Errorf("one skipped, one failed: %+v", keyboard)
	}
	// With the keyboard itself untested, a dead key still counts and a skipped one says nothing
	alone := &effects{run: &report.Run{}, items: []effect{{label: "Volume up", outcome: "skipped"}, {label: "Mute", outcome: "failed"}}}
	alone.finish()
	if keyboard, _ = alone.run.Get("keyboard"); keyboard.State != report.Partial || keyboard.Note != "No effect from: mute. Not checked: volume up" {
		t.Errorf("the keys alone: %+v", keyboard)
	}
	only := &effects{run: &report.Run{}, items: []effect{{label: "Volume up", outcome: "skipped"}}}
	only.finish()
	if _, tested := only.run.Get("keyboard"); tested {
		t.Error("a skipped key made a keyboard result out of nothing")
	}
}

func TestTheVolumeKeysAreTestedFromHalfVolumeAndItIsPutBack(t *testing.T) {
	// At full volume the volume up key does nothing, so the step makes room first
	volume, restored := 1.0, 0
	e := &effects{run: &report.Run{}, items: []effect{
		{label: "Volume up", happened: func(b, n levels) bool { return n.volume > b.volume }},
		{label: "Volume down", happened: func(b, n levels) bool { return n.volume < b.volume }}}}
	e.halve = func() func() {
		before := volume
		volume = 0.5
		return func() { volume = before; restored++ }
	}
	e.start()
	if volume != 0.5 || e.restore == nil {
		t.Fatalf("the volume wasn't set to half: %v", volume)
	}
	// Coming back to the step doesn't take half as the level to put back
	e.start()
	if !strings.Contains(e.view(80, 20), "goes back to where it was afterwards") {
		t.Errorf("the step doesn't say the volume was changed:\n%s", e.view(80, 20))
	}
	e.before = levels{volume: 0.5}
	e.update(levelsMsg(levels{volume: 0.55}))
	e.update(levelsMsg(levels{volume: 0.5}))
	if !e.done() || e.items[0].outcome != "seen" || e.items[1].outcome != "seen" {
		t.Fatalf("the keys weren't seen: %+v", e.items)
	}
	if volume != 1.0 || restored != 1 {
		t.Errorf("the volume wasn't put back once: %v, %d times", volume, restored)
	}
	// Ending the step early puts it back too
	early := &effects{run: &report.Run{}, items: []effect{{label: "Volume up", happened: func(b, n levels) bool { return false }}}}
	early.halve = e.halve
	early.start()
	early.cleanup()
	early.cleanup()
	if volume != 1.0 || restored != 2 {
		t.Errorf("ending early: %v, %d times", volume, restored)
	}
	// Without volume keys to ask for, the volume is left alone
	bright := &effects{run: &report.Run{}, items: []effect{{label: "Brightness down", happened: func(b, n levels) bool { return false }}}}
	bright.halve = func() func() { t.Error("the volume was changed for the brightness keys"); return func() {} }
	bright.start()
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

// scene is a camera frame with a picture in it.
func scene() picture.Frame {
	frame := picture.Frame{Width: 64, Height: 36, Pixels: make([]byte, 64*36*3)}
	for i := range frame.Pixels {
		frame.Pixels[i] = byte(i * 7)
	}
	return frame
}

func TestCameraTakesItsPictureOnlyWhenToldAndShowsItInTheTerminal(t *testing.T) {
	dir := t.TempDir()
	run := &report.Run{}
	c := newCamera(run, dir)
	c.canOpen = true
	window := &tools.Viewer{Closed: make(chan struct{})}
	opened := ""
	c.open = func(file string) *tools.Viewer {
		opened = file
		return window
	}
	if c.start() != nil || c.phase != "intro" {
		t.Fatalf("it started at %q", c.phase)
	}
	if intro := c.view(80, 30); !strings.Contains(intro, "When you press Enter it takes one picture and shows it here") || !strings.Contains(intro, "never leaves this computer") {
		t.Errorf("the intro doesn't say what will happen:\n%s", intro)
	}
	// Enter takes it at once: the intro was the warning
	if c.update(press(tea.KeyEnter)) == nil || c.phase != "capturing" {
		t.Fatalf("after Enter it is %q", c.phase)
	}
	if waiting := c.view(80, 30); !strings.Contains(waiting, "adjusted to the light") {
		t.Errorf("the wait isn't explained:\n%s", waiting)
	}
	// The picture is drawn here with the question. No window opens and nothing is written
	c.update(frameMsg{frame: scene(), took: 8 * time.Second, settled: true})
	// Under the picture, before the question, it says the picture goes nowhere
	if view := c.view(80, 40); !strings.Contains(view, "This picture stays on this computer. It is not sent anywhere") ||
		strings.Index(view, "This picture stays") > strings.Index(view, "Is that a real picture") || strings.Index(view, "This picture stays") < strings.LastIndex(view, "▀") {
		t.Errorf("where the picture goes isn't said under it:\n%s", view)
	}
	if c.phase != "ask" || opened != "" || !strings.Contains(c.view(80, 40), "▀") || !strings.Contains(c.view(80, 40), "Is that a real picture") {
		t.Fatalf("phase %q, opened %q:\n%s", c.phase, opened, c.view(80, 40))
	}
	if _, err := os.Stat(filepath.Join(dir, "camera.png")); err == nil {
		t.Error("the picture was written without being asked for")
	}
	// o opens it at full size, and the question stays
	if !strings.Contains(c.keys(), "o to see it at full size") {
		t.Errorf("keys: %s", c.keys())
	}
	c.update(press('o'))
	if c.phase != "ask" || opened != filepath.Join(dir, "camera.png") {
		t.Fatalf("after o: phase %q, opened %q", c.phase, opened)
	}
	if _, err := os.Stat(opened); err != nil {
		t.Fatal("the picture wasn't written for the viewer")
	}
	c.update(press('2'))
	result, _ := run.Get("camera")
	if !c.done() || result.State != report.Partial || !strings.Contains(result.Evidence, "a 64x36 frame") || !strings.HasSuffix(result.Evidence, ", after 8 s of adjusting to the light") {
		t.Errorf("result %+v", result)
	}
	// Nothing of the picture is left behind
	if _, err := os.Stat(opened); err == nil {
		t.Error("the picture is still on disk after the step")
	}

	// Skipping takes no picture
	c = newCamera(&report.Run{}, dir)
	c.update(press('s'))
	if !c.done() {
		t.Error("skipping didn't end the step")
	}

	// A camera that was still getting brighter when time ran out says so, and a quick one says nothing of it
	c = newCamera(&report.Run{}, dir)
	c.update(press(tea.KeyEnter))
	c.update(frameMsg{frame: scene(), took: 15 * time.Second, settled: false})
	if !strings.HasSuffix(c.evidence(), ", still adjusting to the light after 15 s") {
		t.Errorf("evidence: %s", c.evidence())
	}
	c.update(frameMsg{frame: scene(), took: 1200 * time.Millisecond, settled: true})
	if !strings.HasSuffix(c.evidence(), "contrast "+fmt.Sprint(func() int { _, contrast := scene().Brightness(); return contrast }())) {
		t.Errorf("evidence: %s", c.evidence())
	}

	// In a terminal that draws pictures itself, the picture is handed to it and drawn sharp in
	// its cells, replaced by another on "Take another", and dropped with the answer
	sharp := &report.Run{}
	c = newCamera(sharp, dir)
	c.inTerminal = true
	c.view(100, 40)
	c.update(press(tea.KeyEnter))
	handed := c.update(frameMsg{frame: scene(), took: time.Second, settled: true})
	raw, isRaw := tea.Msg(nil), false
	if handed != nil {
		raw = handed()
		_, isRaw = raw.(tea.RawMsg)
	}
	if !isRaw || !strings.HasPrefix(fmt.Sprint(raw.(tea.RawMsg).Msg), "\x1b_Ga=T,U=1,f=100,q=2,i=77,") {
		t.Fatalf("the picture wasn't handed to the terminal: %v", raw)
	}
	if view := c.view(100, 40); !strings.Contains(view, "\U0010EEEE") || strings.Contains(view, "▀") {
		t.Errorf("the terminal's own picture isn't in the view:\n%s", view)
	}
	// In a window now narrower than the picture, the blocks stand in
	if view := c.view(c.columns-2, 40); strings.Contains(view, "\U0010EEEE") || !strings.Contains(view, "▀") {
		t.Error("a window narrower than the picture still shows the terminal's own")
	}
	dropped := c.update(press('1'))
	if !c.done() || dropped == nil || fmt.Sprint(dropped().(tea.RawMsg).Msg) != picture.Forget(pictureID) || sharp.State("camera") != report.Works {
		t.Errorf("after the answer: done %v, dropped %v, state %q", c.done(), dropped != nil, sharp.State("camera"))
	}
	// Elsewhere nothing is handed over
	c = newCamera(&report.Run{}, dir)
	c.inTerminal = false
	c.view(100, 40)
	c.update(press(tea.KeyEnter))
	if c.update(frameMsg{frame: scene(), settled: true}) != nil || c.update(press('1')) != nil {
		t.Error("a terminal that can't draw pictures was sent one")
	}

	// Another program holding the camera is named as that, and nothing is reported for it
	busy := &report.Run{}
	c = newCamera(busy, dir)
	c.update(press(tea.KeyEnter))
	c.update(frameMsg{err: tools.ErrCameraBusy})
	if view := c.view(80, 30); !strings.Contains(view, "Another program is using the camera") || strings.Contains(view, "Measured") || strings.Contains(c.keys(), "full size") {
		t.Errorf("a busy camera:\n%s", view)
	}
	if c.update(press('1')) == nil || c.phase != "capturing" || len(busy.Results) != 0 {
		t.Errorf("trying again: phase %q, results %v", c.phase, busy.Results)
	}
}

func TestTheFanTestIsDoneWithTheOwnerWhereOnlyTheirEarsCanTell(t *testing.T) {
	dir := t.TempDir()
	run := &report.Run{}
	names := func(steps []step) string {
		var out []string
		for _, s := range steps {
			out = append(out, s.name())
		}
		return strings.Join(out, ", ")
	}
	// No fan speed sensor: the step is one of those done with the owner, and asks for their ear
	ear := newHeat(run, machine.Fixture(dir), dir)
	ear.byEar = true
	all := []step{newKeyboard(run), ear, newQuestions(run, probe.Facts{}, nil), newAuto(run, deep.Test{Name: "Video decoding"})}
	if got := names(choose(all, []string{"manual"})); got != "Keyboard, Temperatures and fan, Questions" {
		t.Errorf("done with the owner: %s", got)
	}
	if got := names(choose(all, []string{"auto"})); got != "Video decoding" {
		t.Errorf("runs by itself: %s", got)
	}
	if got := names(choose(all, []string{"fans"})); got != "Temperatures and fan" {
		t.Errorf("asked for by name: %s", got)
	}
	handsOff(all, []string{"temperatures"})
	if ear.handsOff || stepCheck[ear.name()] != "fans" {
		t.Errorf("by ear: hands off %v, check %q", ear.handsOff, stepCheck[ear.name()])
	}
	// With a sensor it measures by itself, and asks nothing even when run alone
	read := newHeat(run, machine.Fixture(dir), dir)
	read.sensor = true
	all = []step{newKeyboard(run), newQuestions(run, probe.Facts{}, nil), read}
	if got := names(choose(all, []string{"auto"})) + " / " + names(choose(all, []string{"manual"})); got != "Temperatures / Keyboard, Questions" {
		t.Errorf("with a sensor: %s", got)
	}
	handsOff(all, []string{"temperatures"})
	if !read.handsOff || strings.Contains(read.keys(), "hear the fan") {
		t.Errorf("with a sensor it still asks: %v", read.handsOff)
	}
	// The menu lists each under its own group
	entry := func(step string) menuEntry {
		for _, e := range menu {
			if e.step == step {
				return e
			}
		}
		return menuEntry{}
	}
	if e := entry(heatByEar); e.auto || e.word != "temperatures" || e.label != "Temperatures, and the fan by ear" {
		t.Errorf("the by-ear entry: %+v", e)
	}
	if e := entry(heatAlone); !e.auto {
		t.Errorf("the sensor entry: %+v", e)
	}
}

func TestTheOpeningMenuRunsEverythingOrOnePart(t *testing.T) {
	run := &report.Run{}
	run.Set("wifi", report.Works, report.Measured, "wlan0 connected", "")
	dir := t.TempDir()
	video := newAuto(run, deep.Test{Name: "Video decoding", Checks: []string{"video-decode"}, Run: func(func(string)) []report.Result { return nil }})
	steps := []step{newKeyboard(run), &effects{run: run}, newSpeakers(run, dir), newHeadphones(run, dir), newCamera(run, dir), newQuestions(run, probe.Facts{}, nil), video}
	last := &summary{run: run, options: Options{Send: false}}
	// What this machine found the last time each thing was tested
	tested := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)
	kept := map[string]report.Kept{
		"speakers":    {Result: report.Result{Check: "speakers", State: report.Partial, How: report.Owner, Note: "Both speakers play, but the left is louder than the right."}, When: tested},
		"microphones": {Result: report.Result{Check: "microphones", State: report.Works, How: report.Measured}, When: tested},
		"camera":      {Result: report.Result{Check: "camera", State: report.Works, How: report.Owner}, When: tested},
		"wifi":        {Result: report.Result{Check: "wifi", State: report.Works, How: report.Measured}, When: tested},
		"hibernate":   {Result: report.Result{Check: "hibernate", State: report.Broken, How: report.Measured, Evidence: "the kernel offers no hibernation"}, When: tested},
	}
	open := func() (*model, *welcome) {
		m := &model{run: run, summary: last, failed: map[string]string{}, width: 100, height: 60}
		start := &welcome{run: run, kept: kept}
		for _, entry := range menu {
			if len(choose(steps, []string{entry.word})) > 0 {
				start.parts = append(start.parts, entry)
			}
		}
		start.pick = func(word string) {
			run.Results = nil
			m.steps = append(append([]step{start}, choose(steps, []string{word})...), last)
		}
		m.steps = append(append([]step{start}, steps...), last)
		return m, start
	}

	// Only what this machine has is offered
	m, start := open()
	drawn := m.View().Content
	for _, text := range []string{"Everything", "about fifteen minutes", "Keyboard, volume and brightness keys", "Speakers and microphones", "Headphones", "Camera", "hw-test camera", "Video decoding"} {
		if !strings.Contains(drawn, text) {
			t.Errorf("the menu lacks %q:\n%s", text, drawn)
		}
	}
	if strings.Contains(drawn, "Touchscreen") || strings.Contains(drawn, "Sleep") || strings.Contains(drawn, "Graphics: Vulkan") {
		t.Errorf("the menu offers what the machine doesn't have:\n%s", drawn)
	}
	// What runs by itself is listed apart from what is done with the owner, and comes first
	alone, with := strings.Index(drawn, "Runs by itself"), strings.Index(drawn, "Done with you")
	if alone < 0 || !(alone < strings.Index(drawn, "All of these in one go") && strings.Index(drawn, "Video decoding") < with && with < strings.Index(drawn, "Keyboard, volume")) {
		t.Errorf("the two groups are off:\n%s", drawn)
	}
	// Each group opens with a way to run all of it
	if both := strings.LastIndex(drawn, "All of these in one go"); strings.Count(drawn, "All of these in one go") != 2 || !(with < both && both < strings.Index(drawn, "Keyboard, volume")) || strings.Contains(drawn, "about ten minutes") || strings.Contains(drawn, "about four minutes") {
		t.Errorf("the group done with the owner has no way to run all of it:\n%s", drawn)
	}
	// Each part shows how it stood the last time it was tested here, and the screen says where
	// the machine stands overall and what fails that no part of the menu covers
	plain := ansi.ReplaceAllString(drawn, "")
	for _, text := range []string{"◐ Speakers and microphones", "19 Nov · Speakers: Both speakers play", "✓ Camera", "○ Headphones", "○ Video decoding",
		"3 working", "1 partly", "1 broken", "27 not tested", "✗ Hibernation: the kernel offers no hibernation"} {
		if !strings.Contains(plain, text) {
			t.Errorf("the status lacks %q:\n%s", text, plain)
		}
	}
	// Nothing in the menu has a number, and every part is one line
	if regexp.MustCompile(`(?m)^\s*[›]?\s*\d+ (Everything|Camera|Quit)`).MatchString(plain) || strings.Contains(m.steps[0].keys(), "number") {
		t.Errorf("the menu is numbered:\n%s", plain)
	}
	// With the cursor on a part that fails, the reason is said in full under the menu, with
	// the command that goes straight to the part
	for range 2 {
		m.Update(press(tea.KeyDown))
	}
	under := strings.Join(strings.Fields(ansi.ReplaceAllString(m.View().Content, "")), " ")
	if !strings.Contains(under, "◐ 19 Nov · Speakers: Both speakers play, but the left is louder than the right.") || !strings.Contains(under, "hw-test speakers goes straight to this part.") {
		t.Errorf("under the menu:\n%s", under)
	}
	for range 2 {
		m.Update(press(tea.KeyUp))
	}
	// The menu is the list of what there is: no second list of steps above it, and nothing to skip
	if strings.Contains(drawn, "○ Send") || strings.Contains(drawn, "Ctrl+N") {
		t.Errorf("the start screen repeats the steps:\n%s", drawn)
	}
	// Once the site has said which model this is, its page is shown, and o opens it
	start.update(modelMsg{model: report.Model{ID: "lenovo-yoga-slim-7x-14q8x9", Page: "https://arewearmyet.com/models/lenovo-yoga-slim-7x-14q8x9"}})
	if drawn = m.View().Content; !strings.Contains(drawn, "arewearmyet.com/models/lenovo-yoga-slim-7x-14q8x9") || !strings.Contains(drawn, "o to open this computer's page") {
		t.Errorf("the model's page isn't shown:\n%s", drawn)
	}
	// Enter on the first entry runs it all
	m.Update(press(tea.KeyEnter))
	if m.at != 1 || len(m.steps) != 9 || run.State("wifi") != report.Works {
		t.Errorf("the whole test: at %d of %d", m.at, len(m.steps))
	}

	// The camera alone: only it and the send step are left, and only it is reported
	m, start = open()
	at := 0
	for i, part := range start.parts {
		if part.word == "camera" {
			at = i + 2
		}
	}
	for range at - 1 {
		m.Update(press(tea.KeyDown))
	}
	m.Update(press(tea.KeyEnter))
	if len(m.steps) != 3 || m.steps[m.at].name() != "Camera" || len(run.Results) != 0 {
		t.Errorf("one part: %d steps, at %q, results %+v", len(m.steps), m.steps[m.at].name(), run.Results)
	}
	// The part is remembered, so the menu comes up again after it
	if start.part != "camera" || start.quit {
		t.Errorf("part %q, quit %v", start.part, start.quit)
	}

	// Quit is the last entry, and leaves without starting anything
	m, start = open()
	if !strings.Contains(m.View().Content, "Quit") {
		t.Errorf("the menu lacks Quit:\n%s", m.View().Content)
	}
	start.update(press(tea.KeyUp))
	quit := start.update(press(tea.KeyEnter))
	if !start.quit || start.part != "" || quit == nil {
		t.Errorf("quit %v, part %q", start.quit, start.part)
	}
}

func TestTheSendScreenSaysWhatGoesAndThatNoPictureDoes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "camera.png"), []byte("a picture"), 0o600)
	os.WriteFile(filepath.Join(dir, "voice.wav"), []byte("a recording"), 0o600)
	run := &report.Run{}
	run.Set("camera", report.Works, report.Owner, "a 1920x1092 frame, brightness 41 of 255, contrast 18", "")
	s := &summary{run: run, options: Options{Send: true}, dir: dir, recorded: true}
	s.start()
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("%d files are still there when the send screen comes up", len(left))
	}
	// Wrapped lines joined again, so a sentence broken across two is still found
	drawn := strings.Join(strings.Fields(s.view(90, 30)), " ")
	for _, text := range []string{"Webcam", "everything that is sent", "No picture and no recording is sent", "already deleted", "Send the lines above"} {
		if !strings.Contains(drawn, text) {
			t.Errorf("the send screen lacks %q:\n%s", text, drawn)
		}
	}

	// A round that made no picture and no recording doesn't speak of them
	video := &report.Run{}
	video.Set("video-decode", report.Works, report.Measured, "60 frames of 720p H.264 decoded on the hardware decoder in 0.2 s", "")
	s = &summary{run: video, options: Options{Send: true}}
	s.start()
	if drawn = strings.Join(strings.Fields(s.view(90, 30)), " "); !strings.Contains(drawn, "everything that is sent") || strings.Contains(drawn, "picture") || strings.Contains(drawn, "recording") {
		t.Errorf("the send screen of a video round:\n%s", drawn)
	}
	camera, decoding := newCamera(run, dir), newAuto(run, deep.Test{Name: "Video decoding"})
	if !records([]step{decoding, camera}) || records([]step{decoding, newHeadphones(run, dir), newUSB(run, machine.Fixture(dir))}) {
		t.Error("which rounds record is off")
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
	m.steps = []step{&welcome{run: run}, newTouchscreen(run, machine.Fixture(t.TempDir()), "screen", ""), newQuestions(run, probe.Facts{}, map[string]bool{"usb": true, "external-display": true}), last}
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
	if got := names(choose(all, []string{"sound"})); got != "Sound, Headphones" {
		t.Errorf("sound: %s", got)
	}
	if got := names(choose(all, []string{"camera", "keyboard"})); got != "Keyboard, Volume and brightness keys, Camera" {
		t.Errorf("camera and keyboard: %s", got)
	}
	// Everything done with the owner in one go: not the sound, which measures itself, nor the
	// voice step that only stands in for it
	if got := names(choose(all, []string{"manual"})); got != "Keyboard, Volume and brightness keys, Headphones, Camera, Questions" {
		t.Errorf("manual: %s", got)
	}
	if Automatic([]string{"manual"}, false) {
		t.Error("the tests done with the owner count as needing nobody")
	}
	if got := names(choose(all, []string{"voice"})); got != "Voice" {
		t.Errorf("voice: %s", got)
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

func TestTouchscreenCountsTouchesByItsInterrupt(t *testing.T) {
	// A touchscreen on I2C as the device tree describes it: pin 51 of the GPIO controller
	root := t.TempDir()
	set := func(path string, data []byte) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), data, 0o644)
	}
	device := "sys/devices/platform/soc/a80000.i2c/i2c-0/0-0014"
	set(device+"/of_node/interrupts-extended", []byte{0, 0, 0, 0x61, 0, 0, 0, 0x33, 0, 0, 0, 8})
	set(device+"/0018:27C6:0123.0001/input/input5/event5/dev", []byte("13:69\n"))
	os.MkdirAll(filepath.Join(root, "sys/class/input"), 0o755)
	os.Symlink(filepath.Join(root, device, "0018:27C6:0123.0001/input/input5/event5"), filepath.Join(root, "sys/class/input/event5"))
	os.Symlink("..", filepath.Join(root, device, "0018:27C6:0123.0001/input/input5/event5/device"))
	// Three lines share the driver's name; another controller has a pin 51 too
	for irq, line := range map[string][3]string{"138": {"51", "hid-over-i2c", "653,0,0"}, "141": {"3", "hid-over-i2c", "14377,0,0"}, "77": {"51", "", "0,0,0"}} {
		set("sys/kernel/irq/"+irq+"/hwirq", []byte(line[0]+"\n"))
		set("sys/kernel/irq/"+irq+"/actions", []byte(line[1]+"\n"))
		set("sys/kernel/irq/"+irq+"/per_cpu_count", []byte(line[2]+"\n"))
	}
	sys := machine.Fixture(root)
	run := &report.Run{}
	screen := newTouchscreen(run, sys, "hid-over-i2c 27C6:0123", "/sys/class/input/event5")
	if screen.line != "/sys/kernel/irq/138" || screen.wantsMouse() {
		t.Fatalf("interrupt line %q", screen.line)
	}
	now := time.Unix(1000, 0)
	screen.clock = func() time.Time { return now }
	screen.start()
	count := int64(653)
	// A touch: the line fires for a moment, then goes quiet
	touch := func() {
		for i := 0; i < 5; i++ {
			count += 7
			now = now.Add(100 * time.Millisecond)
			screen.update(touchMsg{count})
		}
		for i := 0; i < 5; i++ {
			now = now.Add(100 * time.Millisecond)
			screen.update(touchMsg{count})
		}
	}
	touch()
	touch()
	if screen.touches != 2 || screen.ask != nil {
		t.Fatalf("after two touches: %d counted", screen.touches)
	}
	touch()
	if screen.ask == nil || !strings.Contains(screen.ask.question, "registered 3 touches") {
		t.Fatalf("after three touches: %d counted, no question", screen.touches)
	}
	screen.update(press('1'))
	result, _ := run.Get("touchscreen")
	if !screen.done() || result.State != report.Works || result.How != report.Measured || !strings.Contains(result.Evidence, "3 touches, its interrupt fired 105 times") {
		t.Errorf("result %+v", result)
	}

	// Touching does nothing on the desktop, though the line fires
	run = &report.Run{}
	screen = newTouchscreen(run, sys, "screen", "/sys/class/input/event5")
	screen.clock = func() time.Time { return now }
	screen.start()
	screen.update(touchMsg{count + 9})
	screen.update(press('n'))
	if result, _ := run.Get("touchscreen"); result.State != report.Partial {
		t.Errorf("fires without effect: %+v", result)
	}
}

func TestTouchscreenFallsBackToClicksWithoutAnInterrupt(t *testing.T) {
	run := &report.Run{}
	screen := newTouchscreen(run, machine.Fixture(t.TempDir()), "a USB touchscreen", "")
	if !screen.wantsMouse() {
		t.Fatal("it doesn't ask for the pointer")
	}
	screen.update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: 4})
	screen.update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: 4}) // the same place twice counts once
	screen.update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 30, Y: 9})
	if screen.ask != nil {
		t.Fatal("asked after two places")
	}
	screen.update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 60, Y: 2})
	screen.update(press('2'))
	if result, _ := run.Get("touchscreen"); !screen.done() || result.State != report.Partial {
		t.Errorf("result %+v", result)
	}

	run = &report.Run{}
	screen = newTouchscreen(run, machine.Fixture(t.TempDir()), "x", "")
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
	steps := []step{&welcome{run: run}, back, newKeyboard(run), &effects{run: run, items: []effect{{label: "Volume up"}}}, newTouchpad(run, "pad"), newTouchscreen(run, machine.Fixture(dir), "screen", ""),
		newSpeakers(run, dir), newMicrophones(run, dir), newHeadphones(run, dir), newCamera(run, dir), newUSB(run, sys), newScreen(run, sys), newSleep(run, probe.Facts{}),
		newQuestions(run, probe.Facts{Laptop: true}, nil), last}
	steps = append(steps, autos(run)...)
	steps = append(steps, lids(run)...)
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

// Nothing may run past the window's edge, where it would be cut off: every step is drawn in
// a narrow window and each line measured. Only the picture of the keyboard has a width of its own.
func TestNoLineIsCutOffInANarrowWindow(t *testing.T) {
	run := &report.Run{Hardware: map[string]string{"maker": "LENOVO", "product": "83ED"}}
	run.Set("wifi", report.Works, report.Measured, "wlan0 (ath12k_wifi7_pci) connected, with a long line of evidence that goes on and on", "")
	dir := t.TempDir()
	sys := machine.Fixture(dir)
	last := &summary{run: run, options: Options{Send: true}}
	last.start()
	back := &resumed{run: run, pending: report.Pending{Step: "External screen"}, restarted: true}
	back.ask = back.question()
	steps := []step{&welcome{run: run}, back, &effects{run: run, backlight: "x", items: []effect{{label: "Mute again, to turn the sound back on"}}}, newTouchpad(run, "pad"),
		newTouchscreen(run, sys, "screen", ""), newSpeakers(run, dir), newMicrophones(run, dir), newHeadphones(run, dir), newCamera(run, dir), newUSB(run, sys),
		newScreen(run, sys), newSleep(run, probe.Facts{}), newQuestions(run, probe.Facts{Laptop: true, Encrypted: true}, nil), last}
	steps = append(steps, autos(run)...)
	steps = append(steps, lids(run)...)
	for _, width := range []int{72, 50} {
		for at, s := range steps {
			m := &model{run: run, summary: last, failed: map[string]string{}, steps: steps, at: at, width: width, height: 40}
			for _, line := range strings.Split(m.View().Content, "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Errorf("%s at %d columns has a line of %d: %q", s.name(), width, got, strings.TrimSpace(line))
				}
			}
		}
	}
}

// played feeds the sound step what the microphones recorded of each part, as if it had played.
func played(s *speakers, levels ...int) {
	s.update(press(tea.KeyEnter))
	for at, level := range levels {
		heard := audio.Heard{}
		if level > 0 {
			heard = audio.Heard{Windows: 5, Best: 92, Level: level}
		}
		s.update(partMsg{at: at, heard: heard, noise: 9})
	}
}

func TestSoundIsMeasuredFromTheRecordingWithNobodyListening(t *testing.T) {
	run := &report.Run{}
	s := newSpeakers(run, t.TempDir())
	played(s, 240, 104, 60, 300, 180)
	if s.phase != "result" {
		t.Fatalf("phase %q", s.phase)
	}
	speakers, _ := run.Get("speakers")
	microphones, _ := run.Get("microphones")
	if speakers.State != report.Works || speakers.How != report.Measured || !strings.Contains(speakers.Evidence, "left speaker heard at -43 dB, right speaker heard at -50 dB; the left side 7 dB louder than the right") {
		t.Errorf("speakers: %+v", speakers)
	}
	// Both play, so it works. A side that far below the other is said, and marked as worth a look
	if speakers.Look != "The right speaker measures 7 dB quieter than the other." || !strings.Contains(speakers.Note, "That is worth a listen") {
		t.Errorf("an uneven pair: look %q, note %q", speakers.Look, speakers.Note)
	}
	// A few dB apart is how most laptops measure, and nothing to look into
	even := &report.Run{}
	close := newSpeakers(even, t.TempDir())
	played(close, 240, 150, 60, 300, 180)
	if result, _ := even.Get("speakers"); result.State != report.Works || result.Look != "" || result.Note != "" {
		t.Errorf("an even pair: %+v", result)
	}
	if microphones.State != report.Works || microphones.How != report.Measured || !strings.Contains(microphones.Evidence, "300 Hz at -55 dB, 1000 Hz at -41 dB, 3000 Hz at -45 dB; noise with nothing playing -71 dB") {
		t.Errorf("microphones: %+v", microphones)
	}
	drawn := s.view(90, 40)
	for _, text := range []string{"Left", "heard at -43 dB", "the left side 7 dB louder", "3000 Hz", "noise with nothing playing: -71 dB"} {
		if !strings.Contains(drawn, text) {
			t.Errorf("the result doesn't show %q:\n%s", text, drawn)
		}
	}
	// Enter takes the measurement as it is: nothing was asked
	s.update(press(tea.KeyEnter))
	if !s.done() {
		t.Error("not done after Enter")
	}
}

func TestSoundWithOneSpeakerSilentIsPartlyWorking(t *testing.T) {
	run := &report.Run{}
	s := newSpeakers(run, t.TempDir())
	played(s, 240, 0, 60, 300, 180)
	speakers, _ := run.Get("speakers")
	if speakers.State != report.Partial || speakers.Note != "Only the left speaker was heard by the microphones." || run.State("microphones") != report.Works {
		t.Errorf("speakers %+v, microphones %q", speakers, run.State("microphones"))
	}
}

func TestSoundAsksOnlyWhenTheRecordingHoldsNothing(t *testing.T) {
	// The owner heard both sides, so the microphones are what failed
	run := &report.Run{}
	s := newSpeakers(run, t.TempDir())
	played(s, 0, 0, 0, 0, 0)
	if s.phase != "silent" || len(run.Results) != 0 {
		t.Fatalf("phase %q, results %+v", s.phase, run.Results)
	}
	s.update(press('1'))
	speakers, _ := run.Get("speakers")
	microphones, _ := run.Get("microphones")
	if speakers.State != report.Works || speakers.How != report.Owner || microphones.State != report.Broken || microphones.How != report.Measured {
		t.Errorf("speakers %+v, microphones %+v", speakers, microphones)
	}

	// The owner heard nothing either: the speakers failed, and the microphones are left to the voice step
	run = &report.Run{}
	s = newSpeakers(run, t.TempDir())
	played(s, 0, 0, 0, 0, 0)
	s.update(press('4'))
	if run.State("speakers") != report.Broken || run.State("microphones") != "" {
		t.Errorf("speakers %q, microphones %q", run.State("speakers"), run.State("microphones"))
	}
	voice := &skippable{step: newMicrophones(run, t.TempDir()), skip: s.heardTone}
	if voice.done() {
		t.Error("the voice step is skipped though the microphones heard nothing")
	}
}

func TestTheOwnerCanStillSayTheSoundWasWrong(t *testing.T) {
	run := &report.Run{}
	s := newSpeakers(run, t.TempDir())
	played(s, 240, 104, 60, 300, 180)
	s.update(press('p'))
	s.update(press('1'))
	result, _ := run.Get("speakers")
	if !s.done() || result.State != report.Partial || result.How != report.Owner || result.Note != "Both speakers play, but the left is louder than the right." || !strings.Contains(result.Evidence, "7 dB louder") {
		t.Errorf("result %+v", result)
	}

	// In their own words
	run = &report.Run{}
	s = newSpeakers(run, t.TempDir())
	played(s, 240, 230, 60, 300, 180)
	s.update(press('p'))
	s.update(press('5'))
	for _, c := range "a hum under the tone" {
		s.update(press(c))
	}
	s.update(press(tea.KeyEnter))
	if result, _ := run.Get("speakers"); result.Note != "a hum under the tone" || !strings.Contains(result.Evidence, "about equally loud") {
		t.Errorf("in the owner's words: %+v", result)
	}

	// The voice step is passed over once the microphones heard the tones
	voice := &skippable{step: newMicrophones(run, t.TempDir()), skip: s.heardTone}
	if !voice.done() {
		t.Error("the voice step still runs")
	}
}

// speech is four seconds whose loudest part is at the given level.
func speech(level float64) []int16 {
	samples := make([]int16, 4*audio.Rate)
	for i := range samples {
		loudness := level / 10
		if i > audio.Rate && i < 2*audio.Rate {
			loudness = level
		}
		samples[i] = int16(loudness * 1.414 * math.Sin(2*math.Pi*220*float64(i)/audio.Rate))
	}
	return samples
}

func TestMicrophoneLevelIsMeasuredSoQuietIsNotBlamedOnTheSpeakers(t *testing.T) {
	record := func(level float64) (*microphones, *report.Run) {
		run := &report.Run{}
		m := newMicrophones(run, t.TempDir())
		m.update(recordedMsg{samples: speech(level)})
		m.update(playedMsg{})
		return m, run
	}

	// A quiet microphone: the recording itself is far below the tone
	m, run := record(150)
	if !m.quiet() || m.gain < 20 || !strings.Contains(m.view(100, 40), "it is the microphone and not the speakers") || !strings.Contains(m.ask.question, "times louder") {
		t.Fatalf("quiet=%v gain=%.0f\n%s", m.quiet(), m.gain, m.view(100, 40))
	}
	m.update(press('1')) // clear, once made louder
	result, _ := run.Get("microphones")
	if result.State != report.Partial || !strings.Contains(result.Note, "records quietly") || !strings.Contains(result.Evidence, "speech recorded at -47 dB, where the test tone plays at -12 dB") {
		t.Errorf("quiet but clear: %+v", result)
	}

	// A healthy level, and clear
	m, run = record(4000)
	if m.quiet() || m.silent() || !strings.Contains(m.view(100, 40), "healthy level") {
		t.Fatalf("healthy: quiet=%v silent=%v", m.quiet(), m.silent())
	}
	m.update(press('1'))
	if run.State("microphones") != report.Works {
		t.Errorf("healthy and clear: %q", run.State("microphones"))
	}

	// Almost nothing recorded
	m, run = record(8)
	if !m.silent() || !strings.Contains(m.view(100, 40), "picked up no voice") {
		t.Fatalf("silent=%v", m.silent())
	}
	m.update(press('3'))
	if run.State("microphones") != report.Broken {
		t.Errorf("no voice: %q", run.State("microphones"))
	}
}

func TestCtrlNPassesAnyStepOver(t *testing.T) {
	next := tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
	if next.String() != "ctrl+n" {
		t.Fatalf("the key reads as %q", next.String())
	}
	run := &report.Run{}
	dir := t.TempDir()
	last := &summary{run: run, options: Options{Send: false}}
	m := &model{run: run, summary: last, failed: map[string]string{}, width: 100, height: 40}
	// The keyboard step takes every key for itself, and the keys step wants six presses
	m.steps = []step{&welcome{run: run}, newKeyboard(run), &effects{run: run, items: []effect{{label: "Volume up"}, {label: "Mute"}}}, newSpeakers(run, dir), newCamera(run, dir),
		newQuestions(run, probe.Facts{}, nil), last}
	for want := 1; want <= 6; want++ {
		m.Update(next)
		if m.at != want {
			t.Fatalf("after %d presses it is at step %d", want, m.at)
		}
		// Every step of the test shows the key; the last one, which sends, doesn't
		if shown := strings.Contains(m.View().Content, "Ctrl+N for the next step"); shown != (want < 6) {
			t.Errorf("at step %d the key is shown: %v", want, shown)
		}
	}
	// Nothing was reported for what was passed over, and the keyboard didn't take the n as a key
	if len(run.Results) != 0 {
		t.Errorf("results: %+v", run.Results)
	}
	// The last step isn't skipped, and doesn't offer it
	m.Update(next)
	if m.at != 6 || strings.Contains(m.View().Content, "Ctrl+N") {
		t.Errorf("the send step: at %d", m.at)
	}
	if !strings.Contains(m.View().Content, "– Keyboard") {
		t.Errorf("skipped steps aren't marked:\n%s", m.View().Content)
	}
}

func TestSleepWaitsForWifiAndRecordsHowLongItTook(t *testing.T) {
	woke := func(root string) (*sleep, *report.Run, *time.Time) {
		set := func(path, text string) {
			os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
			os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
		}
		set("sys/class/net/wlan0/operstate", "up\n")
		set("sys/class/remoteproc/remoteproc0/name", "adsp\n")
		set("sys/class/remoteproc/remoteproc0/state", "running\n")
		set("sys/power/mem_sleep", "s2idle [deep]\n")
		set("sys/power/suspend_stats/success", "1\n")
		set("sys/power/suspend_stats/fail", "0\n")
		run := &report.Run{}
		s := newSleep(run, probe.Facts{Wifi: "wlan0"})
		s.sys = machine.Fixture(root)
		now := time.Unix(5000, 0)
		s.clock = func() time.Time { return now }
		s.update(press(tea.KeyEnter))
		// Asleep: Wi-Fi drops. Awake: the kernel counts one more sleep, and Wi-Fi is still down
		set("sys/class/net/wlan0/operstate", "down\n")
		s.update(sleepMsg{ok: 2, failed: 0})
		if s.phase != "settling" {
			t.Fatalf("after waking it is %q", s.phase)
		}
		return s, run, &now
	}

	// Wi-Fi comes back after 23 seconds: slow, and not a failure
	root := t.TempDir()
	s, run, now := woke(root)
	for second := 1; second <= 22; second++ {
		*now = now.Add(time.Second)
		s.update(settleMsg{})
	}
	if s.phase != "settling" || !strings.Contains(s.view(90, 30), "Waiting for Wi-Fi to come back, 22 s so far") {
		t.Fatalf("after 22 s: %q\n%s", s.phase, s.view(90, 30))
	}
	os.WriteFile(filepath.Join(root, "sys/class/net/wlan0/operstate"), []byte("up\n"), 0o644)
	*now = now.Add(time.Second)
	s.update(settleMsg{})
	if s.phase != "ask" || len(s.problems) != 0 {
		t.Fatalf("once Wi-Fi is back: %q, problems %v", s.phase, s.problems)
	}
	s.update(press('1'))
	result, _ := run.Get("suspend")
	if result.State != report.Works || !strings.Contains(result.Evidence, "Wi-Fi was back 23 s after waking, the audio processor is running") {
		t.Errorf("result %+v", result)
	}

	// Still down after a minute: that is a problem, whatever the owner answers
	s, run, now = woke(t.TempDir())
	for second := 1; second <= 60; second++ {
		*now = now.Add(time.Second)
		s.update(settleMsg{})
	}
	if s.phase != "ask" {
		t.Fatalf("after a minute it is %q", s.phase)
	}
	s.update(press('1'))
	if result, _ := run.Get("suspend"); result.State != report.Partial || !strings.Contains(result.Evidence, "Wi-Fi hadn't reconnected 60 s after waking") {
		t.Errorf("never back: %+v", result)
	}
}

func TestStatusListsEveryCheckWithWhatFailsFirst(t *testing.T) {
	when := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)
	listed := Status(map[string]report.Kept{
		"wifi":      {Result: report.Result{Check: "wifi", State: report.Works}, Kernel: "7.3.0-59-omarchy", When: when},
		"hibernate": {Result: report.Result{Check: "hibernate", State: report.Broken, Evidence: "the kernel offers no hibernation"}, Kernel: "7.3.0-59-omarchy", When: when},
		"suspend":   {Result: report.Result{Check: "suspend", State: report.Partial, Note: "One sleep ended in a restart."}, Kernel: "7.3.0-59-omarchy", When: when},
	})
	if !strings.HasPrefix(listed, "1 working, 1 partly, 1 broken, 29 not tested") {
		t.Errorf("the head:\n%s", listed)
	}
	broken, partly, works, untested := strings.Index(listed, "✗ Hibernation"), strings.Index(listed, "◐ Sleep and wake"), strings.Index(listed, "✓ Wi-Fi"), strings.Index(listed, "○ ")
	if broken < 0 || !(broken < partly && partly < works && works < untested) {
		t.Errorf("the order is off:\n%s", listed)
	}
	for _, text := range []string{"the kernel offers no hibernation", "One sleep ended in a restart.", "2026-11-19", "7.3.0-59-omarchy"} {
		if !strings.Contains(listed, text) {
			t.Errorf("the list lacks %q", text)
		}
	}
	// One line for each of the checks, and the reasons on lines of their own
	checks := 0
	for _, line := range strings.Split(listed, "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			checks++
		}
	}
	if checks != 32 {
		t.Errorf("it lists %d checks", checks)
	}
}

func TestARoundEndsOnGoingBackToTheStartByDefault(t *testing.T) {
	run := &report.Run{}
	run.Set("camera", report.Works, report.Owner, "a frame", "")

	// Sent: back to the start is where the cursor rests, then the page, then leaving
	s := &summary{run: run, options: Options{Send: true}}
	s.start()
	s.update(sentMsg{sent: report.Sent{Run: "RUN-AAAAAAAAAA", Page: "https://arewearmyet.com/models/x"}})
	if got := strings.Join(s.ask.options, " | "); got != "Back to the start | Open this computer's page in the browser | Quit" || s.ask.cursor != 0 {
		t.Fatalf("the last choice: %s, cursor %d", got, s.ask.cursor)
	}
	s.update(press(tea.KeyEnter))
	if !s.done() || !s.again {
		t.Errorf("Enter: done %v, again %v", s.done(), s.again)
	}

	// Without a network, as after the lid test, it waits and tries again by itself, and says so
	s = &summary{run: run, options: Options{Send: true}}
	s.start()
	if s.update(press(tea.KeyEnter)) == nil || s.phase != "sending" || strings.Contains(s.view(80, 30), "Waiting for the network") {
		t.Fatalf("after Send: phase %q\n%s", s.phase, s.view(80, 30))
	}
	away := &url.Error{Op: "Post", URL: "https://arewearmyet.com/api/runs", Err: &net.DNSError{Err: "no such host", Name: "arewearmyet.com", IsNotFound: true}}
	if s.update(sentMsg{err: away}) == nil || s.phase != "sending" || !strings.Contains(s.view(80, 30), "Waiting for the network") {
		t.Fatalf("with the network away: phase %q\n%s", s.phase, s.view(80, 30))
	}
	if s.update(retryMsg{}) == nil {
		t.Error("it didn't try again")
	}
	s.update(sentMsg{sent: report.Sent{Run: "RUN-BBBBBBBBBB", Page: "https://arewearmyet.com/models/x"}})
	if s.phase != "end" || !strings.Contains(s.ask.question, "Sent as RUN-BBBBBBBBBB") {
		t.Errorf("once the network was back: phase %q, %s", s.phase, s.ask.question)
	}
	// A send that is only slow says the same after a few seconds
	s = &summary{run: run, options: Options{Send: true}}
	s.start()
	s.update(press(tea.KeyEnter))
	s.update(slowMsg{})
	if !strings.Contains(s.view(80, 30), "Waiting for the network") {
		t.Error("a slow send says nothing")
	}
	// After a minute without a network it gives up and asks, and a no from the site is said at once
	s.began = time.Now().Add(-2 * time.Minute)
	s.update(sentMsg{err: away})
	if s.phase != "failed" || !strings.Contains(s.ask.question, "Sending failed") {
		t.Errorf("after a minute: phase %q", s.phase)
	}
	s = &summary{run: run, options: Options{Send: true}}
	s.start()
	s.update(press(tea.KeyEnter))
	s.update(sentMsg{err: fmt.Errorf("reports are closed")})
	if s.phase != "failed" || !strings.Contains(s.ask.question, "reports are closed") {
		t.Errorf("a no from the site: phase %q", s.phase)
	}

	// Opened by an agent for the owner, the round ends after its steps with no screen and nothing sent
	s = &summary{run: run, options: Options{Send: true}, once: true}
	if s.start() != nil || !s.done() || s.farewell() != "" || s.phase == "sending" {
		t.Errorf("once: done %v, farewell %q, phase %q", s.done(), s.farewell(), s.phase)
	}

	// Quit leaves, and doesn't come back
	s = &summary{run: run, options: Options{Send: true}}
	s.start()
	s.update(sentMsg{sent: report.Sent{Run: "RUN-AAAAAAAAAA", Page: "https://arewearmyet.com/models/x"}})
	s.update(press('3'))
	if !s.done() || s.again {
		t.Errorf("Quit: done %v, again %v", s.done(), s.again)
	}

	// Not sending leads to the same choice, without a page to open, and the results stay in view
	s = &summary{run: run, options: Options{Send: true}}
	s.start()
	s.update(press('2'))
	if got := strings.Join(s.ask.options, " | "); got != "Back to the start | Quit" || s.done() || !strings.Contains(s.view(90, 30), "Webcam") {
		t.Errorf("after declining: %s, done %v", got, s.done())
	}
	if s.farewell() != "Not sent." {
		t.Errorf("farewell %q", s.farewell())
	}

	// Asked not to send at all: the results, then the same choice
	s = &summary{run: run, options: Options{Send: false}}
	s.start()
	if drawn := s.view(90, 30); !strings.Contains(drawn, "Webcam") || !strings.Contains(drawn, "Nothing is sent, as asked") || !strings.Contains(drawn, "Back to the start") {
		t.Errorf("with sending off:\n%s", drawn)
	}
}
