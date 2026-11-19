// Package guided is the test done with the owner: keyboard, sound, camera, sleep and a few
// questions, in the terminal. Each part is a step; a step measures what it can and asks only
// what takes eyes or ears.
//
// It runs on machines whose hardware support is unfinished, so nothing a step meets may end
// the test: a step that fails is passed over, the answers are saved from step to step, and a
// computer that goes down in the middle of a step finds its test again on the next run.
package guided

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

// Options are the guided test's settings.
type Options struct {
	API  string
	Send bool
	// Only names the steps to run, as given on the command line. Empty runs them all.
	Only []string
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

// pointer is implemented by a step that needs the pointer's movement, clicks and scrolling.
type pointer interface{ wantsMouse() bool }

// The words a step can be asked for by on the command line, and the steps each one runs.
var stepWords = []struct {
	word  string
	steps []string
}{
	{"keyboard", []string{"Keyboard", "Volume and brightness keys"}},
	{"keys", []string{"Volume and brightness keys"}},
	{"touchpad", []string{"Touchpad"}},
	{"touchscreen", []string{"Touchscreen"}},
	{"sound", []string{"Speakers", "Microphone", "Headphones"}},
	{"speakers", []string{"Speakers"}},
	{"microphone", []string{"Microphone"}},
	{"headphones", []string{"Headphones"}},
	{"camera", []string{"Camera"}},
	{"usb", []string{"USB ports"}},
	{"screen", []string{"External screen"}},
	{"sleep", []string{"Sleep"}},
	{"questions", []string{"Questions"}},
}

// The check a crash during a step is recorded for.
var stepCheck = map[string]string{
	"Keyboard": "keyboard", "Volume and brightness keys": "keyboard", "Touchpad": "touchpad", "Touchscreen": "touchscreen",
	"Speakers": "speakers", "Microphone": "microphones", "Headphones": "headphones", "Camera": "camera",
	"USB ports": "usb", "External screen": "external-display", "Sleep": "suspend",
}

// StepWords lists the words a single step can be asked for by.
func StepWords() []string {
	var words []string
	for _, entry := range stepWords {
		words = append(words, entry.word)
	}
	return words
}

// KnownStep says whether a word names a step.
func KnownStep(word string) bool {
	for _, entry := range stepWords {
		if entry.word == word {
			return true
		}
	}
	return false
}

type model struct {
	run     *report.Run
	steps   []step
	at      int
	width   int
	height  int
	aborted bool
	summary *summary
	// failed holds steps that hit a fault of the test's own and were passed over.
	failed map[string]string
	// save writes the test to disk when a step begins. Nil in tests.
	save func(step string)
}

// build makes every step this machine can do, in order.
func build(run *report.Run, facts probe.Facts, dir string) []step {
	var steps []step
	covered := map[string]bool{}
	add := func(checks string, s ...step) {
		steps = append(steps, s...)
		for _, check := range strings.Fields(checks) {
			covered[check] = true
		}
	}
	sys := machine.Real()
	if facts.Keyboard != "" {
		add("keyboard", newKeyboard(run), newEffects(run, facts))
	}
	if facts.Touchpad != "" {
		add("touchpad", newTouchpad(run, facts.Touchpad))
	}
	if facts.Touchscreen != "" {
		add("touchscreen", newTouchscreen(run, sys, facts.Touchscreen, facts.TouchscreenDevice))
	}
	if facts.SoundCard != "" && tools.Have("pw-play", "pw-record") {
		speakers, microphones := newSpeakers(run, dir), newMicrophones(run, dir)
		microphones.speakers = speakers
		// Microphones that heard the tones are already measured as working by then; their own
		// step goes on to how well they record, and skipping it keeps that measurement
		add("speakers microphones headphones", speakers, microphones, newHeadphones(run, dir))
	}
	if facts.Camera != "" && tools.Have("cam") {
		add("camera", newCamera(run, dir))
	}
	add("usb", newUSB(run, sys))
	add("external-display", newScreen(run, sys))
	if facts.Laptop && facts.Lid != "" {
		add("suspend", newSleep(run, facts))
	}
	// What has no step of its own on this machine is asked about instead
	return append(steps, newQuestions(run, facts, covered))
}

// choose keeps the steps the given words name.
func choose(steps []step, words []string) []step {
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
	var chosen []step
	for _, s := range steps {
		if wanted[s.name()] {
			chosen = append(chosen, s)
		}
	}
	return chosen
}

// Run shows the guided test and sends its results.
func Run(run *report.Run, facts probe.Facts, options Options) error {
	dir, err := os.MkdirTemp("", "hw-test-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	run.Mode = "guided"
	// A run of chosen steps reports those steps only
	if len(options.Only) > 0 {
		run.Results = nil
	}
	steps := build(run, facts, dir)
	if len(options.Only) > 0 {
		if steps = choose(steps, options.Only); len(steps) == 0 {
			return fmt.Errorf("this computer has nothing to test for %s", strings.Join(options.Only, ", "))
		}
	}

	last := &summary{run: run, options: options}
	m := &model{run: run, summary: last, width: 100, height: 30, failed: map[string]string{}}
	last.failed = m.failed
	sleepMode := machine.Real().Read("/sys/power/mem_sleep")
	m.save = func(step string) { _ = report.SavePending(step, sleepMode, run) }

	// A test found on disk stopped before its end: the computer went down, or the test did
	var opening step = &welcome{run: run, only: len(options.Only) > 0}
	if pending, found := report.LoadPending(); found {
		opening, steps = resume(run, pending, report.BootID(), opening, steps)
		report.ClearPending()
	}

	m.steps = append(append([]step{opening}, steps...), last)
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		// The saved test stays, so the next run picks it up
		return err
	}
	report.ClearPending()
	if final.(*model).aborted {
		fmt.Println("Stopped. Nothing was sent.")
		return nil
	}
	fmt.Println(last.farewell())
	return nil
}

// resume picks a stopped test up at the step it stopped in, with its answers restored. boot
// is this boot's id: a test saved in another boot means the computer restarted since. A
// saved test whose step isn't among these steps is left alone.
func resume(run *report.Run, pending report.Pending, boot string, opening step, steps []step) (step, []step) {
	for at, s := range steps {
		if s.name() != pending.Step {
			continue
		}
		pending.Restore(run)
		back := &resumed{run: run, pending: pending, restarted: pending.Restarted(boot)}
		back.ask = back.question()
		rest := append([]step{&skippable{step: s, skip: func() bool { return back.skip }}}, steps[at+1:]...)
		return back, rest
	}
	return opening, steps
}

func (m *model) Init() tea.Cmd { return m.safeStart(m.steps[0]) }

// caught notes a step's own fault, so the step is passed over and the rest of the test, with
// everything answered so far, carries on.
func (m *model) caught(s step) {
	if fault := recover(); fault != nil {
		m.failed[s.name()] = fmt.Sprint(fault)
	}
}

func (m *model) safeStart(s step) (cmd tea.Cmd) {
	defer m.caught(s)
	return s.start()
}

func (m *model) safeUpdate(s step, msg tea.Msg) (cmd tea.Cmd) {
	defer m.caught(s)
	return s.update(msg)
}

func (m *model) over(s step) (done bool) {
	if _, failed := m.failed[s.name()]; failed {
		return true
	}
	defer func() {
		if fault := recover(); fault != nil {
			m.failed[s.name()] = fmt.Sprint(fault)
			done = true
		}
	}()
	return s.done()
}

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
	cmds := []tea.Cmd{m.safeUpdate(m.steps[m.at], msg)}
	// On to the next step, past any that has nothing to do on this machine
	for m.over(m.steps[m.at]) {
		if m.at == len(m.steps)-1 {
			return m, tea.Quit
		}
		m.at++
		if m.save != nil {
			m.save(m.steps[m.at].name())
		}
		cmds = append(cmds, m.safeStart(m.steps[m.at]))
	}
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	width := min(m.width, 110)
	current := m.steps[m.at]

	// Where the test is: every step's name, the current one marked
	var progress []string
	for i, s := range m.steps {
		_, failed := m.failed[s.name()]
		switch {
		case failed:
			progress = append(progress, close_.Render("! "+s.name()))
		case i < m.at:
			progress = append(progress, good.Render("✓ ")+s.name())
		case i == m.at:
			progress = append(progress, accent.Render("● "+s.name()))
		default:
			progress = append(progress, "○ "+s.name())
		}
	}
	machineName := strings.TrimSpace(m.run.Hardware["maker"] + " " + first(m.run.Hardware["family"], m.run.Hardware["product"]))
	body, hint := m.draw(current, width-4, m.height-8)
	content := lipgloss.JoinVertical(lipgloss.Left,
		title.Render("Are we ARM yet?")+"  "+machineName,
		wrapProgress(progress, width-4),
		"",
		body,
		"",
		wrapHint(hint+" · Ctrl+C to stop", width-4),
	)
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	view.AltScreen = true
	if step, ok := current.(allKeys); ok && step.wantsAllKeys() {
		// Every key as its own event, with the key it is on a US keyboard, so the picture
		// of the keyboard lights up by position whatever the layout
		view.KeyboardEnhancements = tea.KeyboardEnhancements{ReportAlternateKeys: true, ReportAllKeysAsEscapeCodes: true, ReportAssociatedText: true}
	}
	if step, ok := current.(pointer); ok && step.wantsMouse() {
		view.MouseMode = tea.MouseModeAllMotion
	}
	return view
}

// draw is a step's picture and hint line, or a plain note when drawing it fails.
func (m *model) draw(s step, width, height int) (body, hint string) {
	defer func() {
		if fault := recover(); fault != nil {
			m.failed[s.name()] = fmt.Sprint(fault)
			body, hint = "This step ran into a problem of the test's own and is passed over.", "Any key to go on"
		}
	}()
	return s.view(width, height), s.keys()
}

// wrapHint breaks the line of key hints between hints, so none is cut off at the window's
// edge, and a single hint longer than the window wraps like any text.
func wrapHint(hint string, width int) string {
	var lines []string
	line := ""
	for _, item := range strings.Split(hint, " · ") {
		switch {
		case line == "":
			line = item
		case lipgloss.Width(line)+3+lipgloss.Width(item) <= width:
			line += " · " + item
		default:
			lines = append(lines, line)
			line = item
		}
	}
	return wrap(strings.Join(append(lines, line), "\n"), width)
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

// skippable is a step that may be passed over without being shown.
type skippable struct {
	step
	skip func() bool
}

func (s *skippable) done() bool { return s.skip() || s.step.done() }

func (s *skippable) start() tea.Cmd {
	if s.skip() {
		return nil
	}
	return s.step.start()
}

// The wrapped step still gets every key or the pointer when it asks for them.
func (s *skippable) wantsAllKeys() bool {
	inner, ok := s.step.(allKeys)
	return ok && inner.wantsAllKeys()
}

func (s *skippable) wantsMouse() bool {
	inner, ok := s.step.(pointer)
	return ok && inner.wantsMouse()
}

// welcome shows what the quick check measured and what the guided test will do.
type welcome struct {
	run *report.Run
	// only is set when chosen steps are run and not the whole test, which need no introduction.
	only    bool
	started bool
}

func (w *welcome) name() string   { return "Start" }
func (w *welcome) start() tea.Cmd { return nil }
func (w *welcome) done() bool     { return w.started || w.only }
func (w *welcome) keys() string   { return "Enter to start" }

func (w *welcome) update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok && (key.String() == "enter" || key.String() == "space") {
		w.started = true
	}
	return nil
}

func (w *welcome) view(width, height int) string {
	intro := "This checks what works on this computer and puts it on its page on arewearmyet.com, so the next owner knows and the people fixing things see what's left. " +
		"It takes about ten minutes. It has already measured what it can, and now tests the keyboard, touchpad, sound, camera, ports and sleep with you. Any step can be skipped."
	measured := resultsTable(w.run.Results, width)
	return wrap(intro, width) + "\n\n" + bold.Render("Measured just now") + "\n" + measured + "\n\n" +
		wrap("Only results like the ones above are sent, with the model and software versions. No serial numbers, addresses or names.", width) + "\n" +
		good.Render("🔒 ") + wrap("The microphone recording and the camera picture are only for you to check. They never leave this computer and are deleted when the test ends.", width-3)
}

// resumed opens a test that stopped before its end. When the computer has restarted since,
// it asks what happened: a restart in the middle of a step is what that step found, but only
// the owner knows whether the computer went down by itself.
type resumed struct {
	run       *report.Run
	pending   report.Pending
	restarted bool
	ask       choice
	// skip is set when the step it stopped in is to be passed over.
	skip bool
	over bool
}

func (r *resumed) question() choice {
	step := r.pending.Step
	if r.restarted {
		return choice{question: "The last test stopped during its " + step + " step, and the computer has restarted since. Your answers up to there are kept. What happened?",
			options: []string{"The computer crashed or restarted by itself during that step", "I restarted it or closed the test myself"}}
	}
	return choice{question: "The last test stopped during its " + step + " step. Your answers up to there are kept.",
		options: []string{"Try the " + step + " step again", "Skip the " + step + " step"}}
}

func (r *resumed) name() string   { return "Welcome back" }
func (r *resumed) start() tea.Cmd { return nil }
func (r *resumed) done() bool     { return r.over }
func (r *resumed) keys() string   { return choiceKeys }

func (r *resumed) update(msg tea.Msg) tea.Cmd {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	picked := r.ask.update(press)
	if picked < 0 {
		return nil
	}
	r.over = true
	switch {
	case r.restarted && picked == 0:
		// It went down by itself: that is the step's result, and the step isn't run again
		r.skip = true
		if check := stepCheck[r.pending.Step]; check != "" {
			r.pending.Crashed(r.run, check)
		}
	case !r.restarted && picked == 1:
		r.skip = true
	}
	return nil
}

func (r *resumed) view(width, height int) string {
	text := r.ask.view(width)
	if r.restarted {
		text += "\n\n" + wrap("A crash during a step is recorded as what that step found, and the test goes on with the next step.", width)
	}
	return text
}
