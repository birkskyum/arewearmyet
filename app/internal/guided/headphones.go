package guided

import (
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/audio"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

// headphones plays the tone once wired headphones are plugged in, and asks where it came from.
type headphones struct {
	run   *report.Run
	dir   string
	phase string // intro, playing, ask
	ask   choice
	over  bool
}

func newHeadphones(run *report.Run, dir string) *headphones {
	return &headphones{run: run, dir: dir, phase: "intro", ask: choice{
		question: "Where did the tones come from?",
		options:  []string{"The headphones, left then right", "The headphones, but only one side", "The laptop's speakers, not the headphones", "Nowhere, I heard nothing", "Play them again"},
	}}
}

func (h *headphones) name() string   { return "Headphones" }
func (h *headphones) start() tea.Cmd { return nil }
func (h *headphones) done() bool     { return h.over }

func (h *headphones) keys() string {
	switch h.phase {
	case "intro":
		return "Enter once they're plugged in · s if you have none at hand · a if it has no headphone jack"
	case "playing":
		return "Playing…"
	}
	return choiceKeys
}

// play sounds the tone on the left, then on the right.
func (h *headphones) play() tea.Cmd {
	h.phase = "playing"
	dir := h.dir
	return func() tea.Msg {
		for _, side := range []int{audio.Left, audio.Right} {
			file := filepath.Join(dir, "headphones.wav")
			if err := os.WriteFile(file, audio.Tone(side, 1.5), 0o600); err != nil {
				return playedMsg{err}
			}
			if err := tools.Play(file, 5*time.Second); err != nil {
				return playedMsg{err}
			}
		}
		return playedMsg{}
	}
}

func (h *headphones) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case playedMsg:
		h.phase = "ask"
	case tea.KeyPressMsg:
		switch h.phase {
		case "intro":
			switch msg.String() {
			case "enter", "space":
				return h.play()
			case "a":
				h.run.Set("headphones", report.Absent, report.Owner, "", "")
				h.over = true
			case "s":
				h.over = true
			}
		case "ask":
			set := func(state, note string) {
				h.run.Set("headphones", state, report.Owner, "", note)
				h.over = true
			}
			switch h.ask.update(msg) {
			case 0:
				set(report.Works, "")
			case 1:
				set(report.Partial, "Only one side of the headphones plays.")
			case 2:
				set(report.Broken, "With headphones plugged in, the sound stays on the speakers.")
			case 3:
				set(report.Broken, "With headphones plugged in, there is no sound at all.")
			case 4:
				return h.play()
			}
		}
	}
	return nil
}

func (h *headphones) view(width, height int) string {
	switch h.phase {
	case "intro":
		return wrap("Now the headphone jack. If you have wired headphones at hand, plug them in and press Enter: a short tone plays on the left, then on the right.", width)
	case "playing":
		return accent.Render("▶ Playing a tone on the left, then on the right")
	}
	return h.ask.view(width)
}
