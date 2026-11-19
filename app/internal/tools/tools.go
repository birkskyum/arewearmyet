// Package tools runs the programs the guided test needs: PipeWire's player and recorder,
// libcamera's cam, the volume control and the browser. Every one of them is given a time
// limit and killed at the end of it, because players and recorders have been seen to hang
// after their last sample.
package tools

import (
	"context"
	"encoding/json"
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

// ErrNoTerminal says there is no way here to open a terminal window.
var ErrNoTerminal = errors.New("no terminal window can be opened from here")

// Terminal opens a terminal window on the owner's desktop running the given program, the way
// Omarchy opens its own screens, and returns once it is on its way.
func Terminal(program string, args ...string) error { return TerminalIn("", program, args...) }

// TerminalIn is Terminal with the folder the program starts in.
func TerminalIn(dir, program string, args ...string) error {
	var line []string
	switch custom := strings.Fields(os.Getenv("HW_TEST_TERMINAL")); {
	case len(custom) > 0:
		line = custom
	case Have("omarchy-launch-tui"):
		line = []string{"omarchy-launch-tui", "--app-id=org.omarchy.hw-test"}
	case Have("xdg-terminal-exec"):
		line = []string{"xdg-terminal-exec", "-e"}
	default:
		return ErrNoTerminal
	}
	command := exec.Command(line[0], append(append(line[1:], program), args...)...)
	command.Dir = dir
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	go command.Wait()
	return nil
}

// Session is a conversation of the owner's agent, as its own program names it. People use
// different agents, by whichever subscription they have, and each keeps its conversations
// its own way. Going on in the same one keeps everything the agent has learnt so far.
type Session struct {
	// Kind is the program that holds it: t3 (a T3 Code thread) or herdr (a herdr pane), which
	// both run whatever agent the owner has inside them, or codex or claude for those two
	// started plainly in a terminal.
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
}

// holders are the programs that say which conversation this is, and how. The ones that hold
// other agents inside them come first: their thread or pane is what the owner looks at.
var holders = [][2]string{{"T3CODE_THREAD_ID", "t3"}, {"HERDR_PANE_ID", "herdr"}, {"CODEX_THREAD_ID", "codex"}, {"CLAUDE_CODE_SESSION_ID", "claude"}}

// CurrentSessions are the conversations this program was started from, outermost first:
// a Codex running in a T3 Code thread is both, and the thread is tried first.
func CurrentSessions() []Session {
	var held []Session
	for _, known := range holders {
		if id := strings.TrimSpace(os.Getenv(known[0])); id != "" {
			held = append(held, Session{Kind: known[1], ID: id})
		}
	}
	return held
}

// CurrentSession is the outermost of them, or none.
func CurrentSession() Session {
	if held := CurrentSessions(); len(held) > 0 {
		return held[0]
	}
	return Session{}
}

// T3 Code runs whichever agent the owner has, by its own name for it. These are its names for
// the agents Omarchy can set as the default.
var t3Drivers = map[string]string{"codex": "codex", "claude": "claudeAgent", "agy": "antigravity", "opencode": "opencode", "cursor-agent": "cursor", "grok": "grok"}

// t3Model picks the agent and model a new T3 Code thread runs: the owner's default agent in
// Omarchy, on the model T3 Code lists for it first. HW_TEST_T3_MODEL, as provider/model, says
// otherwise. Empty means T3 Code's own default for the project.
func t3Model() (provider, model string) {
	if given := strings.SplitN(os.Getenv("HW_TEST_T3_MODEL"), "/", 2); len(given) == 2 {
		return given[0], given[1]
	}
	chosen, _ := exec.Command("omarchy-default-agent").Output()
	provider = t3Drivers[strings.TrimSpace(string(chosen))]
	if provider == "" {
		return "", ""
	}
	home := os.Getenv("T3CODE_HOME")
	if home == "" {
		user, _ := os.UserHomeDir()
		home = filepath.Join(user, ".t3")
	}
	var listed struct {
		Manifest struct {
			CurrentModels map[string][]string `json:"currentModels"`
			Providers     map[string]struct {
				Defaults map[string]string `json:"defaults"`
			} `json:"providers"`
		} `json:"manifest"`
	}
	if data, err := os.ReadFile(filepath.Join(home, "userdata", "model-manifest.json")); err == nil {
		_ = json.Unmarshal(data, &listed)
	}
	if model = listed.Manifest.Providers[provider].Defaults["chat"]; model == "" && len(listed.Manifest.CurrentModels[provider]) > 0 {
		model = listed.Manifest.CurrentModels[provider][0]
	}
	if model == "" {
		return "", ""
	}
	return provider, model
}

// t3Wait is how long T3 Code is given to come up once it has been started.
var t3Wait = 40 * time.Second

// t3Up makes sure T3 Code's own server answers, starting the app when it is installed but
// not running. Someone who uses T3 Code wants their agent there, not in a terminal, just
// because the app happened to be closed.
func t3Up(dir string) bool {
	answers := func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "t3", "thread", "list", "--project", dir)
		out, err := command.CombinedOutput()
		// A folder that is no project yet is still an answer from a running server
		return err == nil || !strings.Contains(string(out), "No running T3 Code server")
	}
	if answers() {
		return true
	}
	if !Have("gtk-launch") {
		return false
	}
	command := exec.Command("gtk-launch", "t3code.desktop")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if command.Start() != nil {
		return false
	}
	go command.Wait()
	for waited := time.Duration(0); waited < t3Wait; waited += time.Second {
		time.Sleep(time.Second)
		if answers() {
			return true
		}
	}
	return false
}

// T3Show brings T3 Code's window to the front, so the owner sees their agent's thread in its
// list. T3 Code has no way from outside to open one given thread: what it offers, t3 app,
// always opens a new, empty one for the folder, which would hide the thread that is at work.
func T3Show(dir string) {
	if Have("omarchy-hyprland-focus-app") {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "omarchy-hyprland-focus-app", "T3Code").Run()
	}
}

// T3Start starts a thread in T3 Code for the work in a folder and returns its id. T3 Code
// keeps the thread, so the same conversation can be written to again later, whatever agent
// runs in it. It fails where T3 Code isn't installed or its app isn't running.
func T3Start(dir, title, message string) (string, error) {
	if !Have("t3") {
		return "", ErrNoSession
	}
	if !t3Up(dir) {
		return "", fmt.Errorf("T3 Code isn't running and couldn't be started")
	}
	run := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "t3", args...)
		command.Dir = dir
		return command.CombinedOutput()
	}
	// The folder has to be a project of T3 Code's. Adding one that is there already changes nothing
	_, _ = run("project", "add", dir)
	args := []string{"thread", "start", "--json", "--title", title}
	if provider, model := t3Model(); provider != "" {
		args = append(args, "--provider", provider, "--model", model)
	}
	out, err := run(append(args, dir, message)...)
	var started struct {
		Thread string `json:"threadId"`
	}
	if at := strings.LastIndex(string(out), "{"); err == nil && at >= 0 {
		_ = json.Unmarshal(out[at:], &started)
	}
	if err != nil || started.Thread == "" {
		return "", fmt.Errorf("T3 Code didn't start a thread (%v): %s", err, lastLine(string(out)))
	}
	return started.Thread, nil
}

// AgentWindowOpen says whether a window of the owner's agent is open on the desktop.
func AgentWindowOpen() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "hyprctl", "clients", "-j").Output()
	if err != nil {
		return false
	}
	var windows []struct {
		Class string `json:"class"`
	}
	_ = json.Unmarshal(out, &windows)
	for _, window := range windows {
		if strings.HasPrefix(window.Class, "org.omarchy.agent") || window.Class == "org.omarchy.codex" || window.Class == "org.omarchy.claude" {
			return true
		}
	}
	return false
}

// FocusAgentWindow brings the agent's window to the front, where Omarchy has a way to.
func FocusAgentWindow() {
	if Have("omarchy-hyprland-focus-app") {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "omarchy-hyprland-focus-app", "org.omarchy.agent").Run()
	}
}

// ErrAgentOpen says the conversation is open in a window that can't be written to from outside.
var ErrAgentOpen = errors.New("the agent's window is open and can't be written to from here")

// Show takes the owner to a conversation of their agent without saying anything in it: the
// window it is open in, or a new window on the same conversation.
func Show(session Session, dir string) error {
	switch {
	case session.ID == "":
		return ErrNoSession
	case session.Kind == "t3" && Have("t3"):
		if !t3Up(dir) {
			return fmt.Errorf("T3 Code isn't running and couldn't be started")
		}
		T3Show(dir)
		return nil
	case (session.Kind == "codex" || session.Kind == "claude") && AgentWindowOpen():
		FocusAgentWindow()
		return nil
	case session.Kind == "codex" && Have("codex"):
		return TerminalIn(dir, "codex", "--approve-for-me", "resume", session.ID)
	case session.Kind == "claude" && Have("claude"):
		return TerminalIn(dir, "claude", "--permission-mode", "auto", "--resume", session.ID)
	}
	return ErrNoSession
}

// ErrNoSession says there is no earlier session to go on in.
var ErrNoSession = errors.New("there is no earlier agent session to go on in")

// Continue gives a message to an earlier session of the owner's agent, in the way of the
// program that holds it. A T3 Code thread and a herdr pane take it where they are. Codex and
// Claude Code open again in a terminal window, on the same conversation.
func Continue(session Session, dir, message string) error {
	// told runs a program that passes the message on, and says why when it wouldn't
	told := func(name string, args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, args[0], args[1:]...)
		command.Dir = dir
		if out, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("%s didn't take the message (%v): %s", name, err, lastLine(string(out)))
		}
		return nil
	}
	switch {
	case session.ID == "":
		return ErrNoSession
	case session.Kind == "t3" && Have("t3"):
		if !t3Up(dir) {
			return fmt.Errorf("T3 Code isn't running and couldn't be started")
		}
		return told("T3 Code", "t3", "thread", "send", session.ID, message)
	case session.Kind == "herdr" && Have("herdr"):
		return told("herdr", "herdr", "agent", "prompt", session.ID, message)
	case session.Kind == "codex" && Have("codex"):
		// An open Codex takes a message into the conversation it is showing. A closed one
		// opens again on it
		if AgentWindowOpen() {
			if err := told("Codex", "codex", "queue", "--thread", session.ID, "--message", message); err != nil {
				return err
			}
			FocusAgentWindow()
			return nil
		}
		return TerminalIn(dir, "codex", "--approve-for-me", "resume", session.ID, message)
	case session.Kind == "claude" && Have("claude"):
		if AgentWindowOpen() {
			return ErrAgentOpen
		}
		return TerminalIn(dir, "claude", "--permission-mode", "auto", "--resume", session.ID, "--", message)
	}
	return ErrNoSession
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
			// A launcher often hands the rest to a child and is gone at once. The child needs
			// a moment to get its window registered before the caller ends too
			time.Sleep(3 * time.Second)
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
