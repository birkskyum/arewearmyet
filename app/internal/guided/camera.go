package guided

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/picture"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

type frameMsg struct {
	frame picture.Frame
	// took is how long the camera ran before this frame, and settled whether its picture had
	// stopped getting brighter or darker by then.
	took    time.Duration
	settled bool
	err     error
}

// camera takes one picture when the owner says so, draws it in the terminal and asks whether
// it looks right. A window with the picture at full size opens only when asked for. The
// picture stays on the computer: only how bright it is and how much contrast it has are
// reported.
type camera struct {
	run     *report.Run
	dir     string
	phase   string // intro, capturing, ask
	frame   picture.Frame
	took    time.Duration
	settled bool
	err     error
	// viewer is the window the picture is open in at full size, or nil.
	viewer *tools.Viewer
	// open is what shows the picture in a window, which tests replace.
	open func(file string) *tools.Viewer
	// canOpen says whether there is a viewer to open it in.
	canOpen bool
	// inTerminal says the terminal draws pictures itself, sharp. Otherwise the picture is drawn
	// in coloured blocks.
	inTerminal bool
	// columns and rows are the cells the terminal was told to draw the picture in, when it was.
	columns, rows int
	// width and height are the space the step was last drawn in.
	width, height int
	ask           choice
	over          bool
}

func newCamera(run *report.Run, dir string) *camera {
	return &camera{run: run, dir: dir, phase: "intro", open: tools.View, canOpen: tools.Have("imv"), inTerminal: picture.InTerminal()}
}

func (c *camera) name() string   { return "Camera" }
func (c *camera) done() bool     { return c.over }
func (c *camera) start() tea.Cmd { return nil }

// shown says whether a real picture is on screen, which can then be opened at full size.
func (c *camera) shown() bool {
	return c.phase == "ask" && c.err == nil && !c.frame.Blank()
}

func (c *camera) keys() string {
	switch c.phase {
	case "intro":
		return "Enter to take the picture · s to skip"
	case "capturing":
		return "Taking the picture…"
	}
	if c.shown() && c.canOpen {
		return choiceKeys + " · o to see it at full size"
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

// The terminal holds one picture for the test, under this number.
const pictureID = 77

// space is how many columns and rows the picture may take, beside the question.
func (c *camera) space(width, height int) (int, int) {
	return min(width, 96), max(4, min(30, height-len(c.ask.options)-11))
}

// hand gives the picture to the terminal to draw, in the cells the step has room for.
func (c *camera) hand() tea.Cmd {
	c.columns, c.rows = 0, 0
	if !c.inTerminal || !c.shown() {
		return nil
	}
	columns, rows := c.frame.Fit(c.space(c.width, c.height))
	sequence, err := c.frame.Hand(pictureID, columns, rows)
	if columns == 0 || err != nil {
		return nil
	}
	c.columns, c.rows = columns, rows
	return tea.Raw(sequence)
}

// forget makes the terminal drop the picture once the step is over.
func (c *camera) forget() tea.Cmd {
	if c.columns == 0 {
		return nil
	}
	c.columns, c.rows = 0, 0
	return tea.Raw(picture.Forget(pictureID))
}

func (c *camera) capture() tea.Cmd {
	c.cleanup()
	c.phase = "capturing"
	dir := c.dir
	return func() tea.Msg {
		shot, err := tools.Frame(dir)
		if err != nil {
			return frameMsg{err: err}
		}
		frame, err := picture.PPM(shot.PPM)
		return frameMsg{frame: frame, took: shot.Took, settled: shot.Settled, err: err}
	}
}

func (c *camera) evidence() string {
	if c.err != nil {
		return c.err.Error()
	}
	mean, contrast := c.frame.Brightness()
	evidence := fmt.Sprintf("a %dx%d frame, brightness %d of 255, contrast %d", c.frame.Width, c.frame.Height, mean, contrast)
	seconds := int(c.took.Round(time.Second).Seconds())
	switch {
	case !c.settled && seconds > 0:
		evidence += fmt.Sprintf(", still adjusting to the light after %d s", seconds)
	case seconds >= 3:
		evidence += fmt.Sprintf(", after %d s of adjusting to the light", seconds)
	}
	return evidence
}

// question puts up what to ask about the picture just taken.
func (c *camera) question() {
	c.phase = "ask"
	switch {
	case errors.Is(c.err, tools.ErrCameraBusy):
		c.ask = choice{question: "Another program is using the camera, like a browser tab with a video call or a camera page. Close it, then try again.",
			options: []string{"Try again", "Skip"}}
	case c.err != nil:
		c.ask = choice{question: "The camera gave no picture here. Does it work in other apps, like a video call in the browser?",
			options: []string{"Yes, it works there", "No, it doesn't work anywhere", "Try again", "Skip"}}
	case c.frame.Blank():
		c.ask = choice{question: "The picture came out black or blank. Is the camera covered, or its shutter closed?",
			options: []string{"It was covered: take another", "No, nothing covers it", "Skip"}}
	default:
		c.ask = choice{question: "Is that a real picture of what's in front of the camera?",
			options: []string{"Yes, and the colours look right", "Yes, but it's too dark, pale or off in colour", "No, it's black, frozen or garbage", "Take another", "Skip"}}
	}
}

func (c *camera) update(msg tea.Msg) (dropped tea.Cmd) {
	switch msg := msg.(type) {
	case frameMsg:
		c.frame, c.took, c.settled, c.err = msg.frame, msg.took, msg.settled, msg.err
		c.question()
		return c.hand()
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
		case "capturing":
			return nil
		}
		// The picture at full size, in a window of its own that q closes. Answering closes it too.
		if msg.String() == "o" && c.shown() && c.viewer == nil {
			if file, err := c.frame.PNG(); err == nil && os.WriteFile(c.picture(), file, 0o600) == nil {
				c.viewer = c.open(c.picture())
			}
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
		// Whatever the answer, the terminal lets go of the picture
		defer func() {
			if c.over {
				dropped = c.forget()
			}
		}()
		switch {
		case errors.Is(c.err, tools.ErrCameraBusy):
			if picked == 0 {
				return c.capture()
			}
			c.finish()
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
	c.width, c.height = width, height
	switch c.phase {
	case "intro":
		return wrap("Now the camera. When you press Enter it takes one picture and shows it here, so you can see whether the camera works. Cover the camera first if you'd rather not be in it: a picture of anything shows it works.", width) + "\n\n" + private(width)
	case "capturing":
		return accent.Render("● Taking a picture with the camera") + "\n\n" +
			wrap("It waits until the camera has adjusted to the light, which takes a few seconds, and up to fifteen in a dim room. Its light is on meanwhile.", width)
	}
	if c.err != nil || c.frame.Blank() {
		if errors.Is(c.err, tools.ErrCameraBusy) {
			return c.ask.view(width)
		}
		return c.ask.view(width) + "\n\n" + wrap("Measured: "+c.evidence()+".", width)
	}
	// The picture, in whatever height the question leaves: sharp where the terminal draws it
	// itself, and in coloured blocks elsewhere or once the window is narrower than it
	columns, rows := c.space(width, height)
	drawn := c.frame.Render(columns, rows)
	if c.columns > 0 && c.columns <= width {
		drawn = picture.Cells(pictureID, c.columns, c.rows)
	}
	// Seeing themselves is when people wonder where the picture goes, so it says so right under it
	stays := good.Render("🔒 ") + wrap("This picture stays on this computer. It is not sent anywhere, and it is gone when you answer.", width-3)
	return drawn + "\n" + stays + "\n\n" + c.ask.view(width) + "\n\n" +
		wrap("Measured: "+c.evidence()+". Only these numbers are sent.", width)
}
