// Package tools runs the programs the guided test needs: PipeWire's player and recorder,
// libcamera's cam, the volume control and the browser. Every one of them is given a time
// limit and killed at the end of it, because players and recorders have been seen to hang
// after their last sample.
package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Have says whether a program is installed.
func Have(names ...string) bool {
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			return false
		}
	}
	return true
}

// Play plays a sound file and returns when it ends, or after limit.
func Play(file string, limit time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	command := exec.CommandContext(ctx, "pw-play", file)
	command.WaitDelay = time.Second
	err := command.Run()
	if ctx.Err() != nil {
		return nil // it played, and hung on the way out
	}
	return err
}

// Recording is a microphone recording in progress.
type Recording struct {
	File    string
	command *exec.Cmd
	done    chan error
}

// Record starts recording the default microphone to a mono WAV file.
func Record(file string) (*Recording, error) {
	command := exec.Command("pw-record", "--rate", "48000", "--channels", "1", "--format", "s16", file)
	if err := command.Start(); err != nil {
		return nil, err
	}
	recording := &Recording{File: file, command: command, done: make(chan error, 1)}
	go func() { recording.done <- command.Wait() }()
	return recording, nil
}

// Stop ends the recording, asking first and killing after a second, and returns the file's bytes.
func (r *Recording) Stop() []byte {
	_ = r.command.Process.Signal(syscall.SIGINT)
	select {
	case <-r.done:
	case <-time.After(time.Second):
		_ = r.command.Process.Kill()
		// A recorder that can't even be killed is left behind rather than waited for
		select {
		case <-r.done:
		case <-time.After(2 * time.Second):
		}
	}
	data, _ := os.ReadFile(r.File)
	return data
}

// So far returns what has been written to the recording up to now, for a level meter.
func (r *Recording) SoFar() []byte {
	data, _ := os.ReadFile(r.File)
	return data
}

// Frame takes a few frames from the first camera and returns the last one as a PPM file's
// bytes. The first frames are still adjusting their exposure. cam takes its optional values
// attached (--capture=8), and a PPM file holds only 24-bit pixels, hence the pixel format.
func Frame(dir string) ([]byte, error) {
	var last error
	for _, format := range []string{"BGR888", "RGB888"} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		command := exec.CommandContext(ctx, "cam", "-c", "1", "--capture=8", "--stream", "pixelformat="+format, "--file="+filepath.Join(dir, "frame-#.ppm"))
		command.WaitDelay = 3 * time.Second
		out, err := command.CombinedOutput()
		cancel()
		frames, _ := filepath.Glob(filepath.Join(dir, "frame-*.ppm"))
		if len(frames) > 0 {
			sort.Strings(frames)
			data, err := os.ReadFile(frames[len(frames)-1])
			for _, frame := range frames {
				os.Remove(frame)
			}
			return data, err
		}
		last = fmt.Errorf("cam gave no frame (%v): %s", err, lastLine(string(out)))
	}
	return nil, last
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	line := lines[len(lines)-1]
	if len(line) > 160 {
		line = line[:160]
	}
	return line
}

// Volume is the default speakers' volume, 0 to 1 and above, and whether they are muted.
func Volume() (level float64, muted, ok bool) { return volume("@DEFAULT_AUDIO_SINK@") }

// AtHalfVolume sets the speakers to half volume, unmuted, and returns what puts them back as
// they were. A sound test at a known volume gives numbers that can be compared between
// machines and from one kernel to the next. Without a volume control it does nothing.
func AtHalfVolume() (restore func()) {
	level, muted, ok := Volume()
	if !ok {
		return func() {}
	}
	set := func(level float64, muted bool) {
		for _, args := range [][]string{{"set-volume", "@DEFAULT_AUDIO_SINK@", strconv.FormatFloat(level, 'f', 2, 64)}, {"set-mute", "@DEFAULT_AUDIO_SINK@", map[bool]string{true: "1", false: "0"}[muted]}} {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			command := exec.CommandContext(ctx, "wpctl", args...)
			command.WaitDelay = time.Second
			_ = command.Run()
			cancel()
		}
	}
	set(0.5, false)
	return func() { set(level, muted) }
}

// InputVolume is the default microphone's input volume, and whether it is muted.
func InputVolume() (level float64, muted, ok bool) { return volume("@DEFAULT_AUDIO_SOURCE@") }

func volume(device string) (level float64, muted, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "wpctl", "get-volume", device)
	command.WaitDelay = time.Second
	out, err := command.Output()
	if err != nil {
		return 0, false, false
	}
	// "Volume: 0.50" or "Volume: 0.50 [MUTED]"
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, false, false
	}
	level, err = strconv.ParseFloat(fields[1], 64)
	return level, strings.Contains(string(out), "MUTED"), err == nil
}

// Viewer is an image viewer showing a picture in a window of its own.
type Viewer struct {
	command *exec.Cmd
	// Closed is closed when the viewer's window has gone.
	Closed chan struct{}
}

// View opens a picture in imv, Omarchy's image viewer, where q closes it. It returns nil
// when there is no viewer to open it in.
func View(file string) *Viewer {
	if !Have("imv") {
		return nil
	}
	command := exec.Command("imv", file)
	if command.Start() != nil {
		return nil
	}
	viewer := &Viewer{command: command, Closed: make(chan struct{})}
	go func() {
		_ = command.Wait()
		close(viewer.Closed)
	}()
	return viewer
}

// Close shuts the viewer's window if it is still open.
func (v *Viewer) Close() {
	if v == nil {
		return
	}
	select {
	case <-v.Closed:
	default:
		if v.command != nil && v.command.Process != nil {
			_ = v.command.Process.Kill()
		}
	}
}

// Open shows a web page in the default browser, without waiting for it.
func Open(url string) {
	command := exec.Command("xdg-open", url)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if command.Start() == nil {
		go command.Wait()
	}
}

// ErrNoAgent says Omarchy has no agent chosen to launch.
var ErrNoAgent = errors.New("no default agent is chosen")

// ErrNotOmarchy says there is no Omarchy agent launcher on this system.
var ErrNotOmarchy = errors.New("this system has no omarchy-agent")

// Agent opens the owner's chosen coding agent in a window of its own, in the given folder
// and with the given first message, the way Omarchy's own menu does.
func Agent(dir, prompt string) error {
	// Another way of opening an agent than Omarchy's, for other distros: a command line that
	// gets the first message as its last argument, like HW_TEST_AGENT="foot -e claude"
	if line := strings.Fields(os.Getenv("HW_TEST_AGENT")); len(line) > 0 {
		command := exec.Command(line[0], append(line[1:], prompt)...)
		command.Dir = dir
		command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := command.Start(); err != nil {
			return err
		}
		go command.Wait()
		return nil
	}
	if !Have("omarchy-agent", "omarchy-default-agent") {
		return ErrNotOmarchy
	}
	chosen, _ := exec.Command("omarchy-default-agent").Output()
	if strings.TrimSpace(string(chosen)) == "" {
		return ErrNoAgent
	}
	command := exec.Command("omarchy-agent", "--prompt", prompt)
	command.Dir = dir
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	go command.Wait()
	return nil
}
