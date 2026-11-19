package guided

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	q := newQuestions(run, probe.Facts{Laptop: true, Touchpad: "pad", SoundCard: "card", Bluetooth: "hci0"})
	var checks []string
	for _, question := range q.list {
		checks = append(checks, question.check)
	}
	if got := strings.Join(checks, " "); got != "touchpad headphones usb external-display installer" {
		t.Errorf("asked: %s", got)
	}

	q.update(press('1')) // touchpad: yes
	q.update(press('5')) // headphones: no jack
	q.update(press('2')) // usb: partly, then a note
	for _, c := range "left port dead" {
		q.update(press(c))
	}
	q.update(press(tea.KeyEnter))
	q.update(press('4')) // external screen: haven't tried
	q.update(press('3')) // installer: no
	q.update(press(tea.KeyEnter))
	if !q.done() {
		t.Fatal("not done")
	}
	want := map[string]string{"touchpad": report.Works, "headphones": report.Absent, "usb": report.Partial, "external-display": "", "installer": report.Broken}
	for check, state := range want {
		if run.State(check) != state {
			t.Errorf("%s: %q, want %q", check, run.State(check), state)
		}
	}
	if usb, _ := run.Get("usb"); usb.Note != "left port dead" || usb.How != report.Owner {
		t.Errorf("usb: %+v", usb)
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
	m := &model{run: run, summary: last, steps: []step{&welcome{run: run}, &effects{run: run}, newQuestions(run, probe.Facts{}), last}}
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
