package guided

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/picture"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

type frameMsg struct {
	frame picture.Frame
	err   error
}

// countMsg is one second of the countdown before the picture.
type countMsg struct{}

// camera takes a frame, draws it in the terminal and asks whether it looks right.
type camera struct {
	run   *report.Run
	dir   string
	phase string // intro, countdown, capturing, ask
	count int
	frame picture.Frame
	err   error
	ask   choice
	over  bool
}

func newCamera(run *report.Run, dir string) *camera {
	return &camera{run: run, dir: dir, phase: "intro"}
}

func (c *camera) name() string { return "Camera" }
func (c *camera) done() bool   { return c.over }

func (c *camera) keys() string {
	switch c.phase {
	case "intro":
		return "Enter when you're ready · s to skip"
	case "countdown":
		return "s to skip"
	case "capturing":
		return "Taking the picture…"
	}
	return choiceKeys
}

// countdown gives a few seconds' warning before the picture is taken.
func (c *camera) countdown(from int) tea.Cmd {
	c.phase, c.count = "countdown", from
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return countMsg{} })
}

func (c *camera) capture() tea.Cmd {
	c.phase = "capturing"
	dir := c.dir
	return func() tea.Msg {
		data, err := tools.Frame(dir)
		if err != nil {
			return frameMsg{err: err}
		}
		frame, err := picture.PPM(data)
		return frameMsg{frame: frame, err: err}
	}
}

func (c *camera) start() tea.Cmd { return nil }

func (c *camera) evidence() string {
	if c.err != nil {
		return c.err.Error()
	}
	mean, contrast := c.frame.Brightness()
	return fmt.Sprintf("a %dx%d frame, brightness %d of 255, contrast %d", c.frame.Width, c.frame.Height, mean, contrast)
}

func (c *camera) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case countMsg:
		if c.phase != "countdown" || c.over {
			return nil
		}
		c.count--
		if c.count <= 0 {
			return c.capture()
		}
		return tea.Tick(time.Second, func(time.Time) tea.Msg { return countMsg{} })
	case frameMsg:
		c.frame, c.err, c.phase = msg.frame, msg.err, "ask"
		switch {
		case c.err != nil:
			c.ask = choice{question: "The camera gave no picture here. Does it work in other apps, like a video call in the browser?",
				options: []string{"Yes, it works there", "No, it doesn't work anywhere", "Try again", "Skip"}}
		case c.frame.Blank():
			c.ask = choice{question: "The picture came out black or blank. Is the camera covered, or its shutter closed?",
				options: []string{"It was covered: take another", "No, nothing covers it", "Skip"}}
		default:
			c.ask = choice{question: "This is what the camera sees, drawn coarsely. Is it a real picture of what's in front of it?",
				options: []string{"Yes, and the colours look right", "Yes, but it's too dark, pale or off in colour", "No, it's black, frozen or garbage", "Take another", "Skip"}}
		}
	case tea.KeyPressMsg:
		switch c.phase {
		case "intro":
			switch msg.String() {
			case "enter", "space":
				return c.countdown(5)
			case "s":
				c.over = true
			}
			return nil
		case "countdown":
			if msg.String() == "s" {
				c.over = true
			}
			return nil
		case "capturing":
			return nil
		}
		set := func(state, how, note string) {
			c.run.Set("camera", state, how, c.evidence(), note)
			c.over = true
		}
		picked := c.ask.update(msg)
		if picked < 0 {
			return nil
		}
		switch {
		case c.err != nil:
			switch picked {
			case 0:
				set(report.Partial, report.Owner, "The camera works in apps, but libcamera's cam tool got no frame from it.")
			case 1:
				set(report.Broken, report.Owner, "")
			case 2:
				return c.countdown(3)
			default:
				c.over = true
			}
		case c.frame.Blank():
			switch picked {
			case 0:
				return c.countdown(3)
			case 1:
				set(report.Broken, report.Measured, "The frame is black or blank with nothing covering the camera.")
			default:
				c.over = true
			}
		default:
			switch picked {
			case 0:
				set(report.Works, report.Owner, "")
			case 1:
				set(report.Partial, report.Owner, "There is a picture, but too dark, pale or off in colour.")
			case 2:
				set(report.Broken, report.Owner, "The picture is black, frozen or garbage.")
			case 3:
				return c.countdown(3)
			default:
				c.over = true
			}
		}
	}
	return nil
}

func (c *camera) view(width, height int) string {
	switch c.phase {
	case "intro":
		return wrap("Now the camera. It takes one picture and draws it here, so you can see whether the camera works. Nothing is taken until you press Enter, and then it counts down from five.", width) + "\n\n" + private(width)
	case "countdown":
		return accent.Render(fmt.Sprintf("Taking a picture in  %d", c.count)) + "\n\n" + soft.Render("Look at the camera, or cover it if you'd rather not be in it: a picture of anything shows it works.") + "\n\n" + private(width)
	}
	if c.phase == "capturing" {
		return accent.Render("● Taking a picture with the camera") + "\n\n" + soft.Render("Its light may come on for a moment.") + "\n\n" + private(width)
	}
	if c.err != nil || c.frame.Blank() {
		return c.ask.view(width) + "\n\n" + soft.Render(wrap("Measured: "+c.evidence()+".", width))
	}
	// The picture gets whatever height the question leaves
	rows := max(6, height-len(c.ask.options)-8)
	return c.frame.Render(width, rows) + "\n\n" + c.ask.view(width) + "\n\n" + soft.Render("Measured: "+c.evidence()+". Only these numbers are sent.") + "\n" + private(width)
}
