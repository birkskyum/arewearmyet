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
// Where the sound server offers no headphone output, it asks first whether the computer has a
// headphone socket at all. Many laptops have only USB-C ports, and an owner who plugs USB-C
// headphones in would otherwise report a jack that isn't there as broken.
type headphones struct {
	run   *report.Run
	dir   string
	phase string // socket, intro, playing, ask
	ask   choice
	// socket asks whether there is one, where the sound server offers no headphone output
	socket choice
	// none is set when the sound server offers no headphone output, and goes with the result
	none bool
	over bool
}

// headphoneOutput is how the step learns whether a headphone output is offered.
var headphoneOutput = tools.HeadphoneOutput

// noOutput is the evidence that goes with a result where no headphone output is offered.
const noOutput = "the sound server offers no headphone output on the built-in sound card"

func newHeadphones(run *report.Run, dir string) *headphones {
	h := &headphones{run: run, dir: dir, phase: "intro", ask: choice{
		question: "Where did the tones come from?",
		options:  []string{"The headphones, left then right", "The headphones, but only one side", "The laptop's speakers, not the headphones", "Nowhere, I heard nothing", "Play them again"},
	}}
	if offered, known := headphoneOutput(); known && !offered {
		h.none = true
		h.phase = "socket"
		h.socket = choice{
			question: "Does this computer have a headphone socket, the small round one for a 3.5 mm plug?",
			options:  []string{"No, it has none", "Yes, it has one"},
		}
	}
	return h
}

func (h *headphones) name() string   { return "Headphones" }
func (h *headphones) start() tea.Cmd { return nil }
func (h *headphones) done() bool     { return h.over }

func (h *headphones) keys() string {
	switch h.phase {
	case "socket":
		return choiceKeys
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
		case "socket":
			switch h.socket.update(msg) {
			case 0:
				h.run.Set("headphones", report.Absent, report.Owner, noOutput, "")
				h.over = true
			case 1:
				h.phase = "intro"
			}
		case "intro":
			switch msg.String() {
			case "enter", "space":
				return h.play()
			case "a":
				h.run.Set("headphones", report.Absent, report.Owner, h.evidence(), "")
				h.over = true
			case "s":
				h.over = true
			}
		case "ask":
			set := func(state, note string) {
				h.run.Set("headphones", state, report.Owner, h.evidence(), note)
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

// evidence says that no headphone output is offered, where that is so. A socket that is there with
// no output behind it is a fault of its own, which an agent can start from.
func (h *headphones) evidence() string {
	if h.none {
		return noOutput
	}
	return ""
}

func (h *headphones) view(width, height int) string {
	switch h.phase {
	case "socket":
		return wrap("Now the headphone jack. Linux offers no headphone output on this computer, as on laptops that have only USB-C ports. USB-C headphones and adapters are sound cards of their own and are not this test.", width) + "\n\n" + h.socket.view(width)
	case "intro":
		return wrap("Now the headphone jack, the small round socket for a 3.5 mm plug. If you have wired headphones at hand, plug them in and press Enter. A short tone plays on the left, then on the right. USB-C headphones and adapters are sound cards of their own and are not this test.", width)
	case "playing":
		return accent.Render("▶ Playing a tone on the left, then on the right")
	}
	return h.ask.view(width)
}
