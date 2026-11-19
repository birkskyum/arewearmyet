package guided

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/picture"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

type frameMsg struct {
	frame picture.Frame
	err   error
}

// viewedMsg says the picture's window was closed.
type viewedMsg struct{}

// camera takes one picture when the owner says so, shows it in a window of its own at full
// size, and asks whether it looks right. The picture stays on the computer: only how bright
// it is and how much contrast it has are reported.
type camera struct {
	run   *report.Run
	dir   string
	phase string // intro, capturing, viewing, ask
	frame picture.Frame
	err   error
	// viewer is the window the picture is open in, or nil when it is drawn in the terminal only.
	viewer *tools.Viewer
	// open is what shows the picture in a window, which tests replace.
	open func(file string) *tools.Viewer
	ask  choice
	over bool
}

func newCamera(run *report.Run, dir string) *camera {
	return &camera{run: run, dir: dir, phase: "intro", open: tools.View}
}

func (c *camera) name() string   { return "Camera" }
func (c *camera) done() bool     { return c.over }
func (c *camera) start() tea.Cmd { return nil }

func (c *camera) keys() string {
	switch c.phase {
	case "intro":
		return "Enter to take the picture · s to skip"
	case "capturing":
		return "Taking the picture…"
	case "viewing":
		return "Press q in the picture's window to close it · Enter here if no window opened"
	}
	return choiceKeys
}

func (c *camera) picture() string { return filepath.Join(c.dir, "camera.png") }

// cleanup closes the picture's window and deletes the picture.
func (c *camera) cleanup() {
	c.viewer.Close()
	c.viewer = nil
	os.Remove(c.picture())
}

func (c *camera) finish() {
	c.cleanup()
	c.over = true
}

func (c *camera) capture() tea.Cmd {
	c.cleanup()
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

func (c *camera) evidence() string {
	if c.err != nil {
		return c.err.Error()
	}
	mean, contrast := c.frame.Brightness()
	return fmt.Sprintf("a %dx%d frame, brightness %d of 255, contrast %d", c.frame.Width, c.frame.Height, mean, contrast)
}

// question puts up what to ask about the picture just taken.
func (c *camera) question() {
	c.phase = "ask"
	switch {
	case c.err != nil:
		c.ask = choice{question: "The camera gave no picture here. Does it work in other apps, like a video call in the browser?",
			options: []string{"Yes, it works there", "No, it doesn't work anywhere", "Try again", "Skip"}}
	case c.frame.Blank():
		c.ask = choice{question: "The picture came out black or blank. Is the camera covered, or its shutter closed?",
			options: []string{"It was covered: take another", "No, nothing covers it", "Skip"}}
	default:
		c.ask = choice{question: "Was that a real picture of what's in front of the camera?",
			options: []string{"Yes, and the colours look right", "Yes, but it's too dark, pale or off in colour", "No, it's black, frozen or garbage", "Take another", "Skip"}}
	}
}

func (c *camera) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case frameMsg:
		c.frame, c.err = msg.frame, msg.err
		if c.err != nil || c.frame.Blank() {
			c.question()
			return nil
		}
		// A real picture: shown at full size in a window, which the owner closes with q
		if file, err := c.frame.PNG(); err == nil && os.WriteFile(c.picture(), file, 0o600) == nil {
			c.viewer = c.open(c.picture())
		}
		if c.viewer == nil {
			c.question()
			return nil
		}
		c.phase = "viewing"
		closed := c.viewer.Closed
		return func() tea.Msg {
			<-closed
			return viewedMsg{}
		}
	case viewedMsg:
		if c.phase == "viewing" {
			c.question()
		}
	case tea.KeyPressMsg:
		switch c.phase {
		case "intro":
			switch msg.String() {
			case "enter", "space":
				return c.capture()
			case "s":
				c.finish()
			}
			return nil
		case "viewing":
			if msg.String() == "enter" {
				c.viewer.Close()
				c.question()
			}
			return nil
		case "capturing":
			return nil
		}
		set := func(state, how, note string) {
			c.run.Set("camera", state, how, c.evidence(), note)
			c.finish()
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
				c.finish()
			}
		case c.frame.Blank():
			switch picked {
			case 0:
				return c.capture()
			case 1:
				set(report.Broken, report.Measured, "The frame is black or blank with nothing covering the camera.")
			default:
				c.finish()
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
				c.finish()
			}
		}
	}
	return nil
}

func (c *camera) view(width, height int) string {
	switch c.phase {
	case "intro":
		return wrap("Now the camera. When you press Enter it takes one picture and opens it in a window, so you can see whether the camera works. Cover the camera first if you'd rather not be in it: a picture of anything shows it works.", width) + "\n\n" + private(width)
	case "capturing":
		return accent.Render("● Taking a picture with the camera") + "\n\n" + "Its light may come on for a moment."
	case "viewing":
		return accent.Render("The picture is open in its own window.") + "\n\n" +
			wrap("Have a look at it, then press q in that window to close it and come back here.", width) + "\n\n" + private(width)
	}
	if c.err != nil || c.frame.Blank() {
		return c.ask.view(width) + "\n\n" + wrap("Measured: "+c.evidence()+".", width)
	}
	// A small copy of the picture as a reminder, in whatever height the question leaves
	rows := max(4, min(14, height-len(c.ask.options)-9))
	return c.frame.Render(min(width, 60), rows) + "\n\n" + c.ask.view(width) + "\n\n" +
		wrap("Measured: "+c.evidence()+". Only these numbers are sent, never the picture, which is deleted when you answer.", width)
}
