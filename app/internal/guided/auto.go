package guided

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/deep"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// autoMsg carries what an automatic test measured.
type autoMsg struct {
	name    string
	results []report.Result
	// fault is set when the test hit a problem of its own.
	fault string
}

// autoTick redraws the seconds a running test has taken.
type autoTick struct{ name string }

// goOnMsg ends a step that goes on by itself once its results have been up for a moment.
type goOnMsg struct{ name string }

// unattended is implemented by the steps that can run with nobody at the keyboard, and are
// told so when the round is the automatic tests alone.
type unattended interface{ alone(bool) }

// linger is how long an automatic step's results stay up before the test goes on.
const linger = 3 * time.Second

// auto is a test that needs nobody. It runs when its turn comes, shows what it measured and goes on
// by itself. Nothing in it waits for a key.
type auto struct {
	run   *report.Run
	test  deep.Test
	phase string // running, shown
	began time.Time
	took  time.Duration
	// results are what this test measured, and fault its own problem when it had one.
	results  []report.Result
	fault    string
	handsOff bool
	linger   time.Duration
	over     bool
}

func newAuto(run *report.Run, test deep.Test) *auto {
	return &auto{run: run, test: test, linger: linger}
}

func (a *auto) name() string        { return a.test.Name }
func (a *auto) done() bool          { return a.over }
func (a *auto) alone(handsOff bool) { a.handsOff = handsOff }

func (a *auto) keys() string {
	switch a.phase {
	case "shown":
		return "Goes on by itself in a moment · Enter to go on now"
	}
	return "Measuring, nothing for you to do · s to skip"
}

func (a *auto) tick() tea.Cmd {
	name := a.test.Name
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return autoTick{name} })
}

func (a *auto) start() tea.Cmd {
	a.phase, a.began, a.results, a.fault = "running", time.Now(), nil, ""
	name, work := a.test.Name, a.test.Run
	measure := func() (msg tea.Msg) {
		// A test that falls over reports nothing, and the next one still runs
		defer func() {
			if fault := recover(); fault != nil {
				msg = autoMsg{name: name, fault: fmt.Sprint(fault)}
			}
		}()
		return autoMsg{name: name, results: work(func(string) {})}
	}
	return tea.Batch(measure, a.tick())
}

// keep writes a result into the run. A check the quick check already found failing stays failing
// unless this test found it worse, since a deeper look that is content doesn't undo it.
func keep(run *report.Run, result report.Result) {
	if had, ok := run.Get(result.Check); ok && severity[had.State] > severity[result.State] {
		return
	}
	run.Set(result.Check, result.State, result.How, result.Evidence, result.Note)
}

func (a *auto) goOn() tea.Cmd {
	name := a.test.Name
	return tea.Tick(a.linger, func(time.Time) tea.Msg { return goOnMsg{name} })
}

func (a *auto) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case autoTick:
		if msg.name == a.test.Name && a.phase == "running" {
			return a.tick()
		}
	case autoMsg:
		if msg.name != a.test.Name || a.phase != "running" {
			return nil
		}
		a.results, a.fault, a.took = msg.results, msg.fault, time.Since(a.began)
		for _, result := range a.results {
			keep(a.run, result)
		}
		a.phase = "shown"
		return a.goOn()
	case goOnMsg:
		if msg.name == a.test.Name && a.phase == "shown" {
			a.over = true
		}
	case tea.KeyPressMsg:
		switch a.phase {
		case "running":
			if msg.String() == "s" {
				a.over = true
			}
		case "shown":
			if key := msg.String(); key == "enter" || key == "space" {
				a.over = true
			}
		}
	}
	return nil
}

func (a *auto) view(width, height int) string {
	if a.phase != "shown" {
		line := fmt.Sprintf("● %s… %.0f s", a.test.Doing, time.Since(a.began).Seconds())
		if a.test.Takes > 0 {
			line += fmt.Sprintf(" of about %.0f", a.test.Takes.Seconds())
		}
		return accent.Render(wrap(line, width)) + "\n\n" + wrap(a.test.About, width)
	}
	if a.fault != "" {
		return close_.Render(wrap("This test ran into a problem of its own ("+a.fault+"). Nothing is reported for it.", width))
	}
	if len(a.results) == 0 {
		return wrap(a.test.Nothing, width)
	}
	return showResults(a.results, width) + "\n\n" + fmt.Sprintf("Measured in %.0f s.", a.took.Seconds())
}

// showResults lists what a test found, each result with what it was read from.
func showResults(results []report.Result, width int) string {
	var lines []string
	for _, result := range results {
		mark := map[string]string{report.Works: "✓ ", report.Partial: "◐ ", report.Broken: "✗ "}[result.State]
		lines = append(lines, stateStyle(result.State).Render(mark+first(checkTitle[result.Check], result.Check)+": "+stateLabel[result.State]))
		for _, detail := range []string{result.Evidence, result.Note} {
			if detail != "" {
				for _, line := range strings.Split(wrap(detail, width-2), "\n") {
					lines = append(lines, "  "+line)
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

// Unattended runs the automatic tests the words name with nobody at the keyboard, for a session
// without a terminal, like an agent's or one over a pipe. The sound test is left out, since it
// plays tones out loud. It reports only what these tests measured.
func Unattended(run *report.Run, tests []deep.Test, words []string, say func(string)) {
	wanted := map[string]bool{}
	for _, word := range words {
		for _, entry := range stepWords {
			if entry.word == word {
				for _, name := range entry.steps {
					wanted[name] = true
				}
			}
		}
	}
	run.Results = nil
	for _, test := range tests {
		if !wanted[test.Name] {
			continue
		}
		say(test.Doing + "…")
		func() {
			defer func() {
				if fault := recover(); fault != nil {
					say("  it ran into a problem of its own and is passed over (" + fmt.Sprint(fault) + ")")
				}
			}()
			for _, result := range test.Run(say) {
				run.Set(result.Check, result.State, result.How, result.Evidence, result.Note)
			}
		}()
	}
}

// AutomaticWords lists the words that run without a terminal.
func AutomaticWords() []string { return automaticWords }

// soundWords run the sound test, which plays tones and listens to them.
var soundWords = map[string]bool{"speakers": true, "microphone": true}

// MaySound says whether the sound test may run with nobody at a terminal. That is when the owner
// handed the speakers or the microphones to an agent, which is their yes to its tones.
func MaySound(now time.Time) bool {
	return report.Handed("speakers", now) || report.Handed("microphones", now)
}

// WantsSound says whether the words ask for the sound test.
func WantsSound(words []string) bool {
	for _, word := range words {
		if soundWords[word] || word == "auto" {
			return true
		}
	}
	return false
}

// Heard runs the sound test with nobody at a terminal. It plays the tones, listens to them and
// writes what the microphones made of them into the run.
func Heard(run *report.Run, dir string, say func(string)) {
	say("Playing test tones at half volume and listening to them…")
	s := newSpeakers(run, dir)
	s.handsOff = true
	for cmd := s.start(); cmd != nil && s.phase == "playing"; {
		cmd = s.update(cmd())
	}
	switch {
	case s.err != nil:
		say("  the sound test couldn't run (" + s.err.Error() + ")")
	case s.phase == "silent":
		say("  the microphones recorded none of the tones, so the speakers can't be measured without the owner listening")
	}
}

// Automatic says whether every word names a test that needs nobody and makes no sound, or the sound
// test where the owner has agreed to it, by handing the speakers over or as the agent says they did
// with --play.
func Automatic(words []string, play bool) bool {
	quiet := map[string]bool{}
	for _, word := range automaticWords {
		quiet[word] = true
	}
	if play || MaySound(time.Now()) {
		for word := range soundWords {
			quiet[word] = true
		}
	}
	for _, word := range words {
		if !quiet[word] {
			return false
		}
	}
	return len(words) > 0
}
