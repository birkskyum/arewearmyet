// Package guided is the test done with the owner: keyboard, sound, camera, sleep and a few
// questions, in the terminal. Each part is a step; a step measures what it can and asks only
// what takes eyes or ears.
package guided

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

// Options are the guided test's settings.
type Options struct {
	API  string
	Send bool
}

// Interactive says whether someone is at a terminal to answer.
func Interactive() bool {
	in, err := os.Stdin.Stat()
	out, err2 := os.Stdout.Stat()
	return err == nil && err2 == nil && in.Mode()&os.ModeCharDevice != 0 && out.Mode()&os.ModeCharDevice != 0
}

// step is one part of the test. It writes its results into the run it was made with.
type step interface {
	// name is the step's short name for the progress line.
	name() string
	// start runs when the step comes up.
	start() tea.Cmd
	// update handles a key press or one of the step's own messages.
	update(msg tea.Msg) tea.Cmd
	// view draws the step in the given width and height.
	view(width, height int) string
	// keys is the hint line under the step.
	keys() string
	// done says whether the step is over.
	done() bool
}

// allKeys is implemented by a step that needs every key press, modifiers included, as the
// keyboard step does.
type allKeys interface{ wantsAllKeys() bool }

type model struct {
	run     *report.Run
	steps   []step
	at      int
	width   int
	height  int
	aborted bool
	summary *summary
}

// Run shows the guided test and sends its results.
func Run(run *report.Run, facts probe.Facts, options Options) error {
	dir, err := os.MkdirTemp("", "awy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	run.Mode = "guided"
	last := &summary{run: run, options: options}
	m := &model{run: run, summary: last, width: 100, height: 30}
	m.steps = append(m.steps, &welcome{run: run})
	if facts.Keyboard != "" {
		m.steps = append(m.steps, newKeyboard(run), newEffects(run, facts))
	}
	if facts.SoundCard != "" && tools.Have("pw-play", "pw-record") {
		speakers, microphones := newSpeakers(run, dir), newMicrophones(run, dir)
		microphones.speakers = speakers
		m.steps = append(m.steps, speakers, microphones)
	}
	if facts.Camera != "" && tools.Have("cam") {
		m.steps = append(m.steps, newCamera(run, dir))
	}
	if facts.Laptop && facts.Lid != "" {
		m.steps = append(m.steps, newSleep(run, facts))
	}
	m.steps = append(m.steps, newQuestions(run, facts), last)

	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}
	if final.(*model).aborted {
		fmt.Println("Stopped. Nothing was sent.")
		return nil
	}
	fmt.Println(last.farewell())
	return nil
}

func (m *model) Init() tea.Cmd { return m.steps[0].start() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.aborted = true
			return m, tea.Quit
		}
	}
	current := m.steps[m.at]
	cmd := current.update(msg)
	if !current.done() {
		return m, cmd
	}
	// On to the next step, past any that has nothing to do on this machine
	cmds := []tea.Cmd{cmd}
	for m.steps[m.at].done() {
		if m.at == len(m.steps)-1 {
			return m, tea.Quit
		}
		m.at++
		cmds = append(cmds, m.steps[m.at].start())
	}
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	width := min(m.width, 110)
	current := m.steps[m.at]

	// Where the test is: every step's name, the current one marked
	var progress []string
	for i, s := range m.steps {
		switch {
		case i < m.at:
			progress = append(progress, good.Render("✓ ")+dim.Render(s.name()))
		case i == m.at:
			progress = append(progress, accent.Render("● "+s.name()))
		default:
			progress = append(progress, dim.Render("○ "+s.name()))
		}
	}
	machine := strings.TrimSpace(m.run.Hardware["maker"] + " " + first(m.run.Hardware["family"], m.run.Hardware["product"]))
	header := title.Render("Are we ARM yet?") + dim.Render("  "+machine)
	body := current.view(width-4, m.height-8)
	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		wrapProgress(progress, width-4),
		"",
		body,
		"",
		dim.Render(current.keys()+" · Ctrl+C to stop"),
	)
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	view.AltScreen = true
	if step, ok := current.(allKeys); ok && step.wantsAllKeys() {
		// Every key as its own event, with the key it is on a US keyboard, so the picture
		// of the keyboard lights up by position whatever the layout
		view.KeyboardEnhancements = tea.KeyboardEnhancements{ReportAlternateKeys: true, ReportAllKeysAsEscapeCodes: true, ReportAssociatedText: true}
	}
	return view
}

// wrapProgress lays the step names out on as many lines as the width needs.
func wrapProgress(items []string, width int) string {
	var lines []string
	line := ""
	for _, item := range items {
		if line != "" && lipgloss.Width(line)+3+lipgloss.Width(item) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += "   "
		}
		line += item
	}
	return strings.Join(append(lines, line), "\n")
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// welcome shows what the quick check measured and what the guided test will do.
type welcome struct {
	run     *report.Run
	started bool
}

func (w *welcome) name() string   { return "Start" }
func (w *welcome) start() tea.Cmd { return nil }
func (w *welcome) done() bool     { return w.started }
func (w *welcome) keys() string   { return "Enter to start" }

func (w *welcome) update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok && (key.String() == "enter" || key.String() == "space") {
		w.started = true
	}
	return nil
}

func (w *welcome) view(width, height int) string {
	intro := "This checks what works on this computer and puts it on its page on arewearmyet.com, so the next owner knows and the people fixing things see what's left. " +
		"It takes about five minutes. It has already measured what it can, and now tests the keyboard, speakers, microphones, camera and sleep with you."
	measured := resultsTable(w.run.Results, width)
	return wrap(intro, width) + "\n\n" + bold.Render("Measured just now") + "\n" + measured + "\n\n" +
		dim.Render(wrap("Nothing personal is sent: no serial numbers, addresses, names or files. Recordings and camera frames stay on this computer and are deleted at the end.", width))
}
