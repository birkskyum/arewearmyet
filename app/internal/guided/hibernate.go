package guided

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// hibernatedMsg is one look, after hibernation was asked for, at whether the computer is back.
type hibernatedMsg struct{}

// askedMsg says how asking the system to hibernate ended, and how long the asking took.
type askedMsg struct {
	said string
	err  error
	took time.Duration
}

// What systemd's sleep program writes once a hibernation is over, and when one failed before the
// computer powered off.
const (
	hibernationReturned = "System returned from sleep operation 'hibernate'"
	hibernationFailed   = "Failed to put system to sleep"
)

// hibernate saves the session to the drive and powers off, and once the owner powers the computer
// on, checks that the session and what it uses came back. A session that doesn't come back ends in
// a fresh start, which the next run finds and records as what this step found.
type hibernate struct {
	run *report.Run
	sys machine.System
	// missing is why hibernation can't be tried here, or "" when it can.
	missing  string
	wifi     string
	wifiWas  bool
	phase    string // unavailable, setup, intro, waiting, settling, ask, note
	asked    time.Time
	woke     time.Time
	back     []string
	problems []string
	// clock is the time, and away how far the wall clock has moved past the monotonic one since a
	// time, which is how long the computer was off. Tests set both.
	clock func() time.Time
	away  func(since time.Time) time.Duration
	// command asks the system to hibernate, and began marks the saved test, so a fresh start
	// after it is known for a hibernation that didn't come back.
	command func() (string, error)
	began   func(on bool)
	ask     choice
	note    line
	over    bool
}

func newHibernate(run *report.Run, facts probe.Facts, sys machine.System) *hibernate {
	h := &hibernate{run: run, sys: sys, wifi: facts.Wifi, phase: "intro", clock: time.Now,
		away: func(since time.Time) time.Duration {
			now := time.Now()
			return now.Round(0).Sub(since.Round(0)) - now.Sub(since)
		},
		began: func(on bool) { _ = report.MarkBegan(on) },
		ask:   choice{question: "Did it come back to where you left off?", options: []string{"Yes, with everything as it was", "It came back, but something was wrong"}},
		note:  line{prompt: "What was wrong after it came back? (Enter to leave it empty)"}}
	h.command = func() (string, error) { return sys.RunFor(30*time.Second, "systemctl", "hibernate") }
	switch h.missing = hibernationMissing(sys); h.missing {
	case "":
	case noHibernation:
		h.phase = "unavailable"
	default:
		h.phase = "setup"
	}
	return h
}

const noHibernation = "the kernel offers no hibernation"

// hibernationMissing says what keeps this computer from hibernating, or "" when nothing does. The
// kernel has to offer it, the kernel line has to say where the saved session is found again, and
// the swap on the drive has to hold what is saved.
func hibernationMissing(sys machine.System) string {
	if !strings.Contains(" "+sys.Read("/sys/power/state")+" ", " disk ") {
		return noHibernation
	}
	if !strings.Contains(" "+sys.Read("/proc/cmdline"), " resume=") {
		return "hibernation isn't set up"
	}
	var swap int64
	for _, line := range strings.Split(sys.ReadAll("/proc/swaps"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] == "Filename" || strings.Contains(fields[0], "zram") {
			continue
		}
		size, _ := strconv.ParseInt(fields[2], 10, 64)
		swap += size * 1024
	}
	if image, ok := sys.Int("/sys/power/image_size"); ok && swap <= image {
		return "the swap on the drive is too small to hold the saved session"
	}
	return ""
}

func (h *hibernate) apart()         {}
func (h *hibernate) name() string   { return "Hibernation" }
func (h *hibernate) start() tea.Cmd { return nil }
func (h *hibernate) done() bool     { return h.over }

func (h *hibernate) keys() string {
	switch h.phase {
	case "unavailable", "setup":
		return "Enter to go on"
	case "intro":
		return "Enter to hibernate · s to skip"
	case "waiting":
		return "Enter if it never powered off · s to skip"
	case "settling":
		return "Checking what came back…"
	case "note":
		return "Type, then Enter"
	}
	return choiceKeys
}

// asking asks the system to hibernate, off the screen's own thread.
func (h *hibernate) asking() tea.Cmd {
	return func() tea.Msg {
		began := time.Now()
		said, err := h.command()
		return askedMsg{said, err, time.Since(began)}
	}
}

func (h *hibernate) poll() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return hibernatedMsg{} })
}

func (h *hibernate) settle() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return settleMsg{} })
}

// sleepLog is what systemd's sleep program has written since hibernation was asked for.
func (h *hibernate) sleepLog() string {
	text, _ := h.sys.RunFor(5*time.Second, "journalctl", "-b", "0", "-t", "systemd-sleep", "-o", "cat", "--no-pager", "-q", "--since", fmt.Sprintf("@%d", h.asked.Unix()))
	return text
}

func (h *hibernate) evidence() string {
	text := fmt.Sprintf("came back from hibernation %.0f s after it was asked for", h.clock().Round(0).Sub(h.asked.Round(0)).Seconds())
	if len(h.back) > 0 {
		text += ", " + strings.Join(h.back, ", ")
	}
	if len(h.problems) > 0 {
		text += ", but " + strings.Join(h.problems, " and ")
	}
	return text
}

// stop ends the step with a result, and the saved test no longer waits for a power-off.
func (h *hibernate) stop(state, how, evidence, note string) {
	h.began(false)
	h.run.Set("hibernate", state, how, evidence, note)
	h.over = true
}

func (h *hibernate) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case askedMsg:
		// A refusal comes at once, like too little swap. Otherwise the asking only queued it
		if h.phase == "waiting" && msg.err != nil && msg.took < 5*time.Second {
			why := strings.TrimSpace(msg.said)
			if why == "" {
				why = msg.err.Error()
			}
			h.stop(report.Broken, report.Measured, "systemctl hibernate was refused ("+why+")", "")
		}
	case hibernatedMsg:
		if h.phase != "waiting" {
			return nil
		}
		said := h.sleepLog()
		switch {
		case strings.Contains(said, hibernationReturned) || h.away(h.asked) > 10*time.Second:
			// Back. Now watch what comes back and time it, as after sleep
			h.began(false)
			h.phase, h.woke = "settling", h.clock()
			return h.settle()
		case strings.Contains(said, hibernationFailed):
			for _, line := range strings.Split(said, "\n") {
				if strings.Contains(line, hibernationFailed) {
					h.stop(report.Broken, report.Measured, "hibernating failed before the computer powered off ("+strings.TrimSpace(line)+")", "")
					break
				}
			}
			return nil
		}
		return h.poll()
	case settleMsg:
		if h.phase != "settling" {
			return nil
		}
		again, back, problems := cameBack(h.sys, h.wifi, h.wifiWas, h.clock().Sub(h.woke))
		if again {
			return h.settle()
		}
		h.back, h.problems = back, problems
		h.phase = "ask"
	case tea.KeyPressMsg:
		switch h.phase {
		case "unavailable", "setup":
			switch msg.String() {
			case "enter", "space", "s":
				if h.phase == "unavailable" {
					// The probe says so too, but a step run alone reports only itself
					h.run.Set("hibernate", report.Broken, report.Measured, noHibernation, "")
				}
				h.over = true
			}
		case "intro":
			switch msg.String() {
			case "enter", "space":
				h.wifiWas = h.wifi != "" && wifiUp(h.sys, h.wifi)
				h.asked = h.clock()
				h.began(true)
				h.phase = "waiting"
				return tea.Batch(h.asking(), h.poll())
			case "s":
				h.over = true
			}
		case "waiting":
			switch msg.String() {
			case "enter":
				h.stop(report.Broken, report.Owner, "asking for hibernation didn't power the computer off", "")
			case "s":
				h.began(false)
				h.over = true
			}
		case "ask":
			switch h.ask.update(msg) {
			case 0:
				// What was measured counts too, so a "yes" with Wi-Fi still down is only partly working
				state := report.Works
				if len(h.problems) > 0 {
					state = report.Partial
				}
				h.stop(state, report.Owner, h.evidence(), "")
			case 1:
				h.phase = "note"
			}
		case "note":
			if h.note.update(msg) {
				h.stop(report.Partial, report.Owner, h.evidence(), h.note.value())
			}
		}
	}
	return nil
}

func (h *hibernate) view(width, height int) string {
	switch h.phase {
	case "unavailable":
		return wrap("This computer's kernel offers no hibernation, so there is nothing to try here.", width)
	case "setup":
		return wrap("Hibernation can't be tried yet, because "+h.missing+". On Omarchy, set it up in another terminal with:", width) + "\n\n" +
			accent.Render("  omarchy-hibernation-setup") + "\n\n" +
			soft.Render(wrap("It makes a swap file as large as the computer's memory, so the drive needs that much free space, and it asks to restart. After the restart, try it with hw-test hibernate.", width))
	case "intro":
		return wrap("Now hibernate. Save your work in other windows first. When you press Enter, the computer saves what's open to the drive and powers off. Once it's off, power it on and unlock. This screen should be here when you're back, and it then checks that Wi-Fi and sound came back too.", width) + "\n\n" +
			soft.Render(wrap("If the computer starts fresh instead, just run the test again. It knows what happened and keeps your answers.", width))
	case "waiting":
		return accent.Render("● Saving the session. Once the computer is off, power it on and unlock")
	case "settling":
		waiting := "sound"
		if h.wifiWas && !wifiUp(h.sys, h.wifi) {
			waiting = "Wi-Fi"
		}
		return good.Render("✓ It came back from hibernation.") + fmt.Sprintf(" Waiting for %s to come back, %.0f s so far (up to a minute)…", waiting, h.clock().Sub(h.woke).Seconds())
	case "note":
		return h.note.view(width)
	}
	return h.ask.view(width) + "\n\n" + soft.Render(wrap("The test measured that it "+h.evidence()+".", width))
}

// afterHibernating says what the kernel's logs show of a hibernation that ended in a fresh start,
// which is how far the boot that hibernated got, and what this boot found on the drive.
func afterHibernating(sys machine.System) string {
	said := []string{"the computer started fresh instead of coming back from hibernation"}
	klog := func(boot string) string {
		return sys.Run("journalctl", "-k", "-b", boot, "--no-pager", "-q", "-o", "cat")
	}
	before := klog("-1")
	if at := strings.LastIndex(before, "PM: hibernation: hibernation entry"); at >= 0 && strings.Contains(before[at:], "PM: hibernation: hibernation exit") {
		said = append(said, "the saved session came back first and went down after that")
	}
	now := klog("0")
	if !strings.Contains(now, "PM: hibernation: resume from hibernation") {
		said = append(said, "this start found no saved session to restore")
	}
	for _, line := range strings.Split(now, "\n") {
		if strings.Contains(line, "resume failed") || strings.Contains(line, "Failed to load image") || strings.Contains(line, "Image mismatch") || strings.Contains(line, "not generated by this kernel") {
			said = append(said, "this start found the saved session and couldn't restore it ("+strings.TrimSpace(line)+")")
			break
		}
	}
	if len(said) == 1 {
		return said[0]
	}
	return strings.Join(said[:len(said)-1], ", ") + ", and " + said[len(said)-1]
}
