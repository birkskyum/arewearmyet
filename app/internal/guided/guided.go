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
	"sort"
	"strings"
	"time"

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

// opening is implemented by the screens before the test proper, which show no list of steps:
// the menu on them is the list.
type opening interface{ isOpening() bool }

// cleaner is implemented by a step that has something open to close when the test ends early,
// like the camera's picture window.
type cleaner interface{ cleanup() }

// The words a step can be asked for by on the command line, and the steps each one runs.
var stepWords = []struct {
	word  string
	steps []string
}{
	{"keyboard", []string{"Keyboard", "Volume and brightness keys"}},
	{"keys", []string{"Volume and brightness keys"}},
	{"touchpad", []string{"Touchpad"}},
	{"touchscreen", []string{"Touchscreen"}},
	{"sound", []string{"Sound", "Headphones"}},
	{"speakers", []string{"Sound"}},
	{"microphone", []string{"Sound"}},
	{"voice", []string{"Voice"}},
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
	"Sound": "speakers", "Voice": "microphones", "Headphones": "headphones", "Camera": "camera",
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
	// skipped holds the steps the owner passed over with Ctrl+N.
	skipped map[string]bool
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
		// The sound step measures the speakers and the microphones together. The voice step,
		// with the owner speaking, is only for microphones that step couldn't hear anything with
		add("speakers microphones headphones", speakers, &skippable{step: microphones, skip: speakers.heardTone}, newHeadphones(run, dir))
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

// Run shows the guided test and sends its results. Started without a step named, it opens on
// a menu, and comes back to that menu after a single part, until the whole test has run or
// the owner quits.
func Run(run *report.Run, facts probe.Facts, options Options) error {
	dir, err := os.MkdirTemp("", "hw-test-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	run.Mode = "guided"
	// What the quick check measured, put back before each round: a part alone reports only itself
	measured := append([]report.Result(nil), run.Results...)
	_ = report.Remember(run, time.Now())
	var said []string
	for round := 0; ; round++ {
		run.Results = append([]report.Result(nil), measured...)
		outcome, err := once(run, facts, options, dir, round == 0)
		if err != nil {
			return err
		}
		if outcome.said != "" {
			said = append(said, outcome.said)
		}
		if !outcome.again {
			break
		}
	}
	if len(said) > 0 {
		fmt.Println(strings.Join(said, "\n"))
	}
	return nil
}

// outcome is how one round of the test ended.
type outcome struct {
	// said is what to leave in the terminal about it.
	said string
	// again is set when a single part was done from the menu, which comes up again.
	again bool
}

// once runs the menu and whatever is chosen from it, or the steps named on the command line.
func once(run *report.Run, facts probe.Facts, options Options, dir string, initial bool) (outcome, error) {
	// A run of chosen steps reports those steps only
	if len(options.Only) > 0 {
		run.Results = nil
	}
	steps := build(run, facts, dir)
	if len(options.Only) > 0 {
		if steps = choose(steps, options.Only); len(steps) == 0 {
			return outcome{}, fmt.Errorf("this computer has nothing to test for %s", strings.Join(options.Only, ", "))
		}
	}

	last := &summary{run: run, options: options, dir: dir}
	m := &model{run: run, summary: last, width: 100, height: 30, failed: map[string]string{}}
	last.failed = m.failed
	sleepMode := machine.Real().Read("/sys/power/mem_sleep")
	m.save = func(step string) { _ = report.SavePending(step, sleepMode, run) }

	// Run without a step named, the test opens on a menu: all of it, or one part alone
	// What the round found is kept on the machine as its latest, whether or not it is sent
	defer func() { _ = report.Remember(run, time.Now()) }()
	start := &welcome{run: run, only: len(options.Only) > 0, kept: report.Local()}
	for _, entry := range menu {
		if len(choose(steps, []string{entry.word})) > 0 {
			start.parts = append(start.parts, entry)
		}
	}
	start.pick = func(word string) {
		// One part alone reports that part only
		run.Results = nil
		m.steps = append(append([]step{start}, choose(steps, []string{word})...), last)
	}
	start.ask = start.question()
	start.api = options.API

	// A test found on disk stopped before its end: the computer went down, or the test did.
	// It stays on disk until the owner has gone on from it, so opening the test and leaving
	// again loses nothing.
	var opening step = start
	resuming := false
	// Only the first round looks: later ones follow a part that just finished
	if pending, found := report.LoadPending(); found && initial {
		opening, steps = resume(run, pending, report.BootID(), opening, steps)
		_, resuming = opening.(*resumed)
	}

	m.steps = append(append([]step{opening}, steps...), last)
	final, err := tea.NewProgram(m).Run()
	// Whatever a step left open is closed, however the test ended
	for _, s := range steps {
		if open, ok := s.(cleaner); ok {
			open.cleanup()
		}
	}
	if err != nil {
		// The saved test stays, so the next run picks it up
		return outcome{}, err
	}
	ended := final.(*model)
	// A test left on its first screen wasn't begun: what was saved before it stays as it was
	if !(resuming && ended.at == 0) {
		report.ClearPending()
	}
	switch {
	case ended.aborted:
		return outcome{said: "Stopped. Nothing more was sent."}, nil
	case start.quit:
		return outcome{}, nil
	}
	// A part chosen from the menu leads back to the menu; the whole test, or steps named on
	// the command line, end here
	return outcome{said: last.farewell(), again: start.part != ""}, nil
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
	if _, failed := m.failed[s.name()]; failed || m.skipped[s.name()] {
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
		switch msg.String() {
		case "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		case "ctrl+n":
			// On to the next step from anywhere, whatever the step itself does with its keys.
			// The last step, which sends the results, isn't one to skip.
			if m.at < len(m.steps)-1 {
				if m.skipped == nil {
					m.skipped = map[string]bool{}
				}
				m.skipped[m.steps[m.at].name()] = true
			}
			msg = tea.KeyPressMsg{}
		}
	}
	var cmds []tea.Cmd
	if !m.skipped[m.steps[m.at].name()] {
		cmds = append(cmds, m.safeUpdate(m.steps[m.at], msg))
	}
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
		case m.skipped[s.name()]:
			progress = append(progress, "– "+s.name())
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
	lines := []string{title.Render("Are we ARM yet?") + "  " + machineName}
	skip := m.skipHint()
	if start, ok := current.(opening); ok && start.isOpening() {
		// The start screen's menu is the list of what there is; a second list above it only repeats it
		skip = ""
	} else {
		lines = append(lines, wrapProgress(progress, width-4))
	}
	lines = append(lines, "", body, "", wrapHint(hint+skip+" · Ctrl+C to stop", width-4))
	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
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

// skipHint names the key that passes any step over, on every step but the last.
func (m *model) skipHint() string {
	if m.at < len(m.steps)-1 {
		return " · Ctrl+N for the next step"
	}
	return ""
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

func (s *skippable) cleanup() {
	if inner, ok := s.step.(cleaner); ok {
		inner.cleanup()
	}
}

// menuEntry is a part the opening menu offers: the word that runs it alone, and the checks it
// tests, whose latest results on this machine are the part's status.
type menuEntry struct {
	word, label string
	checks      []string
}

var menu = []menuEntry{
	{"keyboard", "Keyboard, with the volume and brightness keys", []string{"keyboard", "backlight"}},
	{"touchpad", "Touchpad", []string{"touchpad"}},
	{"touchscreen", "Touchscreen", []string{"touchscreen"}},
	{"sound", "Sound: speakers, microphone and headphones", []string{"speakers", "microphones", "headphones"}},
	{"camera", "Camera", []string{"camera"}},
	{"usb", "USB ports", []string{"usb"}},
	{"screen", "External screen", []string{"external-display"}},
	{"sleep", "Sleep", []string{"suspend"}},
}

// severity orders states from fine to failing, for a part's or the machine's worst.
var severity = map[string]int{report.Works: 1, report.Absent: 1, report.Partial: 2, report.Broken: 3}

// status is a menu part's standing from the machine's latest results: its worst check, said
// in a line and coloured, or that it hasn't been tested here.
func (e menuEntry) status(kept map[string]report.Kept, width int) string {
	var worst report.Kept
	tested := 0
	for _, check := range e.checks {
		if result, ok := kept[check]; ok {
			tested++
			if severity[result.State] > severity[worst.State] {
				worst = result
			}
		}
	}
	if tested == 0 {
		return "○ not tested here yet"
	}
	when := worst.When.Format("2 Jan")
	switch worst.State {
	case report.Partial, report.Broken:
		why := first(worst.Note, worst.Evidence)
		line := map[string]string{report.Partial: "◐ partly", report.Broken: "✗ broken"}[worst.State] + " (" + checkTitle[worst.Check] + ", " + when + ")"
		if why != "" {
			line += ": " + why
		}
		if letters := []rune(line); len(letters) > width-6 && width > 20 {
			line = string(letters[:width-7]) + "…"
		}
		return stateStyle(worst.State).Render(line)
	}
	line := "✓ works, tested " + when
	if tested < len(e.checks) {
		line = fmt.Sprintf("✓ works so far (%d of %d parts tested, %s)", tested, len(e.checks), when)
	}
	return good.Render(line)
}

// welcome is the opening screen: what the test does, and a menu of all of it or one part.
type welcome struct {
	run *report.Run
	// only is set when the steps were named on the command line, which need no menu.
	only bool
	// parts are the menu's entries for what this machine has.
	parts []menuEntry
	// kept is the machine's latest result for each check, which the menu shows as its status.
	kept map[string]report.Kept
	// pick narrows the test to one part.
	pick func(word string)
	ask  choice
	// api is the site, asked which model this computer is so its page can be shown.
	api string
	// model is the site's answer, and looked whether it has answered.
	model   report.Model
	looked  bool
	started bool
	// part is the word of the part chosen from the menu, or "" for the whole test.
	part string
	// quit is set when the owner chose to leave from the menu.
	quit bool
}

// modelMsg is the site's answer to which model this computer is.
type modelMsg struct {
	model report.Model
	err   error
}

func (w *welcome) question() choice {
	options, notes := []string{"Everything, about ten minutes"}, []string{""}
	for _, part := range w.parts {
		options = append(options, part.label)
		notes = append(notes, part.status(w.kept, 100))
	}
	return choice{question: "What would you like to test?", options: append(options, "Quit"), notes: notes}
}

// standing is where the machine stands over all checks: how many work, and what doesn't that
// no part of the menu covers, like something only measured.
func (w *welcome) standing(width int) string {
	counts := map[string]int{}
	var failing []string
	inMenu := map[string]bool{}
	for _, part := range w.parts {
		for _, check := range part.checks {
			inMenu[check] = true
		}
	}
	for check := range checkTitle {
		result, ok := w.kept[check]
		if !ok {
			counts["untested"]++
			continue
		}
		switch severity[result.State] {
		case 1:
			counts["works"]++
		case 2:
			counts["partly"]++
		case 3:
			counts["broken"]++
		}
		if severity[result.State] > 1 && !inMenu[check] {
			mark := map[int]string{2: "◐ ", 3: "✗ "}[severity[result.State]]
			line := mark + checkTitle[check] + ": " + first(result.Note, result.Evidence, stateLabel[result.State])
			if letters := []rune(line); len(letters) > width-2 && width > 20 {
				line = string(letters[:width-3]) + "…"
			}
			failing = append(failing, stateStyle(result.State).Render(line))
		}
	}
	sort.Strings(failing)
	line := fmt.Sprintf("This computer so far: %s, %s, %s, %d not tested.",
		good.Render(fmt.Sprintf("%d working", counts["works"])), close_.Render(fmt.Sprintf("%d partly", counts["partly"])), bad.Render(fmt.Sprintf("%d broken", counts["broken"])), counts["untested"])
	if counts["partly"]+counts["broken"]+counts["untested"] == 0 {
		line = good.Render("Everything works on this computer: all green.")
	}
	if len(failing) > 5 {
		failing = append(failing[:5], fmt.Sprintf("and %d more: hw-test status lists everything", len(failing)-5))
	}
	return strings.Join(append([]string{line}, failing...), "\n")
}

func (w *welcome) name() string    { return "Start" }
func (w *welcome) done() bool      { return w.started || w.only }
func (w *welcome) isOpening() bool { return true }

func (w *welcome) keys() string {
	if w.model.Page != "" {
		return choiceKeys + " · o to open this computer's page"
	}
	return choiceKeys
}

func (w *welcome) start() tea.Cmd {
	if w.only || w.api == "" {
		return nil
	}
	api, run := w.api, w.run
	return func() tea.Msg {
		model, err := report.Match(api, run)
		return modelMsg{model, err}
	}
}

func (w *welcome) update(msg tea.Msg) tea.Cmd {
	if answer, ok := msg.(modelMsg); ok {
		w.looked = true
		if answer.err == nil {
			w.model = answer.model
		}
		return nil
	}
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if press.String() == "o" && w.model.Page != "" {
		tools.Open(w.model.Page)
		return nil
	}
	if len(w.ask.options) == 0 {
		w.ask = w.question()
	}
	picked := w.ask.update(press)
	switch {
	case picked < 0:
		return nil
	case picked > len(w.parts):
		w.quit = true
		return tea.Quit
	case picked > 0 && w.pick != nil:
		w.part = w.parts[picked-1].word
		w.pick(w.part)
	}
	w.started = true
	return nil
}

func (w *welcome) view(width, height int) string {
	if len(w.ask.options) == 0 {
		w.ask = w.question()
	}
	intro := "This checks what works on this computer and puts it on its page on arewearmyet.com, so the next owner knows and the people fixing things see what's left. It measures what it can by itself and tests the rest with you."
	intro += "\n\n" + w.standing(width)
	// Where this computer's results are, once the site has said which model it is
	page := ""
	switch {
	case w.model.Page != "":
		page = "\n\n" + wrap("This computer's page, with what's known and tested so far:", width) + "\n" + accent.Hyperlink(w.model.Page).Render(w.model.Page)
	case w.looked && w.model.ID == "" && w.api != "":
		page = "\n\n" + wrap("The board doesn't have this computer yet. Sending a test adds what it finds, with a form to put the model on the board.", width)
	}
	return wrap(intro, width) + page + "\n\n" + w.ask.view(width) + "\n\n" +
		wrap("After a single part you come back to this menu. To go straight to one part next time, add its name to the command, like hw-test camera or hw-test sound.", width) + "\n\n" +
		wrap("Only results are sent, with the model and software versions. No serial numbers, addresses or names.", width) + "\n" +
		good.Render("🔒 ") + wrap("A picture or recording the test makes never leaves this computer and is deleted when the test ends.", width-3)
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

func (r *resumed) name() string    { return "Welcome back" }
func (r *resumed) isOpening() bool { return true }
func (r *resumed) start() tea.Cmd  { return nil }
func (r *resumed) done() bool      { return r.over }
func (r *resumed) keys() string    { return choiceKeys }

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

// Status lists the machine's latest result for every check, for `hw-test status`: what works
// here, what doesn't and why, and what hasn't been tested, without running anything.
func Status(kept map[string]report.Kept) string {
	var checks []string
	for check := range checkTitle {
		checks = append(checks, check)
	}
	// What fails first, then what works, then what is untested; by name within each
	rank := func(check string) int {
		result, ok := kept[check]
		if !ok {
			return 0
		}
		return severity[result.State]
	}
	sort.Slice(checks, func(a, b int) bool {
		if rank(checks[a]) != rank(checks[b]) {
			return rank(checks[a]) > rank(checks[b])
		}
		return checkTitle[checks[a]] < checkTitle[checks[b]]
	})
	var lines []string
	counts := map[int]int{}
	for _, check := range checks {
		result, ok := kept[check]
		counts[rank(check)]++
		if !ok {
			lines = append(lines, fmt.Sprintf("  ○ %-30s not tested", checkTitle[check]))
			continue
		}
		mark := map[int]string{1: "✓", 2: "◐", 3: "✗"}[severity[result.State]]
		line := fmt.Sprintf("  %s %-30s %-18s %s  %s", mark, checkTitle[check], stateLabel[result.State], result.When.Format("2006-01-02"), result.Kernel)
		if why := first(result.Note, result.Evidence); why != "" && severity[result.State] > 1 {
			line += "\n      " + why
		}
		lines = append(lines, line)
	}
	head := fmt.Sprintf("%d working, %d partly, %d broken, %d not tested", counts[1], counts[2], counts[3], counts[0])
	return head + "\n\n" + strings.Join(lines, "\n")
}
