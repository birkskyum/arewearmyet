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

// part is one sound the test plays while the microphones record: a tone on one speaker, or a
// pitch on both.
type part struct {
	label string
	pitch float64
	// side is audio.Left or audio.Right, or -1 for both speakers.
	side    int
	seconds float64
}

// The test tone on each side shows that speaker works; three pitches on both show what the
// microphones pick up from low to high.
var parts = []part{
	{"the left speaker", audio.Pitch, audio.Left, 1.5},
	{"the right speaker", audio.Pitch, audio.Right, 1.5},
	{"a low tone", 300, -1, 1},
	{"a middle tone", 1000, -1, 1},
	{"a high tone", 3000, -1, 1},
}

// partMsg says a part finished playing, and what the microphones recorded of it.
type partMsg struct {
	at    int
	heard audio.Heard
	// noise is the recording's level with nothing playing, before the sound began.
	noise int
	err   error
}

// playPart plays one part at half volume while the microphones record, and looks for its
// pitch in the recording.
func playPart(dir string, at int) tea.Cmd {
	return func() tea.Msg {
		p := parts[at]
		restore := tools.AtHalfVolume()
		defer restore()
		recording, err := tools.Record(filepath.Join(dir, fmt.Sprintf("heard-%d.wav", at)))
		if err != nil {
			return partMsg{at: at, err: err}
		}
		time.Sleep(500 * time.Millisecond)
		sound := audio.Both(p.pitch, p.seconds)
		if p.side >= 0 {
			sound = audio.Tone(p.side, p.seconds)
		}
		file := filepath.Join(dir, "tone.wav")
		if err = os.WriteFile(file, sound, 0o600); err == nil {
			err = tools.Play(file, 6*time.Second)
		}
		time.Sleep(300 * time.Millisecond)
		samples := audio.Samples(recording.Stop())
		return partMsg{at: at, heard: audio.HearAt(samples, p.pitch), noise: audio.Quietest(samples), err: err}
	}
}

// speakers is the sound test. The speakers play known tones and the microphones record them
// at the same moment: a tone found in the recording shows that the speaker made it and that
// the microphone picked it up, so both are measured with nobody listening. The owner is asked
// only when the recording holds none of the tones, which can be either end's doing.
type speakers struct {
	run   *report.Run
	dir   string
	phase string // intro, playing, result, silent, problem, note
	at    int
	heard []audio.Heard
	// noise is the quietest the microphones were with nothing playing.
	noise int
	err   error
	ask   choice
	// note takes the owner's own words when none of the answers fits.
	note line
	over bool
}

func newSpeakers(run *report.Run, dir string) *speakers {
	return &speakers{run: run, dir: dir, phase: "intro", heard: make([]audio.Heard, len(parts)), note: line{prompt: "What did you hear? (Enter when done)"}}
}

func (s *speakers) name() string   { return "Sound" }
func (s *speakers) start() tea.Cmd { return nil }
func (s *speakers) done() bool     { return s.over }

func (s *speakers) keys() string {
	switch s.phase {
	case "intro":
		return "Enter to start · s to skip"
	case "playing":
		return "Playing and listening…"
	case "result":
		return "Enter to go on · p if something sounded wrong to you · r to play them again"
	case "note":
		return "Type, then Enter"
	}
	return choiceKeys
}

func (s *speakers) left() audio.Heard  { return s.heard[0] }
func (s *speakers) right() audio.Heard { return s.heard[1] }

// heardTone says whether the microphones clearly picked up any of the sounds, which shows
// they work.
func (s *speakers) heardTone() bool {
	for _, heard := range s.heard {
		if heard.Clearly() {
			return true
		}
	}
	return false
}

// balance says how the two sides compared at the microphones, when both were heard. The
// microphones sit closer to one speaker on some machines, so a difference here is something
// to look into, not a verdict.
func (s *speakers) balance() string {
	left, right := s.left(), s.right()
	if !left.Clearly() || !right.Clearly() || left.Level == 0 || right.Level == 0 {
		return ""
	}
	difference := audio.Decibels(left.Level) - audio.Decibels(right.Level)
	louder, quieter := "left", "right"
	if difference < 0 {
		louder, quieter, difference = "right", "left", -difference
	}
	if difference < 3 {
		return "both sides about equally loud at the microphones"
	}
	return fmt.Sprintf("the %s side %.0f dB louder than the %s at the microphones", louder, difference, quieter)
}

// evidence is what the microphones made of the two speakers.
func (s *speakers) evidence() string {
	side := func(h audio.Heard) string {
		if !h.Clearly() {
			return "not heard"
		}
		return fmt.Sprintf("heard at %.0f dB", audio.Decibels(h.Level))
	}
	text := "by the microphones at half volume: left speaker " + side(s.left()) + ", right speaker " + side(s.right())
	if balance := s.balance(); balance != "" {
		text += "; " + balance
	}
	return text
}

// pickup is what the microphones recorded of each pitch, and their noise.
func (s *speakers) pickup() string {
	var tones []string
	for at, p := range parts {
		if p.side >= 0 {
			continue
		}
		name := fmt.Sprintf("%.0f Hz", p.pitch)
		if s.heard[at].Clearly() {
			tones = append(tones, fmt.Sprintf("%s at %.0f dB", name, audio.Decibels(s.heard[at].Level)))
		} else {
			tones = append(tones, name+" not picked up")
		}
	}
	text := "the speakers at half volume recorded as " + strings.Join(tones, ", ") + fmt.Sprintf("; noise with nothing playing %.0f dB", audio.Decibels(s.noise))
	if volume, muted, ok := tools.InputVolume(); ok {
		text += fmt.Sprintf("; input volume %d%%", int(volume*100+0.5))
		if muted {
			text += ", muted"
		}
	}
	return text
}

// measure writes what the recording shows as results: each speaker that was heard works, and
// microphones that heard anything work.
func (s *speakers) measure() {
	switch left, right := s.left().Clearly(), s.right().Clearly(); {
	case left && right:
		s.run.Set("speakers", report.Works, report.Measured, s.evidence(), "")
	case left:
		s.run.Set("speakers", report.Partial, report.Measured, s.evidence(), "Only the left speaker was heard by the microphones.")
	case right:
		s.run.Set("speakers", report.Partial, report.Measured, s.evidence(), "Only the right speaker was heard by the microphones.")
	}
	if s.heardTone() {
		s.run.Set("microphones", report.Works, report.Measured, s.pickup(), "")
	}
}

func (s *speakers) play() tea.Cmd {
	s.phase, s.at, s.noise = "playing", 0, -1
	s.heard = make([]audio.Heard, len(parts))
	return playPart(s.dir, 0)
}

func (s *speakers) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case partMsg:
		if s.phase != "playing" || msg.at != s.at {
			return nil
		}
		s.heard[msg.at], s.err = msg.heard, msg.err
		if s.noise < 0 || msg.noise < s.noise {
			s.noise = msg.noise
		}
		if s.at++; s.at < len(parts) && msg.err == nil {
			return playPart(s.dir, s.at)
		}
		if s.heardTone() {
			s.measure()
			s.phase = "result"
			return nil
		}
		// Nothing of it in the recording: the speakers were silent, or the microphones deaf
		s.phase = "silent"
		s.ask = choice{question: "The microphones recorded none of the tones, so this can't be measured. Did you hear them?",
			options: []string{"Yes, on the left and on the right", "Only on the left", "Only on the right", "No, nothing", "Play them again"}}
	case tea.KeyPressMsg:
		switch s.phase {
		case "intro":
			switch msg.String() {
			case "enter", "space":
				return s.play()
			case "s":
				s.over = true
			}
		case "result":
			switch msg.String() {
			case "enter", "space":
				s.over = true
			case "r":
				return s.play()
			case "p":
				s.phase = "problem"
				s.ask = choice{question: "What sounded wrong?",
					options: []string{"The left is louder than the right", "The right is louder than the left", "Distorted or crackling", "Much too quiet", "Something else", "Nothing after all"}}
			}
		case "silent":
			// The owner heard the speakers, so the silence in the recording is the microphones'
			deaf := func() {
				s.run.Set("microphones", report.Broken, report.Measured, "the speakers played at half volume and the owner heard them, but the recording holds none of it", "")
			}
			switch s.ask.update(msg) {
			case 0:
				s.run.Set("speakers", report.Works, report.Owner, "the microphones recorded nothing, so the speakers couldn't be measured", "")
				deaf()
				s.over = true
			case 1:
				s.run.Set("speakers", report.Partial, report.Owner, "the microphones recorded nothing, so the speakers couldn't be measured", "Only the left speaker plays.")
				deaf()
				s.over = true
			case 2:
				s.run.Set("speakers", report.Partial, report.Owner, "the microphones recorded nothing, so the speakers couldn't be measured", "Only the right speaker plays.")
				deaf()
				s.over = true
			case 3:
				s.run.Set("speakers", report.Broken, report.Owner, "the microphones recorded nothing, so the speakers couldn't be measured", "No sound from the speakers.")
				s.over = true
			case 4:
				return s.play()
			}
		case "problem":
			heard := func(note string) {
				s.run.Set("speakers", report.Partial, report.Owner, s.evidence(), note)
				s.over = true
			}
			switch s.ask.update(msg) {
			case 0:
				heard("Both speakers play, but the left is louder than the right.")
			case 1:
				heard("Both speakers play, but the right is louder than the left.")
			case 2:
				heard("The speakers sound distorted or crackle.")
			case 3:
				heard("The speakers are much too quiet at half volume.")
			case 4:
				s.phase = "note"
			case 5:
				s.phase = "result"
			}
		case "note":
			if s.note.update(msg) {
				s.run.Set("speakers", report.Partial, report.Owner, s.evidence(), s.note.value())
				s.over = true
			}
		}
	}
	return nil
}

func (s *speakers) view(width, height int) string {
	mark := func(heard audio.Heard, label string) string {
		if heard.Clearly() {
			return good.Render("  ✓ ") + fmt.Sprintf("%-18s heard at %.0f dB", label, audio.Decibels(heard.Level))
		}
		return bad.Render("  ✗ ") + fmt.Sprintf("%-18s not heard", label)
	}
	switch s.phase {
	case "intro":
		return wrap("Now the sound. The speakers play a few tones, first on the left, then on the right, then low, middle and high on both, while the microphones record. The test finds the tones in the recording, which checks the speakers and the microphones at once: there is nothing for you to do. The volume is set to half for it and put back after.", width) + "\n\n" + private(width)
	case "playing":
		lines := []string{accent.Render("♪ Playing " + parts[s.at].label), ""}
		for at := 0; at < s.at; at++ {
			lines = append(lines, mark(s.heard[at], parts[at].label))
		}
		return strings.Join(lines, "\n")
	case "silent", "problem":
		return s.ask.view(width)
	case "note":
		return s.note.view(width)
	}
	lines := []string{bold.Render("Speakers")}
	lines = append(lines, mark(s.left(), "Left"), mark(s.right(), "Right"))
	if balance := s.balance(); balance != "" {
		lines = append(lines, "    "+wrap(balance, width-4))
	}
	lines = append(lines, "", bold.Render("Microphone"))
	for at, p := range parts {
		if p.side < 0 {
			lines = append(lines, mark(s.heard[at], fmt.Sprintf("%.0f Hz", p.pitch)))
		}
	}
	lines = append(lines, fmt.Sprintf("    noise with nothing playing: %.0f dB", audio.Decibels(s.noise)), "",
		wrap("Measured from the recording alone. Only these numbers are sent.", width))
	if s.err != nil {
		lines = append(lines, bad.Render(wrap("The player reported: "+s.err.Error(), width)))
	}
	return strings.Join(lines, "\n")
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

func (m *microphones) name() string   { return "Voice" }
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
		text := "Your own voice through the microphone. Press Enter, then say a few words in your normal voice. It records four seconds, measures how loud the recording is, and plays it back, so you can hear how clear it is."
		if m.speakers != nil && m.speakers.phase != "intro" && !m.speakers.heardTone() {
			text = "The microphones recorded none of the tones, so here is another way to test them. Press Enter, then say a few words. The bar moves if the microphone hears you."
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
