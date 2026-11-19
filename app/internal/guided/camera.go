package guided

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/picture"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

type frameMsg struct {
	frame picture.Frame
	err   error
}

// camera takes a frame, draws it in the terminal and asks whether it looks right.
type camera struct {
	run   *report.Run
	dir   string
	phase string // capturing, ask
	frame picture.Frame
	err   error
	ask   choice
	over  bool
}

func newCamera(run *report.Run, dir string) *camera {
	return &camera{run: run, dir: dir, phase: "capturing"}
}

func (c *camera) name() string { return "Camera" }
func (c *camera) done() bool   { return c.over }

func (c *camera) keys() string {
	if c.phase == "capturing" {
		return "Taking a picture…"
	}
	return choiceKeys
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

func (c *camera) start() tea.Cmd { return c.capture() }

func (c *camera) evidence() string {
	if c.err != nil {
		return c.err.Error()
	}
	mean, contrast := c.frame.Brightness()
	return fmt.Sprintf("a %dx%d frame, brightness %d of 255, contrast %d", c.frame.Width, c.frame.Height, mean, contrast)
}

func (c *camera) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
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
		if c.phase != "ask" {
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
				return c.capture()
			default:
				c.over = true
			}
		case c.frame.Blank():
			switch picked {
			case 0:
				return c.capture()
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
				return c.capture()
			default:
				c.over = true
			}
		}
	}
	return nil
}

func (c *camera) view(width, height int) string {
	if c.phase == "capturing" {
		return accent.Render("● Taking a picture with the camera") + "\n\n" + dim.Render("Its light may come on for a moment.")
	}
	if c.err != nil || c.frame.Blank() {
		return c.ask.view(width) + "\n\n" + dim.Render(wrap("Measured: "+c.evidence()+".", width))
	}
	// The picture gets whatever height the question leaves
	rows := max(6, height-len(c.ask.options)-6)
	return c.frame.Render(width, rows) + "\n\n" + c.ask.view(width) + "\n\n" + dim.Render("Measured: "+c.evidence()+".")
}
