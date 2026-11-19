package guided

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/audio"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

// toneMsg says a tone finished playing, and how well the microphones heard it.
type toneMsg struct {
	side  int
	heard audio.Heard
	err   error
}

// playTone plays the test tone on one speaker while the microphones record, and listens for
// it in the recording.
func playTone(dir string, side int) tea.Cmd {
	return func() tea.Msg {
		recording, err := tools.Record(filepath.Join(dir, fmt.Sprintf("heard-%d.wav", side)))
		if err != nil {
			return toneMsg{side: side, err: err}
		}
		time.Sleep(400 * time.Millisecond)
		tone := filepath.Join(dir, "tone.wav")
		if err = os.WriteFile(tone, audio.Tone(side, 2.5), 0o600); err == nil {
			err = tools.Play(tone, 6*time.Second)
		}
		time.Sleep(300 * time.Millisecond)
		return toneMsg{side: side, heard: audio.Hear(audio.Samples(recording.Stop())), err: err}
	}
}

// speakers plays a tone on the left, then on the right, and asks what was heard.
type speakers struct {
	run   *report.Run
	dir   string
	phase string // intro, left, right, ask
	heard [2]audio.Heard
	err   error
	ask   choice
	over  bool
}

func newSpeakers(run *report.Run, dir string) *speakers {
	return &speakers{run: run, dir: dir, phase: "intro", ask: choice{
		question: "What did you hear?",
		options:  []string{"A tone on the left, then on the right, both clean", "Only on the left", "Only on the right", "Both, but distorted or crackling", "Nothing", "Play them again"},
	}}
}

func (s *speakers) name() string   { return "Speakers" }
func (s *speakers) start() tea.Cmd { return nil }
func (s *speakers) done() bool     { return s.over }

func (s *speakers) keys() string {
	switch s.phase {
	case "intro":
		return "Enter to play the tones · s to skip"
	case "ask":
		return choiceKeys
	}
	return "Listening…"
}

// evidence is what the microphones made of the two tones.
func (s *speakers) evidence() string {
	part := func(h audio.Heard) string {
		if h.Windows == 0 {
			return "not at all"
		}
		return fmt.Sprintf("for %.1f s (level %d)", float64(h.Windows)/4, h.Level)
	}
	return "the microphones heard the left tone " + part(s.heard[audio.Left]) + ", the right " + part(s.heard[audio.Right])
}

func (s *speakers) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case toneMsg:
		s.heard[msg.side], s.err = msg.heard, msg.err
		if msg.side == audio.Left && msg.err == nil {
			s.phase = "right"
			return playTone(s.dir, audio.Right)
		}
		s.phase = "ask"
	case tea.KeyPressMsg:
		switch s.phase {
		case "intro":
			switch msg.String() {
			case "enter", "space":
				s.phase = "left"
				return playTone(s.dir, audio.Left)
			case "s":
				s.over = true
			}
		case "ask":
			set := func(state, note string) {
				s.run.Set("speakers", state, report.Owner, s.evidence(), note)
				s.over = true
			}
			switch s.ask.update(msg) {
			case 0:
				set(report.Works, "")
			case 1:
				set(report.Partial, "Only the left speaker plays.")
			case 2:
				set(report.Partial, "Only the right speaker plays.")
			case 3:
				set(report.Partial, "Both speakers play, but distorted or crackling.")
			case 4:
				set(report.Broken, "No sound from the speakers.")
			case 5:
				s.phase = "left"
				return playTone(s.dir, audio.Left)
			}
		}
	}
	return nil
}

func (s *speakers) view(width, height int) string {
	switch s.phase {
	case "intro":
		text := "Two tones play for a couple of seconds each, first on the left speaker, then on the right, while the microphones listen for them. Only how loud the tone was is kept."
		if volume, muted, ok := tools.Volume(); ok && (muted || volume < 0.25) {
			text += "\n\n" + close_.Render(fmt.Sprintf("The sound is %s. Turn it up to about half first, or you won't hear them.", map[bool]string{true: "muted", false: fmt.Sprintf("at %d%%", int(volume*100+0.5))}[muted]))
		}
		return wrap(text, width) + "\n\n" + private(width)
	case "left":
		return accent.Render("◀ ◀ ◀  Playing on the LEFT speaker")
	case "right":
		return accent.Render("Playing on the RIGHT speaker  ▶ ▶ ▶")
	}
	text := s.ask.view(width) + "\n\n" + dim.Render(wrap("Measured: "+s.evidence()+".", width))
	if s.err != nil {
		text += "\n" + bad.Render(wrap("The player reported: "+s.err.Error(), width))
	}
	return text
}

// meterMsg is how loud the microphones are right now, during a recording.
type meterMsg struct{ level int }

// recordedMsg says the recording is over.
type recordedMsg struct{ samples []int16 }

type playedMsg struct{ err error }

// microphones records the owner's voice and plays it back, so they hear for themselves
// whether the microphones work.
type microphones struct {
	run       *report.Run
	dir       string
	speakers  *speakers
	phase     string // intro, recording, playing, ask
	recording *tools.Recording
	began     time.Time
	level     int
	voice     int
	peak      int
	ask       choice
	over      bool
}

const recordFor = 4 * time.Second

func newMicrophones(run *report.Run, dir string) *microphones {
	return &microphones{run: run, dir: dir, phase: "intro", ask: choice{
		question: "Did you hear your own voice?",
		options:  []string{"Yes, clearly", "Yes, but quiet or muffled", "No, only silence or noise", "Record again"},
	}}
}

func (m *microphones) name() string   { return "Microphones" }
func (m *microphones) start() tea.Cmd { return nil }
func (m *microphones) done() bool     { return m.over }

func (m *microphones) keys() string {
	switch m.phase {
	case "intro":
		return "Enter to start recording · s to skip"
	case "recording":
		return "Recording…"
	case "playing":
		return "Playing it back…"
	}
	return choiceKeys
}

func (m *microphones) record() tea.Cmd {
	recording, err := tools.Record(filepath.Join(m.dir, "voice.wav"))
	if err != nil {
		m.run.Set("microphones", report.Broken, report.Measured, "recording didn't start: "+err.Error(), "")
		m.over = true
		return nil
	}
	m.recording, m.began, m.phase, m.level = recording, time.Now(), "recording", 0
	return m.tick()
}

func (m *microphones) tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		if time.Since(m.began) >= recordFor {
			return recordedMsg{samples: audio.Samples(m.recording.Stop())}
		}
		// The loudness of the last fifth of a second written so far
		samples := audio.Samples(m.recording.SoFar())
		if len(samples) > audio.Rate/5 {
			samples = samples[len(samples)-audio.Rate/5:]
		}
		return meterMsg{level: audio.Level(samples)}
	})
}

func (m *microphones) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case meterMsg:
		m.level = msg.level
		return m.tick()
	case recordedMsg:
		m.voice = audio.Level(msg.samples)
		m.peak = 0
		for at := 0; at+audio.Rate/5 <= len(msg.samples); at += audio.Rate / 5 {
			m.peak = max(m.peak, audio.Level(msg.samples[at:at+audio.Rate/5]))
		}
		m.phase = "playing"
		file := m.recording.File
		return func() tea.Msg { return playedMsg{err: tools.Play(file, recordFor+3*time.Second)} }
	case playedMsg:
		m.phase = "ask"
	case tea.KeyPressMsg:
		switch m.phase {
		case "intro":
			switch msg.String() {
			case "enter", "space":
				return m.record()
			case "s":
				// Skipped, but the tones the microphones picked up still say they work
				if m.speakers != nil && (m.speakers.heard[0].Clearly() || m.speakers.heard[1].Clearly()) {
					m.run.Set("microphones", report.Works, report.Measured, m.speakers.evidence(), "")
				}
				m.over = true
			}
		case "ask":
			evidence := fmt.Sprintf("voice level %d, loudest %d", m.voice, m.peak)
			if m.speakers != nil {
				evidence += "; " + m.speakers.evidence()
			}
			set := func(state, note string) {
				m.run.Set("microphones", state, report.Owner, evidence, note)
				m.over = true
			}
			switch m.ask.update(msg) {
			case 0:
				set(report.Works, "")
			case 1:
				set(report.Partial, "The recording plays back quiet or muffled.")
			case 2:
				set(report.Broken, "The recording is silence or noise.")
			case 3:
				return m.record()
			}
		}
	}
	return nil
}

// meter draws a loudness bar. Loudness is shown on a log scale, as ears hear it.
func meter(level, width int) string {
	filled := 0
	if level > 1 {
		filled = int(math.Log10(float64(level)) / math.Log10(20000) * float64(width))
	}
	filled = max(0, min(width, filled))
	return good.Render(strings.Repeat("█", filled)) + dim.Render(strings.Repeat("░", width-filled))
}

func (m *microphones) view(width, height int) string {
	switch m.phase {
	case "intro":
		return wrap("Now your voice. Press Enter, then say a few words in a normal voice. It records four seconds and plays them back to you, so you can hear whether the microphones work.", width) + "\n\n" + private(width)
	case "recording":
		left := recordFor - time.Since(m.began)
		return accent.Render(fmt.Sprintf("● Recording, speak now  (%.0f s left)", math.Max(0, left.Seconds()))) + "\n\n" + meter(m.level, min(width, 60)) + "\n\n" + dim.Render("Only on this computer. Not sent.")
	case "playing":
		return accent.Render("▶ Playing your recording back")
	}
	return m.ask.view(width) + "\n\n" + dim.Render(fmt.Sprintf("Measured: voice level %d, loudest %d. Only these two numbers are sent, not the recording.", m.voice, m.peak))
}
