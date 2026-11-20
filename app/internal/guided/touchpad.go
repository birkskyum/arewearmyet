package guided

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// touchpad watches the pointer in the terminal: moving, clicking and scrolling each light up
// as they happen, so nothing has to be judged from memory.
type touchpad struct {
	run    *report.Run
	device string
	// places counts the cells the pointer has been over; a few of them is a movement.
	places  map[[2]int]bool
	clicked bool
	// wheel holds the scroll directions seen. Which finger direction is "up" depends on the
	// natural scrolling setting, so the test asks for one way and the other, not up and down.
	wheel map[tea.MouseButton]bool
	// nothing asks what the owner sees on the desktop when the terminal saw no pointer at all.
	nothing *choice
	over    bool
}

func newTouchpad(run *report.Run, device string) *touchpad {
	return &touchpad{run: run, device: device, places: map[[2]int]bool{}, wheel: map[tea.MouseButton]bool{}}
}

func (t *touchpad) name() string     { return "Touchpad" }
func (t *touchpad) start() tea.Cmd   { return nil }
func (t *touchpad) done() bool       { return t.over }
func (t *touchpad) wantsMouse() bool { return t.nothing == nil }

func (t *touchpad) moved() bool    { return len(t.places) >= 6 }
func (t *touchpad) scrolled() int  { return len(t.wheel) }
func (t *touchpad) complete() bool { return t.moved() && t.clicked && t.scrolled() >= 2 }

func (t *touchpad) keys() string {
	switch {
	case t.nothing != nil:
		return choiceKeys
	case t.complete():
		return "Enter to go on"
	}
	return "Use the touchpad, not a mouse · n if you tried and what isn't lit doesn't work · s to skip"
}

// seen and unseen list what registered and what didn't, in words for the result.
func (t *touchpad) seen() (seen, unseen []string) {
	add := func(ok bool, what string) {
		if ok {
			seen = append(seen, what)
		} else {
			unseen = append(unseen, what)
		}
	}
	add(t.moved(), "moving the pointer")
	add(t.clicked, "clicking")
	add(t.scrolled() >= 1, "two-finger scrolling")
	if t.scrolled() == 1 {
		unseen = append(unseen, "scrolling the other way")
	}
	return seen, unseen
}

func (t *touchpad) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.MouseMotionMsg:
		t.places[[2]int{msg.X, msg.Y}] = true
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			t.clicked = true
		}
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp || msg.Button == tea.MouseWheelDown {
			t.wheel[msg.Button] = true
		}
	case tea.KeyPressMsg:
		if t.nothing != nil {
			switch t.nothing.update(msg) {
			case 0:
				t.run.Set("touchpad", report.Works, report.Owner, t.device+", the terminal reported no pointer events, the owner says it works on the desktop", "")
				t.over = true
			case 1:
				t.run.Set("touchpad", report.Broken, report.Owner, t.device, "The touchpad doesn't move the pointer.")
				t.over = true
			case 2:
				t.nothing = nil
			}
			return nil
		}
		seen, unseen := t.seen()
		switch msg.String() {
		case "enter", "space":
			if t.complete() {
				t.run.Set("touchpad", report.Works, report.Measured, t.device+", moving the pointer, clicking and two-finger scrolling both ways registered", "")
				t.over = true
			}
		case "n":
			if len(seen) == 0 {
				// A terminal that doesn't report the pointer would look the same as a dead touchpad
				t.nothing = &choice{question: "Nothing registered here. On the desktop, does the touchpad move the pointer?", options: []string{"Yes, it works there", "No, it does nothing", "Go back and try again"}}
				return nil
			}
			t.run.Set("touchpad", report.Partial, report.Owner, t.device+" registered "+strings.Join(seen, ", "), "No response to "+strings.Join(unseen, ", "))
			t.over = true
		case "s":
			t.over = true
		}
	}
	return nil
}

func (t *touchpad) view(width, height int) string {
	if t.nothing != nil {
		return t.nothing.view(width)
	}
	item := func(ok bool, label string) string {
		// A long item wraps under its own text, not under the mark
		lines := strings.Split(wrap(label, width-4), "\n")
		mark, style := "  ○ ", soft
		if ok {
			mark, style = "  ✓ ", good
		}
		text := style.Render(mark + lines[0])
		for _, more := range lines[1:] {
			text += "\n" + style.Render("    "+more)
		}
		return text
	}
	scroll := "Scroll with two fingers, one way and then the other"
	if t.scrolled() == 1 {
		scroll = "Scroll with two fingers. One way seen, now the other way"
	}
	lines := []string{
		wrap("Now the touchpad. Do each of these anywhere in this window, and it lights up when the computer sees it.", width),
		"",
		item(t.moved(), "Move the pointer around"),
		item(t.clicked, "Click, by pressing the touchpad or tapping it"),
		item(t.scrolled() >= 2, scroll),
	}
	if t.complete() {
		lines = append(lines, "", good.Render("The touchpad works.")+" Press Enter to go on.")
	}
	return strings.Join(lines, "\n")
}
