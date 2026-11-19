package guided

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/deep"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// oven is a load whose readings are given: the temperature at each second, and the second a
// fan starts at, if it does.
type oven struct {
	found   deep.Heat
	hot     []float64
	kickAt  int
	stopped int
}

func (o *oven) Step() (deep.Sample, bool) {
	at := len(o.found.Samples)
	sample := deep.Sample{At: time.Duration(at+1) * time.Second, Hot: o.hot[min(at, len(o.hot)-1)], Rate: 1}
	o.found.Samples = append(o.found.Samples, sample)
	o.found.Peak = max(o.found.Peak, sample.Hot)
	if o.kickAt > 0 && at+1 >= o.kickAt {
		o.found.Kicked = &deep.Kick{At: sample.At, From: o.found.Peak, To: sample.Hot}
	}
	return sample, o.found.Kicked != nil || at+1 >= len(o.hot)
}

func (o *oven) Heard() {
	sample := o.found.Samples[len(o.found.Samples)-1]
	o.found.Heard = &sample
}
func (o *oven) NoFan() { o.found.NoFan = true }
func (o *oven) Stop() {
	o.stopped++
	o.found.Loaded = time.Duration(len(o.found.Samples)) * time.Second
}
func (o *oven) Cooled() float64 {
	o.found.After = o.found.Rest + 6
	return o.found.After
}
func (o *oven) Heat() deep.Heat { return o.found }

// warmed is a temperature step over the given readings, on a machine at 38 °C.
func warmed(run *report.Run, kickAt int, hot ...float64) (*heat, *oven) {
	o := &oven{found: deep.Heat{Sensors: 59, Rest: 38, Peak: 38, Slowest: 1}, hot: hot, kickAt: kickAt}
	h := &heat{run: run, second: time.Millisecond, linger: time.Millisecond,
		resting: func() (float64, bool) { return 38, true },
		begin:   func() warmer { return o }}
	return h, o
}

// seconds lets the step's clock run.
func seconds(h *heat, count int) {
	for range count {
		h.update(heatTick{})
	}
}

// heats is the temperature step in each of its phases, for the tests that draw every step.
func heats(run *report.Run) []step {
	cooling, _ := warmed(run, 0, 60)
	cooling.resting = func() (float64, bool) { return 64, false }
	cooling.start()
	loading, _ := warmed(run, 0, 50, 56, 60, 63, 65, 66, 67)
	loading.start()
	seconds(loading, 4)
	after, _ := warmed(run, 3, 50, 60, 55)
	after.start()
	seconds(after, 4)
	asking, _ := warmed(run, 0, 50, 60, 68)
	asking.start()
	seconds(asking, 3+int(deep.CoolAfter.Seconds()))
	shown, _ := warmed(run, 3, 50, 60, 55)
	shown.start()
	seconds(shown, 3+int(deep.CoolAfter.Seconds()))
	return []step{cooling, loading, after, asking, shown}
}

func TestTheTemperatureIsShownAsItClimbsAndTheOwnerSaysWhenTheFanStarts(t *testing.T) {
	run := &report.Run{}
	h, o := warmed(run, 0, 50, 56, 60, 63, 65, 66, 67, 68, 68, 68)
	if h.start() == nil || h.phase != "load" {
		t.Fatalf("it didn't start by itself: %q", h.phase)
	}
	seconds(h, 6)
	drawn := ansi.ReplaceAllString(h.view(90, 30), "")
	for _, text := range []string{"66 °C", "6 s of at most 120", "from 38 °C at rest to 66 °C so far", "press f the moment you hear it speed up", "█"} {
		if !strings.Contains(drawn, text) {
			t.Errorf("while it warms up, the screen lacks %q:\n%s", text, drawn)
		}
	}
	if !strings.Contains(h.keys(), "f the moment you hear the fan") {
		t.Errorf("keys: %s", h.keys())
	}
	// The owner hears the fan: that ends the load there and then, with the reading it came at
	h.update(press('f'))
	if h.phase != "after" || o.stopped != 1 || o.found.Heard == nil || o.found.Heard.Hot != 66 {
		t.Fatalf("phase %q, stopped %d, heard %+v", h.phase, o.stopped, o.found.Heard)
	}
	if drawn = ansi.ReplaceAllString(h.view(90, 30), ""); !strings.Contains(drawn, "You heard the fan at 66 °C, 6 s in") || !strings.Contains(drawn, "Watching it cool") {
		t.Errorf("after the load:\n%s", drawn)
	}
	seconds(h, int(deep.CoolAfter.Seconds()))
	fans, _ := run.Get("fans")
	if h.phase != "shown" || fans.State != report.Works || fans.How != report.Owner || !strings.Contains(fans.Evidence, "heard the fan speed up at 66 °C") {
		t.Fatalf("phase %q, fans %+v", h.phase, fans)
	}
	if h.done() {
		t.Fatal("the result isn't shown")
	}
	h.update(goOnMsg{"Temperatures"})
	if !h.done() {
		t.Error("it waits for a key")
	}
}

func TestAFanSeenInTheReadingsNeedsNobody(t *testing.T) {
	run := &report.Run{}
	h, o := warmed(run, 5, 50, 60, 66, 68, 63, 62)
	h.alone(true)
	h.start()
	if strings.Contains(h.view(90, 30), "press f") || strings.Contains(h.keys(), "hear") {
		t.Errorf("it asks for an ear with nobody there:\n%s\n%s", h.view(90, 30), h.keys())
	}
	// A key meant for the owner does nothing in a round with nobody there
	h.update(press('f'))
	seconds(h, 5)
	if h.phase != "after" || o.stopped != 1 || o.found.Heard != nil || len(o.found.Samples) != 5 {
		t.Fatalf("phase %q after %d readings, stopped %d", h.phase, len(o.found.Samples), o.stopped)
	}
	seconds(h, int(deep.CoolAfter.Seconds()))
	fans, _ := run.Get("fans")
	if h.phase != "shown" || fans.State != report.Works || fans.How != report.Measured || !strings.Contains(fans.Evidence, "while the processor worked on at the same rate") {
		t.Errorf("phase %q, fans %+v", h.phase, fans)
	}
}

func TestAFanThatShowedNowhereIsAskedAboutOnlyWithSomeoneThere(t *testing.T) {
	answer := func(key rune) report.Result {
		run := &report.Run{}
		h, _ := warmed(run, 0, 50, 60, 68)
		h.start()
		seconds(h, 3+int(deep.CoolAfter.Seconds()))
		if h.phase != "ask" || !strings.Contains(h.view(90, 30), "up to 68 °C. Did you hear one?") {
			t.Fatalf("phase %q:\n%s", h.phase, h.view(90, 30))
		}
		// The clock doesn't answer for the owner
		seconds(h, 5)
		if h.phase != "ask" {
			t.Fatalf("the question went away: %q", h.phase)
		}
		h.update(press(key))
		result, _ := run.Get("fans")
		return result
	}
	if heard := answer('1'); heard.State != report.Works || heard.How != report.Owner {
		t.Errorf("heard: %+v", heard)
	}
	if silent := answer('2'); silent.State != report.Partial || silent.Note != deep.FanUnseen {
		t.Errorf("not heard: %+v", silent)
	}
	if fanless := answer('3'); fanless.State != report.Works || !strings.Contains(fanless.Note, "no fan") {
		t.Errorf("no fan: %+v", fanless)
	}
	if unknown := answer('4'); unknown.State != report.Partial || unknown.How != report.Measured {
		t.Errorf("don't know: %+v", unknown)
	}

	// With nobody there, nothing is asked, and what the readings showed is the result
	run := &report.Run{}
	h, _ := warmed(run, 0, 50, 60, 68)
	h.alone(true)
	h.start()
	seconds(h, 3+int(deep.CoolAfter.Seconds()))
	if fans, _ := run.Get("fans"); h.phase != "shown" || fans.State != report.Partial || fans.Note != deep.FanUnseen {
		t.Errorf("phase %q, fans %+v", h.phase, fans)
	}
}

func TestAWarmComputerCoolsBeforeTheLoadAndAFanlessOneIsNotKeptWaiting(t *testing.T) {
	run := &report.Run{}
	h, o := warmed(run, 0, 50, 60, 68)
	degrees := 66.0
	h.resting = func() (float64, bool) { return degrees, degrees < 50 }
	h.start()
	if h.phase != "cooling" || !strings.Contains(h.view(90, 30), "Letting the computer cool first: 66 °C") {
		t.Fatalf("phase %q:\n%s", h.phase, h.view(90, 30))
	}
	degrees = 58
	seconds(h, 1)
	if h.phase != "cooling" || !strings.Contains(h.view(90, 30), "58 °C") {
		t.Fatalf("phase %q:\n%s", h.phase, h.view(90, 30))
	}
	degrees = 48
	seconds(h, 1)
	if h.phase != "load" {
		t.Fatalf("it didn't start once cool: %q", h.phase)
	}
	// One that never cools starts all the same when its time is up, and Enter starts it at once
	h, _ = warmed(run, 0, 70, 71)
	h.resting = func() (float64, bool) { return 66, false }
	h.start()
	seconds(h, int(deep.CoolFirst.Seconds())-1)
	if h.phase != "cooling" {
		t.Fatalf("it started early: %q", h.phase)
	}
	seconds(h, 1)
	if h.phase != "load" {
		t.Fatalf("it never started: %q", h.phase)
	}
	h, _ = warmed(run, 0, 70, 71)
	h.resting = func() (float64, bool) { return 66, false }
	h.start()
	if h.update(press(tea.KeyEnter)); h.phase != "load" {
		t.Errorf("Enter didn't start it: %q", h.phase)
	}

	// A computer without a fan: said once, and the load ends when the sensors have shown the heat
	h, o = warmed(run, 0, 44, 47, 49, 52, 55, 58, 61, 64, 67, 70)
	h.start()
	seconds(h, 1)
	h.update(press('n'))
	if !o.found.NoFan || h.phase != "load" || !strings.Contains(h.view(90, 30), "No fan, as you say") {
		t.Fatalf("no fan: %+v, phase %q", o.found, h.phase)
	}
	seconds(h, 2)
	if h.phase != "after" || len(o.found.Samples) != 3 {
		t.Fatalf("phase %q after %d readings", h.phase, len(o.found.Samples))
	}
	seconds(h, int(deep.CoolAfter.Seconds()))
	if fans, _ := run.Get("fans"); h.phase != "shown" || fans.State != report.Works || !strings.Contains(fans.Note, "no fan") {
		t.Errorf("phase %q, fans %+v", h.phase, fans)
	}

	// Leaving in the middle of the load ends it
	h, o = warmed(run, 0, 50, 60, 68, 70, 71)
	h.start()
	seconds(h, 2)
	m := &model{run: run, summary: &summary{run: run}, failed: map[string]string{}, steps: []step{h, &summary{run: run}}}
	m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	if o.stopped == 0 {
		t.Error("the load runs on after the step was skipped")
	}
}
