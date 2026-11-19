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
	phase string // intro, left, right, ask, note
	heard [2]audio.Heard
	err   error
	ask   choice
	// note takes the owner's own words when none of the answers fits.
	note line
	over bool
}

func newSpeakers(run *report.Run, dir string) *speakers {
	return &speakers{run: run, dir: dir, phase: "intro", ask: choice{
		question: "What did you hear?",
		options: []string{"A tone on the left, then on the right, both clean and equally loud", "Both, but the left is louder than the right", "Both, but the right is louder than the left",
			"Only on the left", "Only on the right", "Both, but distorted or crackling", "Nothing", "Something else", "Play them again"},
	}, note: line{prompt: "What did you hear? (Enter when done)"}}
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
	case "note":
		return "Type, then Enter"
	}
	return "Listening…"
}

// balance says how the two tones compared at the microphones, when both were heard: "the
// left tone was 2.3 times as loud as the right". The microphones sit closer to one speaker on
// some machines, so this backs up what the owner hears and doesn't replace it.
func (s *speakers) balance() string {
	left, right := s.heard[audio.Left], s.heard[audio.Right]
	if !left.Clearly() || !right.Clearly() || left.Level == 0 || right.Level == 0 {
		return ""
	}
	louder, quieter, ratio := "left", "right", float64(left.Level)/float64(right.Level)
	if ratio < 1 {
		louder, quieter, ratio = "right", "left", 1/ratio
	}
	if ratio < 1.5 {
		return "at the microphones the two tones were about equally loud"
	}
	return fmt.Sprintf("at the microphones the %s tone was %.1f times as loud as the %s", louder, ratio, quieter)
}

// heardTone says whether the microphones clearly picked up a tone, which shows they work.
func (s *speakers) heardTone() bool {
	return s.heard[audio.Left].Clearly() || s.heard[audio.Right].Clearly()
}

// microphonesHeard records the microphones as working when they heard the tone: that is a
// measurement, and leaves nothing to ask the owner about them.
func (s *speakers) microphonesHeard() {
	if s.heardTone() {
		s.run.Set("microphones", report.Works, report.Measured, s.evidence(), "")
	}
}

// evidence is what the microphones made of the two tones.
func (s *speakers) evidence() string {
	part := func(h audio.Heard) string {
		if h.Windows == 0 {
			return "not at all"
		}
		return fmt.Sprintf("for %.1f s (level %d)", float64(h.Windows)/4, h.Level)
	}
	text := "the microphones heard the left tone " + part(s.heard[audio.Left]) + ", the right " + part(s.heard[audio.Right])
	if balance := s.balance(); balance != "" {
		text += "; " + balance
	}
	return text
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
				s.microphonesHeard()
				s.over = true
			}
			switch s.ask.update(msg) {
			case 0:
				set(report.Works, "")
			case 1:
				set(report.Partial, "Both speakers play, but the left is louder than the right.")
			case 2:
				set(report.Partial, "Both speakers play, but the right is louder than the left.")
			case 3:
				set(report.Partial, "Only the left speaker plays.")
			case 4:
				set(report.Partial, "Only the right speaker plays.")
			case 5:
				set(report.Partial, "Both speakers play, but distorted or crackling.")
			case 6:
				set(report.Broken, "No sound from the speakers.")
			case 7:
				s.phase = "note"
			case 8:
				s.phase = "left"
				return playTone(s.dir, audio.Left)
			}
		case "note":
			if s.note.update(msg) {
				s.run.Set("speakers", report.Partial, report.Owner, s.evidence(), s.note.value())
				s.microphonesHeard()
				s.over = true
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
	if s.phase == "note" {
		return s.note.view(width)
	}
	text := s.ask.view(width) + "\n\n" + soft.Render(wrap("Measured: "+s.evidence()+".", width))
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

// microphones records a few seconds, measures how loud the recording is, and plays it back
// made as loud as the test tone. A recording is numbers, so its level is the microphone's
// doing alone: a quiet one is the microphone recording quietly, whatever the speakers are
// set to. Played back at the tone's loudness, the owner judges only whether it is clear.
type microphones struct {
	run       *report.Run
	dir       string
	speakers  *speakers
	phase     string // intro, recording, playing, ask, note
	recording *tools.Recording
	began     time.Time
	level     int
	// loudest is the level of the recording's loudest fifth of a second, the voice itself.
	loudest int
	// gain is how many times louder it was played back than recorded.
	gain float64
	// input is the microphone's input volume setting, as a note for the result, or "".
	input string
	ask   choice
	// note takes the owner's own words when none of the answers fits.
	note line
	over bool
}

const (
	recordFor = 4 * time.Second
	// Speech this far below full scale is too quiet to use: the other side of a call would
	// barely hear it. The test tone plays at about -12.
	quietBelow = -40.0
	// Below this the recording holds nothing but the microphone's own noise.
	silentBelow = -60.0
)

func newMicrophones(run *report.Run, dir string) *microphones {
	return &microphones{run: run, dir: dir, phase: "intro", note: line{prompt: "What was it like? (Enter when done)"}}
}

func (m *microphones) name() string   { return "Microphone" }
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
	case "note":
		return "Type, then Enter"
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

// silent and quiet say what the recording's level means.
func (m *microphones) silent() bool { return audio.Decibels(m.loudest) < silentBelow }
func (m *microphones) quiet() bool  { return !m.silent() && audio.Decibels(m.loudest) < quietBelow }

// measured is the recording's level in words, for the screen and the result.
func (m *microphones) measured() string {
	text := fmt.Sprintf("speech recorded at %.0f dB, where the test tone plays at %.0f dB", audio.Decibels(m.loudest), audio.Decibels(audio.ToneLevel))
	if m.input != "" {
		text += "; " + m.input
	}
	return text
}

// verdict explains the level: where a quiet recording's quietness comes from.
func (m *microphones) verdict() string {
	switch {
	case m.silent():
		return "The recording holds almost nothing: the microphone picked up no voice."
	case m.quiet():
		return fmt.Sprintf("The microphone records quietly: your voice is %.0f dB below the test tone you heard. That quietness is in the recording itself, so it is the microphone and not the speakers. It was played back %.0f times louder so you can judge how clear it is.",
			audio.Decibels(audio.ToneLevel)-audio.Decibels(m.loudest), m.gain)
	}
	return "The microphone records at a healthy level. If it sounds quiet anywhere, that is the playback volume, not the microphone."
}

func (m *microphones) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case meterMsg:
		m.level = msg.level
		return m.tick()
	case recordedMsg:
		m.loudest = audio.Loudest(msg.samples)
		boosted, gain := audio.Boost(msg.samples)
		m.gain = gain
		m.input = ""
		if volume, muted, ok := tools.InputVolume(); ok {
			m.input = fmt.Sprintf("input volume %d%%", int(volume*100+0.5))
			if muted {
				m.input += ", muted"
			}
		}
		question := "That was your recording. Is your voice clear?"
		if gain >= 1.5 {
			question = fmt.Sprintf("That was your recording, played %.0f times louder than it was recorded. Is your voice clear?", gain)
		}
		m.ask = choice{question: question, options: []string{"Yes, clear", "Muffled, distorted or noisy", "I can't make out my voice", "Something else", "Record again"}}
		m.phase = "playing"
		file := filepath.Join(m.dir, "voice-played.wav")
		if err := os.WriteFile(file, audio.WAV(boosted, 1), 0o600); err != nil {
			return func() tea.Msg { return playedMsg{err} }
		}
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
				m.over = true
			}
		case "ask":
			set := func(state, note string) {
				m.run.Set("microphones", state, report.Owner, m.measured(), note)
				m.over = true
			}
			switch m.ask.update(msg) {
			case 0:
				// Clear, but a quiet recording is still a microphone that needs its level raised
				if m.quiet() {
					set(report.Partial, fmt.Sprintf("The microphone records quietly, %.0f dB below the test tone. Made louder, the voice is clear.", audio.Decibels(audio.ToneLevel)-audio.Decibels(m.loudest)))
				} else {
					set(report.Works, "")
				}
			case 1:
				set(report.Partial, "The recording is muffled, distorted or noisy.")
			case 2:
				set(report.Broken, "No voice can be made out in the recording.")
			case 3:
				m.phase = "note"
			case 4:
				return m.record()
			}
		case "note":
			if m.note.update(msg) {
				m.run.Set("microphones", report.Partial, report.Owner, m.measured(), m.note.value())
				m.over = true
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
	return good.Render(strings.Repeat("█", filled)) + soft.Render(strings.Repeat("░", width-filled))
}

func (m *microphones) view(width, height int) string {
	switch m.phase {
	case "intro":
		text := "Now the microphone. Press Enter, then say a few words in your normal voice. It records four seconds, measures how loud the recording is, and plays it back."
		if m.speakers != nil && m.speakers.heardTone() {
			text = "The microphones heard the tones, so they work. This checks how well. Press Enter, then say a few words in your normal voice. It records four seconds, measures how loud the recording is, and plays it back."
		}
		return wrap(text, width) + "\n\n" + private(width)
	case "recording":
		left := recordFor - time.Since(m.began)
		return accent.Render(fmt.Sprintf("● Recording, speak now  (%.0f s left)", math.Max(0, left.Seconds()))) + "\n\n" + meter(m.level, min(width, 60)) + "\n\n" + soft.Render("Only on this computer. Not sent.")
	case "playing":
		return accent.Render("▶ Playing your recording back")
	case "note":
		return m.note.view(width)
	}
	return wrap(m.verdict(), width) + "\n\n" + m.ask.view(width) + "\n\n" + soft.Render(wrap("Measured: "+m.measured()+". Only these numbers are sent, not the recording.", width))
}
