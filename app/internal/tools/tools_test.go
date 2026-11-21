package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ppm is a whole frame of one grey.
func ppm(grey byte) []byte {
	pixels := make([]byte, 64*36*3)
	for i := range pixels {
		pixels[i] = grey
	}
	return append([]byte("P6\n64 36\n255\n"), pixels...)
}

func TestBrightnessReadsAWholeFrameAndRefusesATornOne(t *testing.T) {
	if mean, whole := brightness(ppm(77)); !whole || mean != 77 {
		t.Errorf("a whole frame: %v, %v", mean, whole)
	}
	frame := ppm(77)
	for _, torn := range [][]byte{frame[:len(frame)-10], frame[:20], nil, []byte("P6\n")} {
		if _, whole := brightness(torn); whole {
			t.Errorf("a frame of %d bytes counted as whole", len(torn))
		}
	}
}

func TestThePictureHasSettledOnlyWhenItStoppedChangingForASecond(t *testing.T) {
	series := func(means ...float64) []sample {
		var samples []sample
		for i, mean := range means {
			samples = append(samples, sample{time.Duration(i) * 150 * time.Millisecond, mean})
		}
		return samples
	}
	// As measured on a Yoga Slim 7x from a cold start, about five of 255 a second, for seconds
	var ramp []float64
	for i := 0; i < 40; i++ {
		ramp = append(ramp, 17+float64(i)*0.75)
	}
	for i := 4; i <= len(ramp); i++ {
		if settled(series(ramp[:i]...)) {
			t.Fatalf("a picture still brightening counted as settled after %d samples", i)
		}
	}
	// Dark first frames, then steady, and settled once a second of steady frames is in
	steady := []float64{33, 71, 77, 77, 77, 76, 77, 77, 77, 77, 77}
	if settled(series(steady[:8]...)) {
		t.Error("settled before a second of steady frames")
	}
	if !settled(series(steady...)) {
		t.Error("a steady picture never settled")
	}
	// The ramp's end, steady after brightening
	if !settled(series(append(ramp, 47, 47, 47, 46, 47, 47, 47, 47, 47)...)) {
		t.Error("a picture that stopped brightening never settled")
	}
}

// fakeCam puts a cam on the PATH that writes a frame every 50 ms as its script says, and stops
// when interrupted.
func fakeCam(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	body := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in --file=*) pattern=${a#--file=};; esac; done\n" + script
	if err := os.WriteFile(filepath.Join(bin, "cam"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

func TestFrameWaitsForTheCameraToSettleAndStopsItThen(t *testing.T) {
	dir := t.TempDir()
	for grey := 0; grey < 256; grey++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("grey-%d", grey)), ppm(byte(grey)), 0o600)
	}
	// Brightens by two a frame for forty frames, then holds at 100
	fakeCam(t, `trap 'exit 0' INT
i=0
while [ $i -lt 2000 ]; do
  grey=$((20 + i * 2)); [ $grey -gt 100 ] && grey=100
  cp "`+dir+`/grey-$grey" "$(printf '%s' "$pattern" | sed "s/#/cam0-stream0-$(printf %06d $i)/")"
  i=$((i + 1)); sleep 0.05
done
`)
	out := t.TempDir()
	began := time.Now()
	shot, err := Frame(out)
	if err != nil {
		t.Fatal(err)
	}
	mean, whole := brightness(shot.PPM)
	if !whole || mean != 100 || !shot.Settled {
		t.Errorf("brightness %v, whole %v, settled %v", mean, whole, shot.Settled)
	}
	// Forty frames of brightening are two seconds, and a second of steady ones follows
	if took := time.Since(began); took < 2500*time.Millisecond || took > 8*time.Second {
		t.Errorf("it took %v", took)
	}
	if left, _ := filepath.Glob(filepath.Join(out, "*")); len(left) != 0 {
		t.Errorf("frames were left behind: %v", left)
	}
}

func TestFrameSaysWhenAnotherProgramHoldsTheCamera(t *testing.T) {
	fakeCam(t, "echo \"ERROR MediaDevice /dev/media0[qcom-camss]: Failed to setup link 'msm_csiphy4'[1] -> 'msm_csid0'[0]: Device or resource busy\" >&2\necho 'Camera 1 not found' >&2\nexit 1\n")
	if _, err := Frame(t.TempDir()); !errors.Is(err, ErrCameraBusy) {
		t.Errorf("a busy camera gave %v", err)
	}
	fakeCam(t, "echo 'Camera 1 not found' >&2\nexit 1\n")
	if _, err := Frame(t.TempDir()); err == nil || errors.Is(err, ErrCameraBusy) {
		t.Errorf("a missing camera gave %v", err)
	}
}

func TestTheAgentsLauncherIsWaitedForWhereTheCallerEndsAtOnce(t *testing.T) {
	dir := t.TempDir()
	launcher := filepath.Join(dir, "launcher")
	os.WriteFile(launcher, []byte("#!/bin/sh\nsleep 1\necho \"$1\" > "+dir+"/opened\n"), 0o755)
	t.Setenv("HW_TEST_AGENT", launcher)
	opened := func() bool { _, err := os.Stat(filepath.Join(dir, "opened")); return err == nil }
	// From the test's own screen nothing is waited for
	if err := Agent(dir, "read the brief"); err != nil || opened() {
		t.Fatalf("Agent: %v, opened at once %v", err, opened())
	}
	time.Sleep(1500 * time.Millisecond)
	os.Remove(filepath.Join(dir, "opened"))
	// At login the launcher has done its part by the time this returns
	if err := AgentHandedOver(dir, "read the brief"); err != nil || !opened() {
		t.Errorf("AgentHandedOver: %v, opened %v", err, opened())
	}
	// And a launcher that fails is reported
	os.WriteFile(launcher, []byte("#!/bin/sh\nexit 3\n"), 0o755)
	if err := AgentHandedOver(dir, "read the brief"); err == nil {
		t.Error("a launcher that failed wasn't reported")
	}
}

func TestTheSameConversationIsGoneOnInWhateverAgentHoldsIt(t *testing.T) {
	for _, name := range []string{"T3CODE_THREAD_ID", "HERDR_PANE_ID", "CODEX_THREAD_ID", "CLAUDE_CODE_SESSION_ID"} {
		t.Setenv(name, "")
	}
	if got := CurrentSession(); got != (Session{}) {
		t.Fatalf("a session where there is none: %+v", got)
	}
	// Each agent says which conversation it is in its own way. T3 Code runs the others inside
	// its threads, so its thread is the one to go on in
	t.Setenv("CLAUDE_CODE_SESSION_ID", "c0ffee")
	if got := CurrentSession(); got != (Session{Kind: "claude", ID: "c0ffee"}) {
		t.Errorf("in Claude Code: %+v", got)
	}
	t.Setenv("CODEX_THREAD_ID", "01a1f404")
	if got := CurrentSession(); got != (Session{Kind: "codex", ID: "01a1f404"}) {
		t.Errorf("in Codex: %+v", got)
	}
	t.Setenv("HERDR_PANE_ID", "pane-3")
	if got := CurrentSession(); got != (Session{Kind: "herdr", ID: "pane-3"}) {
		t.Errorf("in herdr: %+v", got)
	}
	t.Setenv("T3CODE_THREAD_ID", "thread-7")
	if got := CurrentSession(); got != (Session{Kind: "t3", ID: "thread-7"}) {
		t.Errorf("in T3 Code: %+v", got)
	}
	// All of them are kept, outermost first, so that the next can be tried when one is gone
	if got := fmt.Sprint(CurrentSessions()); got != "[{t3 thread-7} {herdr pane-3} {codex 01a1f404} {claude c0ffee}]" {
		t.Errorf("every holder: %s", got)
	}

	// Stand-ins for the three programs and for the terminal window, which write down how they were called
	dir, bin := t.TempDir(), t.TempDir()
	for _, program := range []string{"t3", "herdr", "codex", "claude", "window"} {
		os.WriteFile(filepath.Join(bin, program), []byte("#!/bin/sh\nprintf '%s|' \"$(basename $0)\" \"$@\" > "+dir+"/called\npwd >> "+dir+"/called\n"), 0o755)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("HW_TEST_TERMINAL", filepath.Join(bin, "window"))
	called := func() string {
		for waited := 0; waited < 40; waited++ {
			if data, err := os.ReadFile(filepath.Join(dir, "called")); err == nil && len(data) > 0 {
				os.Remove(filepath.Join(dir, "called"))
				return string(data)
			}
			time.Sleep(50 * time.Millisecond)
		}
		return ""
	}
	work := t.TempDir()
	for session, want := range map[Session]string{
		// A T3 Code thread takes the message where it is
		{Kind: "t3", ID: "thread-7"}: "t3|thread|send|thread-7|go on|",
		// So does a herdr pane, whatever agent runs in it
		{Kind: "herdr", ID: "pane-3"}: "herdr|agent|prompt|pane-3|go on|",
		// Codex and Claude Code open again in a window, on the same conversation
		{Kind: "codex", ID: "01a1f404"}: "window|codex|--approve-for-me|resume|01a1f404|go on|",
		{Kind: "claude", ID: "c0ffee"}:  "window|claude|--permission-mode|auto|--resume|c0ffee|--|go on|",
	} {
		if err := Continue(session, work, "go on"); err != nil {
			t.Errorf("%+v: %v", session, err)
		}
		if got := called(); !strings.HasPrefix(got, want) || !strings.Contains(got, filepath.Base(work)) {
			t.Errorf("%+v was gone on in with %q, want %q in %s", session, got, want, work)
		}
	}
	// An agent that names no conversation, or one this computer no longer has, has none to go on in
	for _, session := range []Session{{}, {Kind: "codex"}, {Kind: "gemini", ID: "x"}} {
		if err := Continue(session, work, "go on"); !errors.Is(err, ErrNoSession) {
			t.Errorf("%+v: %v", session, err)
		}
	}
	// T3 Code that doesn't answer is an error, so the caller can open a new conversation
	os.WriteFile(filepath.Join(bin, "t3"), []byte("#!/bin/sh\necho 'no server is running' >&2\nexit 1\n"), 0o755)
	if err := Continue(Session{Kind: "t3", ID: "thread-7"}, work, "go on"); err == nil || errors.Is(err, ErrNoSession) || !strings.Contains(err.Error(), "no server is running") {
		t.Errorf("T3 Code not answering: %v", err)
	}
}

func TestAThreadIsStartedInT3CodeForTheOwnersOwnAgent(t *testing.T) {
	dir, bin, home := t.TempDir(), t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(home, "userdata"), 0o755)
	os.WriteFile(filepath.Join(home, "userdata", "model-manifest.json"), []byte(`{"manifest":{"currentModels":{"codex":["gpt-6-astra","gpt-6-luna"],"claudeAgent":["claude-fable-5-2"]},"providers":{"codex":{},"claudeAgent":{"defaults":{"chat":"claude-opus-5-5"}}}}}`), 0o644)
	t.Setenv("T3CODE_HOME", home)
	t.Setenv("HW_TEST_T3_MODEL", "")
	agent := func(name string) {
		os.WriteFile(filepath.Join(bin, "omarchy-default-agent"), []byte("#!/bin/sh\necho "+name+"\n"), 0o755)
	}
	os.WriteFile(filepath.Join(bin, "t3"), []byte("#!/bin/sh\nprintf '%s|' \"$@\" >> "+dir+"/called\necho >> "+dir+"/called\n[ \"$1\" = project ] && { echo 'Added project'; exit 0; }\necho 'some notice'\necho '{\"threadId\":\"mcp:351f\",\"status\":\"running\"}'\n"), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	work := t.TempDir()
	for name, want := range map[string]string{
		// The owner's default agent in Omarchy, by T3 Code's name for it, on the model it lists for it
		"codex":  "--provider|codex|--model|gpt-6-astra|",
		"claude": "--provider|claudeAgent|--model|claude-opus-5-5|",
		// An agent T3 Code doesn't run is left to T3 Code's own default
		"pi": "thread|start|--json|--title|Hardware: speakers|" + work + "|read the brief|",
	} {
		agent(name)
		os.Remove(filepath.Join(dir, "called"))
		id, err := T3Start(work, "Hardware: speakers", "read the brief")
		calls, _ := os.ReadFile(filepath.Join(dir, "called"))
		if err != nil || id != "mcp:351f" || !strings.Contains(string(calls), "project|add|"+work+"|") || !strings.Contains(string(calls), want) || !strings.Contains(string(calls), "|"+work+"|read the brief|") {
			t.Errorf("default agent %s: id %q, %v, called\n%s", name, id, err, calls)
		}
	}
	// HW_TEST_T3_MODEL says otherwise
	t.Setenv("HW_TEST_T3_MODEL", "antigravity/gemini-4-carbon")
	os.Remove(filepath.Join(dir, "called"))
	T3Start(work, "Hardware: speakers", "read the brief")
	if calls, _ := os.ReadFile(filepath.Join(dir, "called")); !strings.Contains(string(calls), "--provider|antigravity|--model|gemini-4-carbon|") {
		t.Errorf("a chosen model: %s", calls)
	}
	// T3 Code that isn't running starts nothing, and says why
	os.WriteFile(filepath.Join(bin, "t3"), []byte("#!/bin/sh\necho 'T3 Code is not running' >&2\nexit 1\n"), 0o755)
	if id, err := T3Start(work, "Hardware: speakers", "read the brief"); err == nil || id != "" || !strings.Contains(err.Error(), "not running") {
		t.Errorf("T3 Code not running: %q, %v", id, err)
	}
}

func TestT3CodeIsStartedWhenItIsInstalledButNotRunning(t *testing.T) {
	dir, bin := t.TempDir(), t.TempDir()
	t.Setenv("HW_TEST_T3_MODEL", "codex/gpt-6-astra")
	// T3 Code's command says no server is running until the app has been started
	os.WriteFile(filepath.Join(bin, "t3"), []byte("#!/bin/sh\nif [ ! -e "+dir+"/running ]; then echo 'No running T3 Code server uses this folder. Start T3 Code, then retry.' >&2; exit 1; fi\nprintf '%s|' \"$@\" >> "+dir+"/called\necho >> "+dir+"/called\n[ \"$1\" = thread ] && [ \"$2\" = start ] && echo '{\"threadId\":\"mcp:77\"}'\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(bin, "gtk-launch"), []byte("#!/bin/sh\necho \"$1\" > "+dir+"/launched\n(sleep 1; touch "+dir+"/running) &\n"), 0o755)
	// The app is started in a group of its own, so it outlives whoever started it
	os.WriteFile(filepath.Join(bin, "systemd-run"), []byte("#!/bin/sh\necho \"$*\" > "+dir+"/scoped\nwhile [ \"$1\" != -- ]; do shift; done\nshift\nexec \"$@\"\n"), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	work := t.TempDir()
	id, err := T3Start(work, "Hardware: speakers", "read the brief")
	launched, _ := os.ReadFile(filepath.Join(dir, "launched"))
	if err != nil || id != "mcp:77" || strings.TrimSpace(string(launched)) != "t3code.desktop" {
		t.Fatalf("id %q, %v, launched %q", id, err, launched)
	}
	if scoped, _ := os.ReadFile(filepath.Join(dir, "scoped")); strings.TrimSpace(string(scoped)) != "--user --scope --collect --quiet -- gtk-launch t3code.desktop" {
		t.Errorf("the app wasn't started in a group of its own: %q", scoped)
	}
	// Where no such group can be made, it is started as before
	os.Remove(filepath.Join(dir, "launched"))
	os.Remove(filepath.Join(dir, "running"))
	os.WriteFile(filepath.Join(bin, "systemd-run"), []byte("#!/bin/sh\necho 'Failed to connect to user scope bus' >&2\nexit 1\n"), 0o755)
	if id, err = T3Start(work, "Hardware: speakers", "read the brief"); err != nil || id != "mcp:77" {
		t.Fatalf("without a group of its own: id %q, %v", id, err)
	}
	if launched, _ = os.ReadFile(filepath.Join(dir, "launched")); strings.TrimSpace(string(launched)) != "t3code.desktop" {
		t.Errorf("not started without a group of its own: %q", launched)
	}
	// Running already, it is only written to
	os.Remove(filepath.Join(dir, "launched"))
	if err := Continue(Session{Kind: "t3", ID: "mcp:77"}, work, "go on"); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "launched")); err == nil {
		t.Error("the app was started again while it was running")
	}
	// Where it can't be brought up, the caller is told, and opens the agent another way
	os.Remove(filepath.Join(dir, "running"))
	os.WriteFile(filepath.Join(bin, "gtk-launch"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t3Wait = 2 * time.Second
	defer func() { t3Wait = 40 * time.Second }()
	if _, err := T3Start(work, "Hardware: speakers", "read the brief"); err == nil || !strings.Contains(err.Error(), "couldn't be started") {
		t.Errorf("T3 Code that never comes up: %v", err)
	}
}

func TestAnOpenCodexTakesTheMessageAndAClosedOneOpensAgain(t *testing.T) {
	dir, bin := t.TempDir(), t.TempDir()
	for _, program := range []string{"codex", "claude", "window", "omarchy-hyprland-focus-app"} {
		os.WriteFile(filepath.Join(bin, program), []byte("#!/bin/sh\nprintf '%s|' \"$(basename $0)\" \"$@\" >> "+dir+"/called\necho >> "+dir+"/called\n"), 0o755)
	}
	windows := func(classes ...string) {
		var list []string
		for _, class := range classes {
			list = append(list, `{"class":"`+class+`"}`)
		}
		os.WriteFile(filepath.Join(bin, "hyprctl"), []byte("#!/bin/sh\necho '["+strings.Join(list, ",")+"]'\n"), 0o755)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("HW_TEST_TERMINAL", filepath.Join(bin, "window"))
	called := func() string {
		time.Sleep(300 * time.Millisecond)
		data, _ := os.ReadFile(filepath.Join(dir, "called"))
		os.Remove(filepath.Join(dir, "called"))
		return string(data)
	}
	work := t.TempDir()
	codex, claude := Session{Kind: "codex", ID: "01a1f404"}, Session{Kind: "claude", ID: "c0ffee"}

	// The agent's window is open, so Codex takes the message into the conversation it shows, and the
	// window comes forward
	windows("com.mitchellh.ghostty", "org.omarchy.agent")
	if !AgentWindowOpen() {
		t.Fatal("the agent's window wasn't seen")
	}
	if err := Continue(codex, work, "go on"); err != nil {
		t.Error(err)
	}
	if got := called(); !strings.Contains(got, "codex|queue|--thread|01a1f404|--message|go on|") || !strings.Contains(got, "omarchy-hyprland-focus-app|org.omarchy.agent|") || strings.Contains(got, "resume") {
		t.Errorf("an open Codex: %q", got)
	}
	// Claude Code can't be written to while it is open, and no second window is opened on it
	if err := Continue(claude, work, "go on"); !errors.Is(err, ErrAgentOpen) || called() != "" {
		t.Errorf("an open Claude Code: %v", err)
	}
	// Going to it only brings its window forward
	if err := Show(codex, work); err != nil || !strings.HasPrefix(called(), "omarchy-hyprland-focus-app|org.omarchy.agent|") {
		t.Errorf("going to an open agent: %v", err)
	}

	// No agent window, so the same conversation opens again, with the message or without
	windows("com.mitchellh.ghostty")
	if AgentWindowOpen() {
		t.Fatal("an agent window was seen where there is none")
	}
	Continue(codex, work, "go on")
	if got := called(); !strings.HasPrefix(got, "window|codex|--approve-for-me|resume|01a1f404|go on|") {
		t.Errorf("a closed Codex: %q", got)
	}
	Show(claude, work)
	if got := called(); !strings.HasPrefix(got, "window|claude|--permission-mode|auto|--resume|c0ffee|") || strings.Contains(got, "go on") {
		t.Errorf("going to a closed Claude Code: %q", got)
	}
}

func TestTheMessageIsTypedIntoTheThreadT3CodeOpensInFront(t *testing.T) {
	dir, bin := t.TempDir(), t.TempDir()
	for _, name := range []string{"SSH_CONNECTION", "SSH_TTY"} {
		t.Setenv(name, "")
	}
	script := func(name, body string) {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755)
	}
	// T3 Code, with one thread there already, and a new one once a message has been typed and sent
	script("t3", `printf '%s|' "$@" >> `+dir+`/called; echo >> `+dir+`/called
[ "$1" = app ] && touch `+dir+`/opened
if [ "$1 $2" = "thread list" ]; then
  if [ -e `+dir+`/sent ]; then echo '[{"threadId":"8411","title":"Speaker balance"},{"threadId":"mcp:old","title":"Earlier"}]'
  else echo '[{"threadId":"mcp:old","title":"Earlier"}]'; fi
fi
exit 0
`)
	script("wtype", `if [ "$1" = -k ]; then [ -e `+dir+`/text ] && touch `+dir+`/sent; else printf '%s' "$1" > `+dir+`/text; fi
`)
	script("omarchy-hyprland-focus-app", "echo \"$1\" > "+dir+"/focused\n")
	front := func(class string) { script("hyprctl", `echo '{"class":"`+class+`"}'`+"\n") }
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t3Settle, t3Appear = 50*time.Millisecond, 3*time.Second
	defer func() { t3Settle, t3Appear = 1500*time.Millisecond, 20*time.Second }()
	work := t.TempDir()

	// T3 Code's window has the keyboard, so the message is typed as one line and sent, and the thread
	// that wasn't there before is the one
	front("com.t3tools.T3Code")
	id, title, err := T3Type(work, "Read BRIEF.md in this folder\nand help me")
	text, _ := os.ReadFile(filepath.Join(dir, "text"))
	focused, _ := os.ReadFile(filepath.Join(dir, "focused"))
	if err != nil || id != "8411" || title != "Speaker balance" || string(text) != "Read BRIEF.md in this folder and help me" {
		t.Errorf("typed into the open thread: id %q, title %q, %v, text %q", id, title, err, text)
	}
	// Its window had the keyboard already, so it wasn't asked forward
	if len(focused) != 0 {
		t.Errorf("the window was asked forward though it was in front: %q", focused)
	}
	if _, opened := os.Stat(filepath.Join(dir, "opened")); opened != nil {
		t.Error("T3 Code wasn't asked to open the folder")
	}
	// Another window has the keyboard, so nothing is typed into it
	os.Remove(filepath.Join(dir, "text"))
	os.Remove(filepath.Join(dir, "sent"))
	front("com.mitchellh.ghostty")
	if _, _, err := T3Type(work, "Read BRIEF.md"); err == nil || !strings.Contains(err.Error(), "didn't come to the front") {
		t.Errorf("another window in front: %v", err)
	}
	if _, typed := os.Stat(filepath.Join(dir, "text")); typed == nil {
		t.Error("the message was typed into another window")
	}
	if asked, _ := os.ReadFile(filepath.Join(dir, "focused")); strings.TrimSpace(string(asked)) != "T3Code" {
		t.Errorf("T3 Code's window wasn't asked forward: %q", asked)
	}
	// Over ssh there is no desktop to type on
	front("com.t3tools.T3Code")
	t.Setenv("SSH_CONNECTION", "100.1.2.3 5555 100.4.5.6 22")
	if _, _, err := T3Type(work, "Read BRIEF.md"); !errors.Is(err, ErrNoSession) {
		t.Errorf("over ssh: %v", err)
	}
}

func TestAWindowForTheOwnerDoesNotInheritAnAgentsBareSettings(t *testing.T) {
	// As measured in a Codex thread, with colours off and paging off, in a terminal that has colours
	agent := []string{"HOME=/home/owner", "NO_COLOR=1", "PAGER=cat", "GIT_PAGER=cat", "TERM=xterm-ghostty", "COLORTERM=truecolor", "CLICOLOR=0", "FORCE_COLOR=0", "PATH=/usr/bin"}
	if got := strings.Join(OwnersEnv(agent), " "); got != "HOME=/home/owner TERM=xterm-ghostty COLORTERM=truecolor PATH=/usr/bin" {
		t.Errorf("an agent's environment became %q", got)
	}
	// What the owner set themselves stays
	own := []string{"PAGER=less", "TERM=xterm-256color", "FORCE_COLOR=1", "CLICOLOR=1"}
	if got := strings.Join(OwnersEnv(append(own, "TERM=dumb")), " "); got != strings.Join(own, " ") {
		t.Errorf("the owner's own settings became %q", got)
	}
	// The window is started with the cleaned settings
	dir, bin := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(bin, "window"), []byte("#!/bin/sh\necho \"colour:${NO_COLOR:-on} pager:${PAGER:-unset}\" > "+dir+"/seen\n"), 0o755)
	t.Setenv("HW_TEST_TERMINAL", filepath.Join(bin, "window"))
	t.Setenv("NO_COLOR", "1")
	t.Setenv("PAGER", "cat")
	if err := Terminal("true"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if seen, _ := os.ReadFile(filepath.Join(dir, "seen")); strings.TrimSpace(string(seen)) != "colour:on pager:unset" {
		t.Errorf("the window saw %q", seen)
	}
}

// The sound server's list of cards says whether a built-in card offers a headphone output. The
// first card is the Yoga Slim 7x's own, which has only USB-C ports.
func TestAHeadphoneOutputIsReadFromTheBuiltInCards(t *testing.T) {
	yoga := `Card #77
	Name: alsa_card.platform-sound
	Properties:
		device.form_factor = "internal"
	Ports:
		[Out] Speaker: Speaker playback (type: Speaker, priority: 100, latency offset: 0 usec, availability unknown)
			Part of profile(s): HiFi
		[In] Mic: Internal microphones (type: Mic, priority: 100, latency offset: 0 usec, availability unknown)
`
	if headphonePort(yoga) {
		t.Error("a headphone output was found on a card with only a speaker")
	}
	jack := strings.Replace(yoga, "		[In] Mic", "		[Out] Headphones: Headphones playback (type: Headphones, priority: 200, latency offset: 0 usec, not available)\n		[In] Mic", 1)
	if !headphonePort(jack) {
		t.Error("the headphone output of a built-in card wasn't found")
	}
	// A USB-C adapter or a Bluetooth headset is a sound card of its own, not the computer's socket
	adapter := yoga + `Card #91
	Name: alsa_card.usb-Generic_USB-C_Audio_Adapter-00
	Ports:
		[Out] analog-output-headphones: Headphones (type: Headphones, priority: 9900, latency offset: 0 usec, available)
Card #95
	Name: bluez_card.AA_BB_CC_DD_EE_FF
	Ports:
		[Out] headset-output: Headset (type: Headset, priority: 0, latency offset: 0 usec, available)
`
	if headphonePort(adapter) {
		t.Error("a USB adapter or a Bluetooth headset counted as the computer's own headphone output")
	}
}

func TestAWindowThatNeverComesUpIsNoWindow(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("HW_TEST_TERMINAL", "")
	WindowWait = 400 * time.Millisecond
	defer func() { WindowWait = 10 * time.Second }()
	ran := filepath.Join(bin, "ran")
	launcher := func(body string) {
		if err := os.WriteFile(filepath.Join(bin, "omarchy-launch-tui"), []byte("#!/bin/sh\nshift\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Over ssh, or before anyone has logged in, the launcher starts and no window follows
	launcher("exit 0")
	if err := Terminal("sh", "-c", ": > "+ran); err != ErrNoTerminal {
		t.Errorf("a launcher that opens nothing: %v", err)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("the program ran without a window")
	}

	// With a desktop the program runs in the window, and that is what counts as open
	launcher(`exec "$@"`)
	if err := Terminal("sh", "-c", ": > "+ran); err != nil {
		t.Errorf("a launcher that opens the window: %v", err)
	}
	for waited := 0; waited < 40; waited++ {
		if _, err := os.Stat(ran); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Error("the program didn't run in the window")
}
