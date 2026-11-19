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

// ErrCameraBusy says another program holds the camera. Outside PipeWire only one can at a time.
var ErrCameraBusy = errors.New("another program is using the camera")

// Shot is a camera frame as a PPM file's bytes, and how the camera got to it.
type Shot struct {
	PPM []byte
	// Took is how long the camera ran before the frame that was kept.
	Took time.Duration
	// Settled is false when the picture was still getting brighter or darker at the end.
	Settled bool
}

// The camera is given this long to adjust to the light. libcamera's software processing raises a
// sensor's gain a step at a time, and from a cold start in a dim room that was measured at
// eight seconds on a Yoga Slim 7x.
const (
	settleFrames = 450
	settleWindow = time.Second
	settleWithin = 2.0
)

type sample struct {
	at   time.Duration
	mean float64
}

// settled says whether the brightness has stopped changing: over the last second it moved by
// no more than two of 255. The first frames are left out, as the camera's processing has only
// just started on them.
func settled(samples []sample) bool {
	if len(samples) < 4 {
		return false
	}
	samples = samples[2:]
	now := samples[len(samples)-1].at
	if now-samples[0].at < settleWindow {
		return false
	}
	low, high := samples[len(samples)-1].mean, samples[len(samples)-1].mean
	for _, s := range samples {
		if now-s.at <= settleWindow {
			low, high = min(low, s.mean), max(high, s.mean)
		}
	}
	return high-low <= settleWithin
}

// brightness is the mean of a PPM frame's colour values, and whether the frame is whole.
func brightness(ppm []byte) (float64, bool) {
	var width, height, depth, at int
	if n, err := fmt.Sscanf(string(ppm[:min(len(ppm), 40)]), "P6\n%d %d\n%d\n", &width, &height, &depth); n != 3 || err != nil {
		return 0, false
	}
	for newlines := 0; at < len(ppm) && newlines < 3; at++ {
		if ppm[at] == '\n' {
			newlines++
		}
	}
	pixels := ppm[at:]
	if width < 1 || height < 1 || len(pixels) != width*height*3 {
		return 0, false
	}
	step := max(1, len(pixels)/60000)
	sum, count := 0, 0
	for i := 0; i < len(pixels); i += step {
		sum += int(pixels[i])
		count++
	}
	return float64(sum) / float64(count), true
}

// Frame runs the first camera until its picture has settled on a brightness, and returns that
// frame. A fixed handful of frames is too few: the picture starts dark and brightens over
// seconds. cam takes its optional values attached (--capture=450), and a PPM file holds only
// 24-bit pixels, hence the pixel format. Frames are deleted as they come, so a long wait
// doesn't fill the folder.
func Frame(dir string) (Shot, error) {
	var last error
	for _, format := range []string{"BGR888", "RGB888"} {
		shot, out, err := capture(dir, format)
		if len(shot.PPM) > 0 {
			return shot, nil
		}
		if strings.Contains(out, "resource busy") {
			return Shot{}, ErrCameraBusy
		}
		last = fmt.Errorf("cam gave no frame (%v): %s", err, lastLine(out))
	}
	return Shot{}, last
}

func capture(dir, format string) (Shot, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "cam", "-c", "1", "--capture="+strconv.Itoa(settleFrames), "--stream", "pixelformat="+format, "--file="+filepath.Join(dir, "frame-#.ppm"))
	// Interrupted, cam stops the camera and finishes the frame it is writing
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
	command.WaitDelay = 3 * time.Second
	var out strings.Builder
	command.Stdout, command.Stderr = &out, &out
	if err := command.Start(); err != nil {
		return Shot{}, "", err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	frames := func() []string {
		found, _ := filepath.Glob(filepath.Join(dir, "frame-*.ppm"))
		sort.Strings(found)
		return found
	}
	var shot Shot
	var samples []sample
	var seen string
	var started time.Time
	var err error
	for running := true; running; {
		select {
		case err = <-done:
			running = false
		case <-time.After(150 * time.Millisecond):
		}
		found := frames()
		// The newest file may be half written, so the one before it is read
		if running && len(found) > 0 {
			found = found[:len(found)-1]
		}
		for len(found) > 1 {
			os.Remove(found[0])
			found = found[1:]
		}
		if len(found) == 0 || found[0] == seen {
			continue
		}
		data, readErr := os.ReadFile(found[0])
		mean, whole := brightness(data)
		if readErr != nil || !whole {
			continue
		}
		if started.IsZero() {
			started = time.Now()
		}
		seen = found[0]
		shot.PPM, shot.Took = data, time.Since(started)
		samples = append(samples, sample{shot.Took, mean})
		if running && !shot.Settled && settled(samples) {
			shot.Settled = true
			cancel()
		}
	}
	for _, frame := range frames() {
		os.Remove(frame)
	}
	return shot, out.String(), err
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
func Agent(dir, prompt string) error { return agent(dir, prompt, 0) }

// AgentHandedOver opens the agent as Agent does, and returns only once its launcher has
// handed the window over to the desktop, or after a while. A caller that ends right away
// needs this: a service's leftover processes are stopped with it, and a launcher stopped
// halfway opens nothing.
func AgentHandedOver(dir, prompt string) error { return agent(dir, prompt, 30*time.Second) }

func agent(dir, prompt string, wait time.Duration) error {
	// started lets the launcher go on by itself, or waits for it to finish its part
	started := func(command *exec.Cmd) error {
		if err := command.Start(); err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		if wait == 0 {
			return nil
		}
		select {
		case err := <-done:
			return err
		case <-time.After(wait):
			return nil
		}
	}
	// Another way of opening an agent than Omarchy's, for other distros: a command line that
	// gets the first message as its last argument, like HW_TEST_AGENT="foot -e claude"
	if line := strings.Fields(os.Getenv("HW_TEST_AGENT")); len(line) > 0 {
		command := exec.Command(line[0], append(line[1:], prompt)...)
		command.Dir = dir
		command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		return started(command)
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
	return started(command)
}
