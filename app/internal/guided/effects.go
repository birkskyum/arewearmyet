package guided

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

// levels are what the volume and brightness keys change.
type levels struct {
	volume     float64
	muted      bool
	brightness int64
}

type levelsMsg levels

// effect is one key checked by what it does: the desktop keeps the volume and brightness
// keys for itself, so no program sees them pressed, but anyone can see the volume move.
type effect struct {
	label string
	// happened says whether the levels changed the way the key should change them.
	happened func(before, now levels) bool
	// outcome is "" while waiting, then seen, failed (pressed, nothing happened) or skipped.
	outcome string
}

// effects asks for the volume, mute and brightness keys one at a time and watches for each
// one's effect.
type effects struct {
	run       *report.Run
	backlight string
	items     []effect
	at        int
	before    levels
	now       levels
	started   bool
	// span is the brightness at its lowest and highest during the step, as evidence.
	low, high int64
}

func newEffects(run *report.Run, facts probe.Facts) *effects {
	e := &effects{run: run, backlight: facts.Backlight}
	if _, _, ok := tools.Volume(); ok {
		e.items = append(e.items,
			effect{label: "Volume up", happened: func(b, n levels) bool { return n.volume > b.volume }},
			effect{label: "Volume down", happened: func(b, n levels) bool { return n.volume < b.volume }},
			effect{label: "Mute", happened: func(b, n levels) bool { return n.muted != b.muted }},
			effect{label: "Mute again, to turn the sound back on", happened: func(b, n levels) bool { return n.muted != b.muted }},
		)
	}
	if e.backlight != "" {
		e.items = append(e.items,
			effect{label: "Brightness down", happened: func(b, n levels) bool { return n.brightness < b.brightness }},
			effect{label: "Brightness up", happened: func(b, n levels) bool { return n.brightness > b.brightness }},
		)
	}
	return e
}

func (e *effects) read() levels {
	var now levels
	now.volume, now.muted, _ = tools.Volume()
	if e.backlight != "" {
		now.brightness, _ = machine.Real().Int(e.backlight + "/brightness")
	}
	return now
}

func (e *effects) poll() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return levelsMsg(e.read()) })
}

func (e *effects) name() string { return "Volume and brightness keys" }
func (e *effects) done() bool   { return e.at >= len(e.items) }
func (e *effects) keys() string {
	return "Press the key it asks for · n if you pressed it and nothing happened · s to skip it"
}

func (e *effects) start() tea.Cmd {
	if e.done() {
		return nil
	}
	e.before = e.read()
	e.now = e.before
	e.low, e.high = e.before.brightness, e.before.brightness
	e.started = true
	return e.poll()
}

// next moves to the following key, and writes the results after the last one.
func (e *effects) next(outcome string) {
	e.items[e.at].outcome = outcome
	e.at++
	e.before = e.now
	if e.done() {
		e.finish()
	}
}

func (e *effects) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case levelsMsg:
		if e.done() {
			return nil
		}
		e.now = levels(msg)
		e.low, e.high = min(e.low, e.now.brightness), max(e.high, e.now.brightness)
		if e.items[e.at].happened(e.before, e.now) {
			e.next("seen")
		}
		if e.done() {
			return nil
		}
		return e.poll()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "n":
			e.next("failed")
		case "s":
			e.next("skipped")
		}
	}
	return nil
}

// finish turns what was seen into results: the brightness keys are the backlight check, and
// a volume or mute key without effect makes the keyboard only partly working.
func (e *effects) finish() {
	var deadKeys []string
	bright := map[string]int{}
	for _, item := range e.items {
		if strings.HasPrefix(item.label, "Brightness") {
			bright[item.outcome]++
		} else if item.outcome == "failed" {
			deadKeys = append(deadKeys, strings.ToLower(strings.Split(item.label, ",")[0]))
		}
	}
	span := fmt.Sprintf("the backlight moved between %d and %d", e.low, e.high)
	switch {
	case bright["seen"] == 2:
		e.run.Set("backlight", report.Works, report.Measured, "both brightness keys work: "+span, "")
	case bright["seen"] == 1 && bright["failed"] == 1:
		e.run.Set("backlight", report.Partial, report.Measured, "only one brightness key works: "+span, "")
	case bright["failed"] == 2:
		e.run.Set("backlight", report.Broken, report.Owner, fmt.Sprintf("both brightness keys pressed, the backlight stayed at %d", e.now.brightness), "")
	}
	if len(deadKeys) == 0 {
		return
	}
	keyboard, _ := e.run.Get("keyboard")
	note := "No effect from: " + strings.Join(deadKeys, ", ")
	if keyboard.Note != "" {
		note = keyboard.Note + ". " + note
	}
	e.run.Set("keyboard", report.Partial, report.Owner, keyboard.Evidence, note)
}

func (e *effects) view(width, height int) string {
	intro := "The desktop handles the volume and brightness keys itself, so these are checked by what they do. Press the key marked with the arrow."
	var out strings.Builder
	out.WriteString(wrap(intro, width) + "\n\n")
	for i, item := range e.items {
		switch {
		case item.outcome == "seen":
			out.WriteString(good.Render("  ✓ "+item.label) + "\n")
		case item.outcome == "failed":
			out.WriteString(bad.Render("  ✗ "+item.label+": nothing happened") + "\n")
		case item.outcome == "skipped":
			out.WriteString(dim.Render("  – "+item.label+": skipped") + "\n")
		case i == e.at:
			out.WriteString(accent.Render("  › "+item.label) + "\n")
		default:
			out.WriteString(dim.Render("    "+item.label) + "\n")
		}
	}
	state := fmt.Sprintf("Volume %d%%", int(e.now.volume*100+0.5))
	if e.now.muted {
		state += ", muted"
	}
	if e.backlight != "" {
		state += fmt.Sprintf(" · backlight %d", e.now.brightness)
	}
	return out.String() + "\n" + dim.Render(state)
}
