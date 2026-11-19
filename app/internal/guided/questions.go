package guided

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// question is something only the owner can tell, about hardware the machine has.
type question struct {
	check string
	text  string
}

// questions asks the few things nothing measured. Each can be answered from the chair, or
// passed over when it hasn't been tried.
type questions struct {
	run   *report.Run
	list  []question
	at    int
	ask   choice
	note  *line
	state string
}

// newQuestions leaves out what the machine doesn't have, what was already measured as working
// and what a step of its own covers. Hardware whose step can't run here, for want of a tool,
// is asked about instead.
func newQuestions(run *report.Run, facts probe.Facts, covered map[string]bool) *questions {
	unmeasured := func(check string) bool { return run.State(check) != report.Works }
	q := &questions{run: run}
	add := func(when bool, check, text string) {
		if when {
			q.list = append(q.list, question{check, text})
		}
	}
	unlock, installer := "When you started the laptop, did you see the disk password prompt and type into it with the built-in keyboard?",
		"If you installed Omarchy on this computer yourself: did the installer start from the USB stick with keyboard and touchpad working?"
	if !facts.Laptop {
		// A mini PC or desktop has no keyboard or touchpad of its own to ask about
		unlock, installer = "When you started the computer, did you see the disk password prompt, and could you type into it?",
			"If you installed Omarchy on this computer yourself: did the installer start from the USB stick, with a picture on the screen and the keyboard working?"
	}
	add(facts.Encrypted, "disk-unlock", unlock)
	add(facts.Touchpad != "" && !covered["touchpad"], "touchpad", "Does the touchpad move the pointer, click and scroll with two fingers?")
	add(facts.SoundCard != "" && !covered["speakers"], "speakers", "Do the speakers play sound, on both sides?")
	add(facts.SoundCard != "" && !covered["microphones"], "microphones", "Does the microphone pick you up in calls or recordings?")
	add(facts.Camera != "" && !covered["camera"], "camera", "Does the webcam give a picture in the browser?")
	add(!covered["usb"], "usb", "Have USB sticks and other devices worked in every USB port you've tried?")
	add(!covered["external-display"] && unmeasured("external-display"), "external-display", "An external screen, if you've tried one: did it show a picture?")
	add(facts.Laptop && unmeasured("charging"), "charging", "With its own charger plugged in, does the battery percentage go up?")
	add(facts.Fingerprint != "", "fingerprint", "The fingerprint reader, if you've set it up: does it unlock with your finger?")
	add(facts.Bluetooth != "" && unmeasured("bluetooth"), "bluetooth", "Bluetooth headphones or a mouse, if you've tried some: do they pair and stay connected?")
	add(true, "installer", installer)
	q.show()
	return q
}

// show puts the current question's answers up.
func (q *questions) show() {
	if q.done() {
		return
	}
	current := q.list[q.at]
	q.ask = choice{question: current.text, options: []string{"Yes", "Partly", "No", "Haven't tried"}}
	q.note = nil
}

func (q *questions) name() string   { return "Questions" }
func (q *questions) start() tea.Cmd { return nil }
func (q *questions) done() bool     { return q.at >= len(q.list) }

func (q *questions) keys() string {
	if q.note != nil {
		return "Type, then Enter"
	}
	return choiceKeys
}

func (q *questions) next() {
	q.at++
	q.show()
}

func (q *questions) update(msg tea.Msg) tea.Cmd {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok || q.done() {
		return nil
	}
	current := q.list[q.at]
	if q.note != nil {
		if q.note.update(press) {
			q.run.Set(current.check, q.state, report.Owner, "", q.note.value())
			q.next()
		}
		return nil
	}
	switch q.ask.update(press) {
	case 0:
		q.run.Set(current.check, report.Works, report.Owner, "", "")
		q.next()
	case 1:
		q.state = report.Partial
		q.note = &line{prompt: "What doesn't work? (Enter to leave it empty)"}
	case 2:
		q.state = report.Broken
		q.note = &line{prompt: "What happens? (Enter to leave it empty)"}
	case 3:
		q.next()
	}
	return nil
}

func (q *questions) view(width, height int) string {
	if q.done() {
		return ""
	}
	count := soft.Render(fmt.Sprintf("Question %d of %d", q.at+1, len(q.list)))
	if q.note != nil {
		return count + "\n\n" + q.note.view(width)
	}
	return count + "\n\n" + q.ask.view(width)
}
