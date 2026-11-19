package guided

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// lidTick is the lid test's look at the switch.
type lidTick struct{}

// lid asks for the lid to be closed and opened, and watches the switch say so. The computer
// may sleep while the lid is down, and the test doesn't run then, so what it missed is read
// from the system's own log of the lid once it is back.
type lid struct {
	run   *report.Run
	sys   machine.System
	phase string // closing, opening
	began time.Time
	// last is when the test last looked, by which a sleep in between shows.
	last  time.Time
	slept time.Duration
	// now gives the time, which tests set.
	now func() time.Time
	// closed and opened are set once the switch has reported each.
	closed, opened bool
	// blind is set when neither the switch's state nor its log can be read here.
	blind bool
	over  bool
}

func newLid(run *report.Run, sys machine.System) *lid {
	return &lid{run: run, sys: sys, phase: "closing", now: time.Now}
}

func (l *lid) name() string { return "Lid" }
func (l *lid) done() bool   { return l.over }

func (l *lid) keys() string {
	return "Close the lid, count to three, open it · n if you did and nothing shows here · s to skip"
}

func (l *lid) tick() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return lidTick{} })
}

// state asks the system whether the lid is down right now.
func (l *lid) state() (down, ok bool) {
	out := strings.TrimSpace(l.sys.Run("busctl", "get-property", "org.freedesktop.login1", "/org/freedesktop/login1", "org.freedesktop.login1.Manager", "LidClosed"))
	return out == "b true", out == "b true" || out == "b false"
}

// logged reads what the system wrote down about the lid since the test began: whether it
// closed, and whether it opened after that.
func (l *lid) logged() (closed, opened, ok bool) {
	out, err := l.sys.RunFor(5*time.Second, "journalctl", "-b", "-u", "systemd-logind", "--no-pager", "-o", "cat", "--since", fmt.Sprintf("@%d", l.began.Unix()))
	if err != nil {
		return false, false, false
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "Lid closed"):
			closed, opened = true, false
		case strings.Contains(line, "Lid opened") && closed:
			opened = true
		}
	}
	return closed, opened, true
}

func (l *lid) start() tea.Cmd {
	l.began = l.now()
	l.last = l.began
	_, live := l.state()
	_, _, kept := l.logged()
	// With neither to read, there is nothing here to measure with
	if l.blind = !live && !kept; l.blind {
		l.over = true
		return nil
	}
	return l.tick()
}

func (l *lid) finish() {
	evidence := "the lid switch reported closed and then open again"
	if l.slept > 0 {
		evidence += fmt.Sprintf("; the computer slept %.0f s while it was down", l.slept.Seconds())
	}
	l.run.Set("lid", report.Works, report.Measured, evidence, "")
	l.over = true
}

func (l *lid) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case lidTick:
		if l.over {
			return nil
		}
		// A long gap between two looks is the computer having slept with the lid down
		now := l.now()
		if gap := now.Sub(l.last); gap > 3*time.Second {
			l.slept += gap
		}
		l.last = now
		if down, ok := l.state(); ok {
			switch {
			case down:
				l.closed, l.phase = true, "opening"
			case l.closed:
				l.opened = true
			}
		}
		// What happened while the test couldn't look is in the log
		if !l.opened {
			if closed, opened, ok := l.logged(); ok {
				l.closed, l.opened = l.closed || closed, opened
				if l.closed {
					l.phase = "opening"
				}
			}
		}
		if l.closed && l.opened {
			l.finish()
			return nil
		}
		return l.tick()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "s":
			l.over = true
		case "n":
			if l.closed {
				l.run.Set("lid", report.Partial, report.Owner, "the lid switch reported closed, and never open again", "Closing the lid is noticed, opening it isn't.")
			} else {
				l.run.Set("lid", report.Broken, report.Owner, "the lid was closed and opened, and the lid switch reported nothing", "")
			}
			l.over = true
		}
	}
	return nil
}

func (l *lid) view(width, height int) string {
	text := "Now the lid. Close it, count to three, and open it again. The computer may go to sleep while it is down: unlock it if it asks, and this screen will be here."
	if l.phase == "opening" {
		return good.Render("✓ The lid switch reported closed.") + " " + accent.Render("Now open it.")
	}
	return wrap(text, width) + "\n\n  ○ Waiting for the lid to close…"
}
