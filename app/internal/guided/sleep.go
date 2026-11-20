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

// settleMsg is one look, after waking, at what has come back so far.
type settleMsg struct{}

// How long Wi-Fi and the audio processor get to come back after waking. Wi-Fi often needs
// ten or twenty seconds to find its network again, which is slow but not broken.
const settleLimit = 60 * time.Second

// asleepMsg says how asking the system for sleep ended, and how long the asking took.
type asleepMsg struct {
	said string
	err  error
	took time.Duration
}

// sleep waits for the computer to sleep and wake, then looks at what came back. The owner closes
// and opens the lid, or, where there is no lid to close, the step asks the system for sleep itself.
type sleep struct {
	// command asks the system for sleep. Nil where the owner closes the lid.
	command  func() (string, error)
	run      *report.Run
	sys      machine.System
	wifi     string
	wifiWas  bool
	phase    string // intro, waiting, settling, ask, note
	ok       int64
	failed   int64
	problems []string
	// woke is when the wake was noticed, and back what came back and how long after it.
	woke time.Time
	back []string
	// clock is the time, which tests set.
	clock func() time.Time
	ask   choice
	note  line
	over  bool
}

func newSleep(run *report.Run, facts probe.Facts) *sleep {
	return &sleep{run: run, sys: machine.Real(), wifi: facts.Wifi, phase: "intro", clock: time.Now,
		ask:  choice{question: "Did it wake up to the lock screen without trouble?", options: []string{"Yes", "It woke, but something was wrong", "It never went to sleep"}},
		note: line{prompt: "What was wrong after waking? (Enter to leave it empty)"}}
}

// newSleepAsked is the step for a computer with no lid to close, which is a desktop or mini PC, or
// a laptop whose lid switch wasn't found. It asks the system for sleep, and the owner wakes it.
func newSleepAsked(run *report.Run, facts probe.Facts, sys machine.System) *sleep {
	s := newSleep(run, facts)
	s.command = func() (string, error) { return sys.RunFor(30*time.Second, "systemctl", "suspend") }
	return s
}

// asleep asks the system for sleep, off the screen's own thread.
func (s *sleep) asleep() tea.Cmd {
	return func() tea.Msg {
		began := time.Now()
		said, err := s.command()
		return asleepMsg{said, err, time.Since(began)}
	}
}

func (s *sleep) name() string   { return "Sleep" }
func (s *sleep) start() tea.Cmd { return nil }
func (s *sleep) done() bool     { return s.over }

func (s *sleep) keys() string {
	switch s.phase {
	case "intro":
		return "Enter when you're ready · s to skip"
	case "waiting":
		if s.command != nil {
			return "Wake it with a key or the power button · s to skip"
		}
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
	if len(s.back) > 0 {
		text += ", " + strings.Join(s.back, ", ")
	}
	if len(s.problems) > 0 {
		text += ", but " + strings.Join(s.problems, " and ")
	}
	return text
}

// wifiUp and audioUp say whether each is back.
func (s *sleep) wifiUp() bool {
	return s.sys.Read("/sys/class/net/"+s.wifi+"/operstate") == "up"
}

func (s *sleep) audioState() string {
	for _, processor := range s.sys.Glob("/sys/class/remoteproc/remoteproc*") {
		if strings.Contains(s.sys.Read(processor+"/name"), "adsp") {
			return s.sys.Read(processor + "/state")
		}
	}
	return ""
}

func (s *sleep) settle() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return settleMsg{} })
}

func (s *sleep) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case sleepMsg:
		if s.phase != "waiting" {
			return nil
		}
		switch {
		case msg.failed > s.failed:
			began := "closing the lid"
			if s.command != nil {
				began = "asking for sleep"
			}
			s.run.Set("suspend", report.Broken, report.Measured, fmt.Sprintf("%s started a sleep that failed (the kernel's count of failed sleeps went from %d to %d)", began, s.failed, msg.failed), "")
			s.over = true
		case msg.ok > s.ok:
			// Awake. Now watch what comes back and time it, for up to a minute. How long Wi-Fi takes is
			// worth knowing, and one look too early would call a slow reconnect a failure
			s.phase, s.woke, s.back, s.problems = "settling", s.clock(), nil, nil
			return s.settle()
		default:
			return s.poll()
		}
	case asleepMsg:
		// A refusal comes at once. A command that ends late, or was given up on, only waited for the wake
		if ok, failed := s.counts(); s.phase == "waiting" && msg.err != nil && msg.took < 5*time.Second && ok == s.ok && failed == s.failed {
			why := strings.TrimSpace(msg.said)
			if why == "" {
				why = msg.err.Error()
			}
			s.run.Set("suspend", report.Broken, report.Measured, "systemctl suspend was refused ("+why+")", "")
			s.over = true
		}
	case settleMsg:
		if s.phase != "settling" {
			return nil
		}
		took := s.clock().Sub(s.woke)
		wifi := !s.wifiWas || s.wifiUp()
		audio := s.audioState()
		if (!wifi || (audio != "" && audio != "running")) && took < settleLimit {
			return s.settle()
		}
		if s.wifiWas {
			if wifi {
				s.back = append(s.back, fmt.Sprintf("Wi-Fi was back %.0f s after waking", took.Seconds()))
			} else {
				s.problems = append(s.problems, fmt.Sprintf("Wi-Fi hadn't reconnected %.0f s after waking", took.Seconds()))
			}
		}
		switch audio {
		case "running":
			s.back = append(s.back, "the audio processor is running")
		case "":
		default:
			s.problems = append(s.problems, "the audio processor was "+audio+" after waking")
		}
		s.phase = "ask"
	case tea.KeyPressMsg:
		switch s.phase {
		case "intro", "waiting":
			switch msg.String() {
			case "enter", "space":
				if s.phase == "intro" {
					s.ok, s.failed = s.counts()
					s.wifiWas = s.wifi != "" && s.sys.Read("/sys/class/net/"+s.wifi+"/operstate") == "up"
					s.phase = "waiting"
					if s.command != nil {
						return tea.Batch(s.asleep(), s.poll())
					}
					return s.poll()
				}
			case "s":
				s.over = true
			}
		case "ask":
			switch s.ask.update(msg) {
			case 0:
				// What was measured counts too, so a "yes" with Wi-Fi still down is only partly working
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
		if s.command != nil {
			return wrap("Now sleep. When you press Enter, the computer goes to sleep. Wait about twenty seconds, then wake it with a key on the keyboard, or with the power button if a key doesn't wake it, and unlock. This screen will be here when you're back, and it then checks that Wi-Fi and sound came back too.", width) + "\n\n" +
				soft.Render(wrap("If the computer restarts instead of waking up, just run the test again. It knows what happened and keeps your answers.", width))
		}
		return wrap("Now sleep. When you press Enter, close the lid, wait about twenty seconds, open it and unlock. This screen will be here when you're back, and it then checks that Wi-Fi and sound came back too.", width) + "\n\n" +
			soft.Render(wrap("If the computer restarts instead of waking up, just run the test again. It knows what happened and keeps your answers.", width))
	case "waiting":
		if s.command != nil {
			return accent.Render("● Going to sleep. Wait twenty seconds, then wake it with a key or the power button, and unlock")
		}
		return accent.Render("● Close the lid now, wait twenty seconds, then open it and unlock")
	case "settling":
		waiting := "sound"
		if s.wifiWas && !s.wifiUp() {
			waiting = "Wi-Fi"
		}
		return good.Render("✓ It slept and woke.") + fmt.Sprintf(" Waiting for %s to come back, %.0f s so far (up to a minute)…", waiting, s.clock().Sub(s.woke).Seconds())
	case "note":
		return s.note.view(width)
	}
	return s.ask.view(width) + "\n\n" + soft.Render(wrap("The test measured that it "+s.evidence()+".", width))
}
