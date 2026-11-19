package guided

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// touchscreen counts taps on the screen: a finger's tap reaches the terminal as a click
// where it lands.
type touchscreen struct {
	run    *report.Run
	device string
	taps   map[[2]int]bool
	over   bool
}

const tapsWanted = 3

func newTouchscreen(run *report.Run, device string) *touchscreen {
	return &touchscreen{run: run, device: device, taps: map[[2]int]bool{}}
}

func (t *touchscreen) name() string     { return "Touchscreen" }
func (t *touchscreen) start() tea.Cmd   { return nil }
func (t *touchscreen) done() bool       { return t.over }
func (t *touchscreen) wantsMouse() bool { return true }

func (t *touchscreen) keys() string {
	if len(t.taps) >= tapsWanted {
		return "Enter to go on"
	}
	return "Tap with a finger, not the touchpad · n if tapping does nothing · a if it has no touchscreen · s to skip"
}

func (t *touchscreen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			t.taps[[2]int{msg.X, msg.Y}] = true
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter", "space":
			if len(t.taps) >= tapsWanted {
				t.run.Set("touchscreen", report.Works, report.Measured, fmt.Sprintf("%s: %d taps registered in different places", t.device, len(t.taps)), "")
				t.over = true
			}
		case "n":
			t.run.Set("touchscreen", report.Broken, report.Owner, t.device, "Tapping the screen does nothing.")
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
	seen := min(len(t.taps), tapsWanted)
	dots := good.Render(strings.Repeat("● ", seen)) + dim.Render(strings.Repeat("○ ", tapsWanted-seen))
	text := wrap("Now the touchscreen. Tap this window with a finger in three different places.", width) + "\n\n  " + dots
	if seen >= tapsWanted {
		text += "\n\n" + good.Render("The touchscreen works.") + " Press Enter to go on."
	}
	return text
}
