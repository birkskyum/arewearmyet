package guided

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// sleepMsg is the kernel's count of sleeps that worked and that failed.
type sleepMsg struct{ ok, failed int64 }

// awakeMsg comes a while after waking, with what didn't come back.
type awakeMsg struct{ problems []string }

// sleep waits for the owner to close and open the lid, then looks at what came back.
type sleep struct {
	run      *report.Run
	sys      machine.System
	wifi     string
	wifiWas  bool
	phase    string // intro, waiting, settling, ask, note
	ok       int64
	failed   int64
	problems []string
	ask      choice
	note     line
	over     bool
}

func newSleep(run *report.Run, facts probe.Facts) *sleep {
	return &sleep{run: run, sys: machine.Real(), wifi: facts.Wifi, phase: "intro",
		ask:  choice{question: "Did it wake up to the lock screen without trouble?", options: []string{"Yes", "It woke, but something was wrong", "It never went to sleep"}},
		note: line{prompt: "What was wrong after waking? (Enter to leave it empty)"}}
}

func (s *sleep) name() string   { return "Sleep" }
func (s *sleep) start() tea.Cmd { return nil }
func (s *sleep) done() bool     { return s.over }

func (s *sleep) keys() string {
	switch s.phase {
	case "intro":
		return "Enter when you're ready · s to skip"
	case "waiting":
		return "Close the lid now · s to skip"
	case "settling":
		return "Checking what came back…"
	case "note":
		return "Type, then Enter"
	}
	return choiceKeys
}

func (s *sleep) counts() (ok, failed int64) {
	ok, _ = s.sys.Int("/sys/power/suspend_stats/success")
	failed, _ = s.sys.Int("/sys/power/suspend_stats/fail")
	return ok, failed
}

func (s *sleep) poll() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		ok, failed := s.counts()
		return sleepMsg{ok, failed}
	})
}

func (s *sleep) evidence() string {
	text := "slept and woke in " + s.sys.Read("/sys/power/mem_sleep")
	if len(s.problems) > 0 {
		return text + ", but " + strings.Join(s.problems, " and ")
	}
	return text + "; Wi-Fi and the audio processor came back"
}

func (s *sleep) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case sleepMsg:
		if s.phase != "waiting" {
			return nil
		}
		if msg.failed > s.failed || msg.ok > s.ok {
			report.ClearPending()
		}
		switch {
		case msg.failed > s.failed:
			s.run.Set("suspend", report.Broken, report.Measured, fmt.Sprintf("closing the lid started a sleep that failed (the kernel's count of failed sleeps went from %d to %d)", s.failed, msg.failed), "")
			s.over = true
		case msg.ok > s.ok:
			// Give Wi-Fi and the audio processor a moment to come back before looking
			s.phase = "settling"
			return tea.Tick(12*time.Second, func(time.Time) tea.Msg {
				var problems []string
				if s.wifiWas && s.sys.Read("/sys/class/net/"+s.wifi+"/operstate") != "up" {
					problems = append(problems, "Wi-Fi hadn't reconnected 12 seconds after waking")
				}
				for _, processor := range s.sys.Glob("/sys/class/remoteproc/remoteproc*") {
					if name := s.sys.Read(processor + "/name"); strings.Contains(name, "adsp") && s.sys.Read(processor+"/state") != "running" {
						problems = append(problems, "the audio processor was "+s.sys.Read(processor+"/state")+" after waking")
					}
				}
				return awakeMsg{problems}
			})
		default:
			return s.poll()
		}
	case awakeMsg:
		s.problems, s.phase = msg.problems, "ask"
	case tea.KeyPressMsg:
		switch s.phase {
		case "intro", "waiting":
			switch msg.String() {
			case "enter", "space":
				if s.phase == "intro" {
					s.ok, s.failed = s.counts()
					s.wifiWas = s.wifi != "" && s.sys.Read("/sys/class/net/"+s.wifi+"/operstate") == "up"
					s.phase = "waiting"
					// Kept on disk while the lid is closed: if the computer restarts instead of
					// waking, the next run finds this and records the restart
					_ = report.SavePending("Sleep", s.sys.Read("/sys/power/mem_sleep"), s.run)
					return s.poll()
				}
			case "s":
				report.ClearPending()
				s.over = true
			}
		case "ask":
			switch s.ask.update(msg) {
			case 0:
				// What was measured counts too: a "yes" with Wi-Fi still down is only partly working
				state := report.Works
				if len(s.problems) > 0 {
					state = report.Partial
				}
				s.run.Set("suspend", state, report.Owner, s.evidence(), "")
				s.over = true
			case 1:
				s.phase = "note"
			case 2:
				s.run.Set("suspend", report.Broken, report.Owner, "the kernel counted a sleep, but the owner saw none", "")
				s.over = true
			}
		case "note":
			if s.note.update(msg) {
				s.run.Set("suspend", report.Partial, report.Owner, s.evidence(), s.note.value())
				s.over = true
			}
		}
	}
	return nil
}

func (s *sleep) view(width, height int) string {
	switch s.phase {
	case "intro":
		return wrap("Now sleep. When you press Enter, close the lid, wait about twenty seconds, open it and unlock. This screen will be here when you're back, and it then checks that Wi-Fi and sound came back too.", width) + "\n\n" +
			dim.Render(wrap("If the computer restarts instead of waking up, just run the test again: it knows what happened and keeps your answers.", width))
	case "waiting":
		return accent.Render("● Close the lid now, wait twenty seconds, then open it and unlock")
	case "settling":
		return good.Render("✓ It slept and woke.") + " Giving Wi-Fi and sound a moment to come back…"
	case "note":
		return s.note.view(width)
	}
	return s.ask.view(width) + "\n\n" + dim.Render(wrap("Measured: "+s.evidence()+".", width))
}
