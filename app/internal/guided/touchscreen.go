package guided

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

type touchMsg struct{ count int64 }

// touchscreen counts touches by the touchscreen's own interrupt, which fires whenever a
// finger is on it. A terminal can't be asked: it gets a tap as a click at the pointer, or not
// at all. Where the interrupt can't be found, clicks in the terminal are counted instead.
type touchscreen struct {
	run    *report.Run
	sys    machine.System
	device string
	// line is the interrupt's folder under /sys/kernel/irq, or "".
	line string
	// last is the count at the latest look, and fired how often it rose since the step began.
	last    int64
	fired   int64
	touches int
	// touching is set while the count is rising; a touch ends when it has stopped for a moment.
	touching bool
	changed  time.Time
	clicks   map[[2]int]bool
	ask      *choice
	over     bool
	// clock is the time, which tests set.
	clock func() time.Time
}

const (
	touchesWanted = 3
	// A finger on the screen fires many times a second; this long without one is a lifted finger.
	touchGap = 350 * time.Millisecond
)

func newTouchscreen(run *report.Run, sys machine.System, device, inputDevice string) *touchscreen {
	t := &touchscreen{run: run, sys: sys, device: device, clicks: map[[2]int]bool{}, clock: time.Now}
	if inputDevice != "" {
		t.line = interruptLine(sys, inputDevice)
	}
	return t
}

func (t *touchscreen) name() string     { return "Touchscreen" }
func (t *touchscreen) done() bool       { return t.over }
func (t *touchscreen) wantsMouse() bool { return t.line == "" && t.ask == nil }

func (t *touchscreen) keys() string {
	if t.ask != nil {
		return choiceKeys
	}
	return "Touch the screen with a finger, lift it, and again · n if touching does nothing · a if it has no touchscreen · s to skip"
}

func (t *touchscreen) poll() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		count, _ := interrupts(t.sys, t.line)
		return touchMsg{count}
	})
}

func (t *touchscreen) start() tea.Cmd {
	if t.line == "" {
		return nil
	}
	if count, ok := interrupts(t.sys, t.line); ok {
		t.last = count
		return t.poll()
	}
	t.line = ""
	return nil
}

func (t *touchscreen) seen() int {
	if t.line == "" {
		return len(t.clicks)
	}
	return t.touches
}

func (t *touchscreen) evidence() string {
	if t.line == "" {
		return fmt.Sprintf("%s, %d taps reached the terminal as clicks in different places", t.device, len(t.clicks))
	}
	return fmt.Sprintf("%s, %d touches, its interrupt fired %d times", t.device, t.touches, t.fired)
}

// enough moves on to the one thing that can't be measured: whether the desktop put the touch
// where the finger was.
func (t *touchscreen) enough() {
	t.ask = &choice{question: fmt.Sprintf("The touchscreen registered %d touches. Did the desktop react where you touched, like focus or the pointer following your finger?", t.seen()),
		options: []string{"Yes, where I touched", "It reacted, but in the wrong place", "No, nothing on the screen reacted"}}
}

func (t *touchscreen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case touchMsg:
		if t.over || t.ask != nil || t.line == "" {
			return nil
		}
		now := t.clock()
		switch {
		case msg.count > t.last:
			t.fired += msg.count - t.last
			t.touching, t.changed = true, now
		case t.touching && now.Sub(t.changed) >= touchGap:
			t.touching = false
			t.touches++
		}
		t.last = msg.count
		if t.touches >= touchesWanted {
			t.enough()
			return nil
		}
		return t.poll()
	case tea.MouseClickMsg:
		if t.line == "" && t.ask == nil && msg.Button == tea.MouseLeft {
			t.clicks[[2]int{msg.X, msg.Y}] = true
			if len(t.clicks) >= touchesWanted {
				t.enough()
			}
		}
	case tea.KeyPressMsg:
		if t.ask != nil {
			switch t.ask.update(msg) {
			case 0:
				t.run.Set("touchscreen", report.Works, report.Measured, t.evidence(), "")
				t.over = true
			case 1:
				t.run.Set("touchscreen", report.Partial, report.Owner, t.evidence(), "Touches register, but the desktop reacts in the wrong place.")
				t.over = true
			case 2:
				t.run.Set("touchscreen", report.Partial, report.Owner, t.evidence(), "The touchscreen registers touches, but nothing on the desktop reacts to them.")
				t.over = true
			}
			return nil
		}
		switch msg.String() {
		case "n":
			if t.fired > 0 || t.touching {
				t.run.Set("touchscreen", report.Partial, report.Owner, t.evidence(), "Touching does nothing on the desktop, though the touchscreen's interrupt fires.")
			} else {
				t.run.Set("touchscreen", report.Broken, report.Owner, t.evidence(), "Touching the screen does nothing.")
			}
			t.over = true
		case "a":
			t.run.Set("touchscreen", report.Absent, report.Owner, "", "")
			t.over = true
		case "s":
			t.over = true
		}
	}
	return nil
}

func (t *touchscreen) view(width, height int) string {
	if t.ask != nil {
		return t.ask.view(width)
	}
	seen := min(t.seen(), touchesWanted)
	dots := good.Render(strings.Repeat("● ", seen)) + strings.Repeat("○ ", touchesWanted-seen)
	text := "Now the touchscreen. Touch the screen with a finger and lift it again, three times, anywhere."
	if t.line == "" {
		text = "Now the touchscreen. Tap this window with a finger in three different places."
	}
	state := ""
	if t.touching {
		state = "   " + accent.Render("touching")
	}
	return wrap(text, width) + "\n\n  " + dots + state
}
