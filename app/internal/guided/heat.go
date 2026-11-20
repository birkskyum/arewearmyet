package guided

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/deep"
	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// heatTick is the temperature test's second.
type heatTick struct{}

// warmer is the temperature test under way, which tests replace with readings of their own.
type warmer interface {
	Step() (deep.Sample, bool)
	Heard()
	NoFan()
	Stop()
	Cooled() float64
	Heat() deep.Heat
}

// heat is the temperature test. It lets a warm computer cool, sets every core to work and
// shows the hottest sensor as it climbs, until a fan is seen starting or the time is up. A
// fan's speed is read where there is a sensor for it, and seen by the temperature it brings
// down where there isn't. The owner, when there is one, says the moment they hear it.
type heat struct {
	run   *report.Run
	phase string // cooling, load, after, ask, shown
	// resting reads the temperature before the load, and begin starts the load.
	resting func() (float64, bool)
	begin   func() warmer
	warming warmer
	// now is the last reading, waited the seconds spent cooling before or after the load.
	now      deep.Sample
	waited   int
	results  []report.Result
	handsOff bool
	// byEar is set on a computer with no fan speed sensor, where only the owner can say when
	// the fan starts. The step is then one of those done with the owner, under its own name.
	// sensor is set where the fan's speed can be read, and nothing is asked.
	byEar, sensor bool
	ask           choice
	// second is the time between readings and linger how long the result stays up.
	second, linger time.Duration
	over           bool
}

func newHeat(run *report.Run, sys machine.System, dir string) *heat {
	return &heat{run: run, second: time.Second, linger: 2 * linger,
		resting: func() (float64, bool) { return deep.Resting(sys) },
		begin:   func() warmer { return deep.StartWarming(sys, dir) }}
}

// The step's two names, measured alone or with the owner listening for the fan.
const (
	heatAlone = "Temperatures"
	heatByEar = "Temperatures and fan"
)

func (h *heat) name() string {
	if h.byEar {
		return heatByEar
	}
	return heatAlone
}
func (h *heat) done() bool          { return h.over }
func (h *heat) alone(handsOff bool) { h.handsOff = handsOff || h.sensor }

// cleanup ends the load when the test is left in the middle of it.
func (h *heat) cleanup() {
	if h.warming != nil {
		h.warming.Stop()
	}
}

func (h *heat) keys() string {
	switch h.phase {
	case "cooling":
		return "Waiting for it to cool · Enter to start now · s to skip"
	case "load":
		if h.handsOff {
			return "Measuring, nothing for you to do · s to end the load"
		}
		return "f the moment you hear the fan speed up · n if this computer has no fan · s to end the load"
	case "after":
		return "Watching it cool"
	case "ask":
		return choiceKeys
	}
	return "Goes on by itself in a moment · Enter to go on now"
}

func (h *heat) tick() tea.Cmd {
	return tea.Tick(h.second, func(time.Time) tea.Msg { return heatTick{} })
}

func (h *heat) start() tea.Cmd {
	h.phase, h.waited, h.warming, h.results = "cooling", 0, nil, nil
	// A computer that is warm from what it did before cools first, so the load has room to show
	if degrees, cool := h.resting(); cool {
		h.load()
	} else {
		h.now = deep.Sample{Hot: degrees}
	}
	return h.tick()
}

func (h *heat) load() {
	h.phase, h.warming = "load", h.begin()
	h.now = deep.Sample{Hot: h.warming.Heat().Rest, Rate: 1}
}

// rest ends the load and watches the sensors follow the heat down.
func (h *heat) rest() {
	h.warming.Stop()
	h.phase, h.waited = "after", 0
}

// sawFan says whether anything has shown a fan, or that there is none.
func sawFan(found deep.Heat) bool {
	return found.Kicked != nil || found.Heard != nil || found.NoFan || found.Fan != ""
}

// conclude writes the result and shows it.
func (h *heat) conclude() tea.Cmd {
	h.results = h.warming.Heat().Report()
	for _, result := range h.results {
		keep(h.run, result)
	}
	h.phase = "shown"
	name := h.name()
	return tea.Tick(h.linger, func(time.Time) tea.Msg { return goOnMsg{name} })
}

func (h *heat) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case heatTick:
		switch h.phase {
		case "cooling":
			h.waited++
			degrees, cool := h.resting()
			h.now = deep.Sample{Hot: degrees}
			if cool || time.Duration(h.waited)*time.Second >= deep.CoolFirst {
				h.load()
			}
		case "load":
			sample, over := h.warming.Step()
			h.now = sample
			// With no fan to wait for, the load has done its part once the sensors have shown the heat
			found := h.warming.Heat()
			if over || found.NoFan && found.Peak-found.Rest >= 10 {
				h.rest()
			}
		case "after":
			h.waited++
			h.now.Hot = h.warming.Cooled()
			if time.Duration(h.waited)*time.Second >= deep.CoolAfter {
				// Only a person can say whether a fan that showed in no reading ran, or isn't there
				if found := h.warming.Heat(); !sawFan(found) && found.Sensors > 0 && !h.handsOff {
					h.phase = "ask"
					h.ask = choice{question: fmt.Sprintf("Nothing in the readings showed a fan starting, up to %.0f °C. Did you hear one?", found.Peak),
						options: []string{"Yes, it sped up", "No, I heard no fan", "This computer has no fan", "I don't know"}}
					return nil
				}
				return h.conclude()
			}
		default:
			return nil
		}
		return h.tick()
	case goOnMsg:
		if msg.name == h.name() && h.phase == "shown" {
			h.over = true
		}
	case tea.KeyPressMsg:
		key := msg.String()
		switch h.phase {
		case "cooling":
			switch key {
			case "enter", "space":
				h.load()
			case "s":
				h.over = true
			}
		case "load":
			switch {
			case key == "s":
				h.rest()
			case key == "f" && !h.handsOff:
				// Heard, which is what the load was for
				h.warming.Heard()
				h.rest()
			case key == "n" && !h.handsOff:
				h.warming.NoFan()
			}
		case "ask":
			switch h.ask.update(msg) {
			case -1:
				return nil
			case 0:
				h.warming.Heard()
			case 2:
				h.warming.NoFan()
			}
			return h.conclude()
		case "shown":
			if key == "enter" || key == "space" {
				h.over = true
			}
		}
	}
	return nil
}

// curve draws the readings as a line of bars, from the temperature at rest up to the peak.
func curve(samples []deep.Sample, rest float64, width int) string {
	if len(samples) > width {
		samples = samples[len(samples)-width:]
	}
	top := rest + 10
	for _, sample := range samples {
		top = max(top, sample.Hot)
	}
	bars := []rune("▁▂▃▄▅▆▇█")
	var out strings.Builder
	for _, sample := range samples {
		at := int((sample.Hot - rest) / (top - rest) * float64(len(bars)-1))
		out.WriteRune(bars[max(0, min(at, len(bars)-1))])
	}
	return out.String()
}

func (h *heat) view(width, height int) string {
	switch h.phase {
	case "cooling":
		return accent.Render(wrap(fmt.Sprintf("○ Letting the computer cool first, now at %.0f °C", h.now.Hot), width)) + "\n\n" +
			wrap(fmt.Sprintf("It is warm from what it did before. The test starts once it is cooler, or in %.0f seconds at the latest, so the heat of the load shows.", (deep.CoolFirst-time.Duration(h.waited)*time.Second).Seconds()), width)
	case "ask":
		return h.readings(width) + "\n\n" + h.ask.view(width)
	case "shown":
		return showResults(h.results, width)
	}
	lines := []string{}
	if h.phase == "load" {
		busy := "Every processor core is working"
		if h.warming.Heat().Graphics {
			busy = "Every processor core and the GPU are working"
		}
		lines = append(lines, accent.Render(wrap(fmt.Sprintf("● %s… %.0f s of at most %.0f", busy, h.now.At.Seconds(), deep.MaxLoad.Seconds()), width)))
	} else {
		lines = append(lines, good.Render("✓ The load is over.")+fmt.Sprintf(" Cooling, now at %.0f °C", h.now.Hot))
	}
	lines = append(lines, "", h.readings(width))
	if h.phase == "load" && !h.handsOff {
		said := "Listen for the fan, and press f the moment you hear it speed up. The load ends as soon as a fan shows, in the readings or by your ear."
		if h.warming.Heat().NoFan {
			said = "No fan, as you say. The load ends once the sensors have shown the heat."
		}
		lines = append(lines, "", wrap(said, width))
	}
	return strings.Join(lines, "\n")
}

// readings is the temperature now, its curve, and what the test has seen so far.
func (h *heat) readings(width int) string {
	found := h.warming.Heat()
	lines := []string{bold.Render(fmt.Sprintf("%.0f °C", h.now.Hot)) + "  " + stateStyle(report.Partial).Render(curve(found.Samples, found.Rest, max(10, min(width-10, 60))))}
	lines = append(lines, wrap(fmt.Sprintf("Hottest of %d sensors, from %.0f °C at rest to %.0f °C so far.", found.Sensors, found.Rest, found.Peak), width))
	if found.Watts > 0 {
		lines = append(lines, wrap(fmt.Sprintf("Drawing up to %.0f W from the battery.", found.Watts), width))
	}
	if found.Slowest > 0 && found.Slowest < 0.75 {
		lines = append(lines, wrap(fmt.Sprintf("The processor was slowed to %.0f%% of its speed to stay cool.", found.Slowest*100), width))
	}
	if found.Fan != "" {
		lines = append(lines, wrap(fmt.Sprintf("The fan (%s) runs at %d rpm, from %d at rest.", found.Fan, h.now.Fan, found.FanRest), width))
	}
	if found.Kicked != nil {
		lines = append(lines, good.Render(wrap(fmt.Sprintf("✓ A fan started, as the temperature fell from %.0f to %.0f °C while the processor worked on at the same rate.", found.Kicked.From, found.Kicked.To), width)))
	}
	if found.Heard != nil {
		lines = append(lines, good.Render(wrap(fmt.Sprintf("✓ You heard the fan at %.0f °C, %.0f s in.", found.Heard.Hot, found.Heard.At.Seconds()), width)))
	}
	return strings.Join(lines, "\n")
}
