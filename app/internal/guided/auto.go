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

// auto is a test that needs nobody: it runs when its turn comes, shows what it measured and
// goes on by itself. Nothing in it waits for a key.
type auto struct {
	run   *report.Run
	test  deep.Test
	phase string // running, shown, ask
	began time.Time
	took  time.Duration
	// results are what this test measured, and fault its own problem when it had one.
	results []report.Result
	fault   string
	// handsOff is set when nobody may be there to answer: the one question some results lead
	// to is then left out.
	handsOff bool
	ask      choice
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
	case "ask":
		return choiceKeys
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
		return autoMsg{name: name, results: work()}
	}
	return tea.Batch(measure, a.tick())
}

// keep writes a result into the run. A check the quick check already found failing stays
// failing unless this test found it worse: a deeper look that is content doesn't undo it.
func (a *auto) keep(result report.Result) {
	if had, ok := a.run.Get(result.Check); ok && severity[had.State] > severity[result.State] {
		return
	}
	a.run.Set(result.Check, result.State, result.How, result.Evidence, result.Note)
}

// fanQuestion is the one thing the temperature test can't measure: whether a computer that
// shows no fan speed has a fan at all.
func (a *auto) fanQuestion() bool {
	for _, result := range a.results {
		if result.Check == "fans" && result.Note == deep.FanUnread {
			return !a.handsOff
		}
	}
	return false
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
			a.keep(result)
		}
		if a.fanQuestion() {
			a.phase = "ask"
			a.ask = choice{question: "The temperatures can be read, but this computer shows no fan speed. Does it have a fan?",
				options: []string{"Yes, and I heard it speed up just now", "Yes, but I didn't hear it", "No, it has no fan", "I don't know"}}
			return nil
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
		case "ask":
			picked := a.ask.update(msg)
			if picked < 0 {
				return nil
			}
			measured, _ := a.run.Get("fans")
			switch picked {
			case 0:
				a.run.Set("fans", report.Partial, report.Owner, measured.Evidence, "The fan was heard speeding up under load. Its speed can't be read, for want of a sensor.")
			case 1:
				a.run.Set("fans", report.Partial, report.Owner, measured.Evidence, fmt.Sprintf("The temperatures can be read. The fan's speed can't, and the fan wasn't heard speeding up in %.0f s of load.", deep.Load.Seconds()))
			case 2:
				a.run.Set("fans", report.Works, report.Owner, measured.Evidence, "This computer has no fan, by its owner's word. Its temperatures can be read and answer to load.")
			}
			a.over = true
		}
	}
	return nil
}

func (a *auto) view(width, height int) string {
	if a.phase == "ask" {
		return a.measured(width) + "\n\n" + a.ask.view(width)
	}
	if a.phase != "shown" {
		line := fmt.Sprintf("● %s… %.0f s", a.test.Doing, time.Since(a.began).Seconds())
		if a.test.Takes > 0 {
			line += fmt.Sprintf(" of about %.0f", a.test.Takes.Seconds())
		}
		return accent.Render(wrap(line, width)) + "\n\n" + wrap(a.test.About, width)
	}
	return a.measured(width)
}

// measured lists what the test found, each result with what it was read from.
func (a *auto) measured(width int) string {
	if a.fault != "" {
		return close_.Render(wrap("This test ran into a problem of its own ("+a.fault+"). Nothing is reported for it.", width))
	}
	if len(a.results) == 0 {
		return wrap(a.test.Nothing, width)
	}
	var lines []string
	for _, result := range a.results {
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
	return strings.Join(lines, "\n") + "\n\n" + fmt.Sprintf("Measured in %.0f s.", a.took.Seconds())
}

// Unattended runs the automatic tests the words name with nobody at the keyboard, for a
// session without a terminal: an agent's, or one over a pipe. The sound test is left out,
// since it plays tones out loud. It reports only what these tests measured.
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
					say("  it ran into a problem of its own and is passed over: " + fmt.Sprint(fault))
				}
			}()
			for _, result := range test.Run() {
				run.Set(result.Check, result.State, result.How, result.Evidence, result.Note)
			}
		}()
	}
}

// AutomaticWords lists the words that run without a terminal.
func AutomaticWords() []string { return automaticWords }

// Automatic says whether every word names a test that needs nobody and makes no sound.
func Automatic(words []string) bool {
	quiet := map[string]bool{}
	for _, word := range automaticWords {
		quiet[word] = true
	}
	for _, word := range words {
		if !quiet[word] {
			return false
		}
	}
	return len(words) > 0
}
