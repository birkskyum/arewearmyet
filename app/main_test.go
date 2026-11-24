package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/term"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

func TestBeforeALogGoesTheOwnerIsToldWhatGoesAndCanSayNo(t *testing.T) {
	said := attached([]report.Log{{Name: "previous.log", Text: "one\ntwo\nthree"}, {Name: "kernel.log", Text: "one"}})
	if said != "Attaching previous.log (3 lines), kernel.log (1 line), with addresses, serial numbers, disk IDs and names taken out.\nIt goes on the model's page, for others with the same computer. To read it first, run this again with --json.\n" {
		t.Errorf("the notice:\n%s", said)
	}
	// The log itself is never printed unasked
	for _, part := range []string{"one\ntwo"} {
		if strings.Contains(said, part) {
			t.Errorf("the notice shows the log:\n%s", said)
		}
	}
	for _, part := range []string{"--json"} {
		if !strings.Contains(said, part) {
			t.Errorf("the notice lacks %q:\n%s", part, said)
		}
	}
	for answer, want := range map[string]bool{"\n": true, "y\n": true, "Yes\n": true, "n\n": false, " NO \n": false, "": false} {
		var asked strings.Builder
		if got := agreed(strings.NewReader(answer), &asked); got != want {
			t.Errorf("answer %q: sent is %v, want %v", answer, got, want)
		}
		if asked.String() != "Send it? [Y/n] " {
			t.Errorf("asked %q", asked.String())
		}
	}
}

func TestANoteIsAskedForAtTheTerminalSoNothingHasToBeTypedOut(t *testing.T) {
	checks := map[string]report.Check{"wifi": {ID: "wifi", Title: "Wi-Fi"}, "suspend": {ID: "suspend", Title: "Sleep and wake"}, "camera": {ID: "camera", Title: "Webcam"}, "lid": {ID: "lid", Title: "Lid switch"}}
	typed := func(lines ...string) reader {
		return plain(bufio.NewReader(strings.NewReader(strings.Join(lines, "\n") + "\n")))
	}

	// hw-test note asks for the part, what happened, whether a log goes, and whether it was before
	// the last restart
	var asked strings.Builder
	args, written := askNote(typed("sleep", "Slept about five minutes. Wi-Fi was gone on wake and needed a restart.", "", "y"), &asked, checks, noteArgs{kind: "problem"}, "")
	if !written || args.check != "suspend" || args.title != "Slept about five minutes. Wi-Fi was gone on wake and needed a restart" || strings.Join(args.logs, ",") != "previous" || !strings.HasPrefix(args.body, "Slept about five") {
		t.Errorf("asked for everything: %+v", args)
	}
	for _, question := range []string{"Which part is it about?", "What happened?", "Should the kernel's log go with it?", "restarted since it happened? [y/N]"} {
		if !strings.Contains(asked.String(), question) {
			t.Errorf("it didn't ask %q:\n%s", question, asked.String())
		}
	}
	// hw-test note wifi gives the part, and Enter on the last question means this start's log
	asked.Reset()
	args, written = askNote(typed("It restarts by itself a few minutes after joining the network", ""), &asked, checks, noteArgs{kind: "problem"}, "wifi")
	if !written || args.check != "wifi" || strings.Join(args.logs, ",") != "kernel" || strings.Contains(asked.String(), "Which part") || args.title != "It restarts by itself a few minutes after joining the network" {
		t.Errorf("with the part given: %+v\n%s", args, asked.String())
	}
	// A tip goes without a log when the owner says so, and nothing is asked of restarts
	asked.Reset()
	args, written = askNote(typed("Camera works in Monologue once the privacy shutter is slid to the right.", "n"), &asked, checks, noteArgs{kind: "problem"}, "camera")
	if !written || args.check != "camera" || len(args.logs) != 0 || strings.Contains(asked.String(), "restarted") {
		t.Errorf("a note without a log: %+v\n%s", args, asked.String())
	}
	// A word that names no part gets the list, and a note that fits none has no part
	asked.Reset()
	args, _ = askNote(typed("flux", "", "The clock is an hour off after every restart of the laptop.", "", "n"), &asked, checks, noteArgs{kind: "note"}, "")
	if args.check != "" || !strings.Contains(asked.String(), `No part is called "flux" here. The parts are camera, lid, suspend, wifi.`) || args.logs[0] != "kernel" {
		t.Errorf("an unknown part: %+v\n%s", args, asked.String())
	}
	// Too little is asked for again, and nothing at all writes nothing
	asked.Reset()
	args, written = askNote(typed("camera", "dark", "The picture stays dark for the first seconds in every app."), &asked, checks, noteArgs{}, "")
	if !written || !strings.Contains(asked.String(), "A little more, please") || args.check != "camera" {
		t.Errorf("too little: %+v", args)
	}
	if _, written = askNote(typed("camera", ""), &asked, checks, noteArgs{}, ""); written {
		t.Error("an empty note was written")
	}
	// What was given on the command line isn't asked for again
	asked.Reset()
	args, _ = askNote(typed("It froze when the dock was plugged in during sleep."), &asked, checks, noteArgs{check: "suspend", title: "Freezes with a dock", logs: []string{"boot-3"}}, "")
	if asked.String() != "What happened? A sentence or two, then Enter.\n> " || args.title != "Freezes with a dock" || args.logs[0] != "boot-3" {
		t.Errorf("given options were asked for again:\n%s", asked.String())
	}

	// At the owner's terminal the line can be edited as it is typed, with the arrow keys, Home and
	// End, and Ctrl+C stops without writing anything
	left, home := "\x1b[D", "\x1b[H"
	keys := "wifi\r" + "Drops after sleep wrks again" + strings.Repeat(left, 9) + "o" + home + "Wi-Fi " + "\r" + "n\r"
	ready, restored := 0, 0
	terminal := func(keys string) reader {
		return editable(strings.NewReader(keys), func(*term.Terminal) (func(), error) {
			ready++
			return func() { restored++ }, nil
		})
	}
	asked.Reset()
	args, written = askNote(terminal(keys), &asked, checks, noteArgs{kind: "problem"}, "")
	if !written || args.check != "wifi" || args.body != "Wi-Fi Drops after sleep works again" || len(args.logs) != 0 || ready != 3 || restored != 3 {
		t.Errorf("edited at the terminal, %d ready and %d restored: %+v\n%q", ready, restored, args, asked.String())
	}
	if _, written = askNote(terminal("wifi\r"+"It drops off after sleep, every time"+"\x03"), &asked, checks, noteArgs{kind: "problem"}, ""); written {
		t.Error("Ctrl+C wrote the note")
	}
	// A terminal that can't be made ready is read a line at a time
	broken := editable(strings.NewReader("Stays dark for the first seconds\n"), func(*term.Terminal) (func(), error) { return nil, errors.New("not a terminal") })
	if answer, ok := broken(io.Discard); !ok || strings.TrimSpace(answer) != "Stays dark for the first seconds" {
		t.Errorf("read plainly: %q, %v", answer, ok)
	}

	// Everyday words, ids and a title's own word all find the part
	for word, want := range map[string]string{"Sleep": "suspend", "wi-fi": "wifi", "WIFI": "wifi", "webcam": "camera", "switch": "lid", "suspend": "suspend"} {
		if id, known := part(word, checks); !known || id != want {
			t.Errorf("%q means %q, want %q", word, id, want)
		}
	}
	if _, known := part("and", map[string]report.Check{"a": {Title: "Sleep and wake"}, "b": {Title: "Audio and battery"}}); known {
		t.Error("a word two parts share named one of them")
	}
	// A long first sentence is cut at a word
	long := headline("The laptop goes to sleep fine but when it wakes up again after more than about thirty minutes the screen stays black and nothing helps")
	if len(long) > 100 || strings.HasSuffix(long, " ") || !strings.HasPrefix(long, "The laptop goes to sleep fine") {
		t.Errorf("headline: %q", long)
	}
	// And a long text at the end of a sentence
	if got := headline("It freezes when woken with a dock plugged in. Only a long press on the power button helps then, and the log shows nothing at all."); got != "It freezes when woken with a dock plugged in" {
		t.Errorf("headline: %q", got)
	}
}

func TestOnePartIsPickedForTheAgentByItsNumber(t *testing.T) {
	lines := []string{"Hibernation: broken. the kernel offers no hibernation", "Webcam: partly. Too dark"}
	for answer, want := range map[string]int{"2\n": 1, " 1 \n": 0, "\n": -1, "3\n": -1, "camera\n": -1, "": -1} {
		var asked strings.Builder
		picked, ok := pickOne(bufio.NewReader(strings.NewReader(answer)), &asked, lines)
		if !ok {
			picked = -1
		}
		if picked != want {
			t.Errorf("answer %q picked %d, want %d", answer, picked, want)
		}
		if !strings.Contains(asked.String(), "one part at a time") || !strings.Contains(asked.String(), "  2  Webcam: partly. Too dark") {
			t.Errorf("asked:\n%s", asked.String())
		}
	}
}

func TestAnAgentHasTheOwnerDoAStepAndGetsItsResultBack(t *testing.T) {
	// A stand-in for the terminal window. It finds the file the step is to answer in, and answers as
	// the step would once the owner is through
	dir := t.TempDir()
	window := filepath.Join(dir, "window")
	os.WriteFile(window, []byte(`#!/bin/sh
echo "$@" > `+dir+`/opened
while [ $# -gt 1 ]; do [ "$1" = --done ] && file=$2; shift; done
sleep 1
printf '{"steps":["camera"],"results":[{"check":"camera","state":"works","how":"owner"}]}\n' > "$file"
`), 0o755)
	t.Setenv("HW_TEST_TERMINAL", window)
	out := os.Stdout
	read, write, _ := os.Pipe()
	os.Stdout = write
	code := ask([]string{"camera"})
	write.Close()
	os.Stdout = out
	var answered struct {
		Steps   []string        `json:"steps"`
		Results []report.Result `json:"results"`
	}
	if err := json.NewDecoder(read).Decode(&answered); err != nil || code != 0 || len(answered.Results) != 1 || answered.Results[0].State != report.Works {
		t.Fatalf("exit %d, %v, answered %+v", code, err, answered)
	}
	// The step was opened for the owner alone, to end by itself, with nothing sent
	opened, _ := os.ReadFile(filepath.Join(dir, "opened"))
	for _, part := range []string{" camera ", "--no-send", "--once", "--done "} {
		if !strings.Contains(string(opened), part) {
			t.Errorf("the window was opened with %q, lacking %q", opened, part)
		}
	}
	// A word that names no step opens nothing
	if ask([]string{"flux"}) != 2 || ask(nil) != 2 {
		t.Error("something that is no step was opened")
	}

	// A step that can't run leaves its window open with the reason, and the agent is told it at
	// once, also when the step ended before it could answer at all
	os.WriteFile(window, []byte("#!/bin/sh\n\"$@\" </dev/null >"+dir+"/shown 2>&1 &\n"), 0o755)
	step := filepath.Join(dir, "hw-test")
	executable = func() (string, error) { return step, nil }
	ownerWait = 10 * time.Second
	defer func() { executable, ownerWait = os.Executable, 30*time.Minute }()
	for name, test := range map[string]struct{ step, said string }{
		"no step": {`while [ $# -gt 1 ]; do [ "$1" = --done ] && file=$2; shift; done
printf '{"steps":["camera"],"results":null,"error":"this computer has nothing to test for camera"}\n' > "$file"
echo "hw-test: the guided test couldn't run here: this computer has nothing to test for camera" >&2
exit 1`, "camera couldn't run in the owner's window: this computer has nothing to test for camera"},
		"no answer": {"exit 3", "hw-test ended with exit code 3 before the step was through"},
	} {
		os.WriteFile(step, []byte("#!/bin/sh\n"+test.step+"\n"), 0o755)
		os.Remove(filepath.Join(dir, "shown"))
		stderr := os.Stderr
		read, write, _ := os.Pipe()
		os.Stderr = write
		started := time.Now()
		code := ask([]string{"camera"})
		write.Close()
		os.Stderr = stderr
		said, _ := io.ReadAll(read)
		if code != 1 || !strings.Contains(string(said), test.said) || time.Since(started) > 5*time.Second {
			t.Errorf("%s: exit %d after %s, said:\n%s", name, code, time.Since(started).Round(time.Second), said)
		}
		for waited := 0; waited < 50; waited++ {
			if shown, _ := os.ReadFile(filepath.Join(dir, "shown")); strings.Contains(string(shown), "Press Enter to close this window.") {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if shown, _ := os.ReadFile(filepath.Join(dir, "shown")); !strings.Contains(string(shown), "Press Enter to close this window.") {
			t.Errorf("%s: the window didn't wait to be read:\n%s", name, shown)
		}
	}
}

// An agent that finds the computer doesn't have a part records that, which takes back what was
// recorded for it. Anything else that is no state is refused, with the states named.
func TestOneResultCanSayThePartIsNotThere(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	// The owner said in the guided test that sound stayed on the speakers
	if code := tell(&report.Run{Model: "lenovo-yoga-slim-7x-14q8x9"}, "http://127.0.0.1:1", "headphones", report.Broken, report.Owner, "Sound stays on the speakers.", "", false); code != 0 {
		t.Fatalf("the owner's result was refused with exit code %d", code)
	}
	run := &report.Run{Model: "lenovo-yoga-slim-7x-14q8x9"}
	if code := tell(run, "http://127.0.0.1:1", "headphones", report.Absent, "agent", "Three USB-C ports and no 3.5 mm jack.", "", false); code != 0 {
		t.Fatalf("absent was refused with exit code %d", code)
	}
	if kept := report.Local()["headphones"]; kept.State != report.Absent {
		t.Errorf("the computer's own results hold %q for the jack, not absent", kept.State)
	}
	if code := tell(&report.Run{Model: "lenovo-yoga-slim-7x-14q8x9"}, "http://127.0.0.1:1", "headphones", "missing", "agent", "No jack.", "", false); code != 2 {
		t.Errorf("an unknown state was taken, with exit code %d", code)
	}
}

// That a part works because of a change is said of a part that works here, at least partly, and
// what was recorded on the site can be read back on the computer
func TestANeedIsForAPartThatWorksAndIsKept(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	run := &report.Run{Model: "lenovo-yoga-slim-7x-14q8x9", Kernel: "7.3.0-73-omarchy"}
	// No result yet, then a broken one
	if code := need(run, "http://127.0.0.1:1", "suspend", "The kernel option x.", "agent", false, false); code != 2 {
		t.Errorf("a need for an untested part ended with %d", code)
	}
	if code := tell(&report.Run{Model: run.Model}, "http://127.0.0.1:1", "suspend", report.Broken, report.Owner, "It never wakes.", "", false); code != 0 {
		t.Fatal(code)
	}
	if code := need(run, "http://127.0.0.1:1", "suspend", "The kernel option x.", "agent", false, false); code != 2 {
		t.Errorf("a need for a broken part ended with %d", code)
	}
	if code := need(run, "http://127.0.0.1:1", "no-such-check", "The kernel option x.", "agent", false, false); code != 2 {
		t.Errorf("a need for no check ended with %d", code)
	}
	if code := tell(&report.Run{Model: run.Model}, "http://127.0.0.1:1", "suspend", report.Works, report.Owner, "It wakes every time now.", "", false); code != 0 {
		t.Fatal(code)
	}
	if code := need(run, "http://127.0.0.1:1", "suspend", "The kernel option x.", "measured", false, false); code != 2 {
		t.Errorf("a need said to be measured ended with %d", code)
	}
	if code := need(run, "http://127.0.0.1:1", "suspend", "The kernel option x.", "agent", true, false); code != 0 {
		t.Errorf("a need for a working part, not sent, ended with %d", code)
	}
	// Sent to a site that takes it, it is kept, and taken back it is gone
	var got []report.Need
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body report.Need
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = append(got, body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"page": "https://arewearmyet.com/models/lenovo-yoga-slim-7x-14q8x9"}`))
	}))
	defer site.Close()
	if code := need(run, site.URL, "suspend", "The kernel option x, MAC aa:bb:cc:dd:ee:ff.", "agent", true, true); code != 0 {
		t.Fatalf("sending ended with %d", code)
	}
	want := report.Need{Check: "suspend", Text: "The kernel option x, MAC xx:xx:xx:xx:xx:xx.", How: "agent", Proved: true, Kernel: "7.3.0-73-omarchy"}
	if len(got) != 1 || got[0] != want || report.Needs()["suspend"] != want {
		t.Errorf("sent %+v, kept %+v", got, report.Needs())
	}
	if code := need(run, site.URL, "suspend", "none", "agent", false, true); code != 0 {
		t.Fatalf("taking back ended with %d", code)
	}
	if len(report.Needs()) != 0 || got[1].Text != "" {
		t.Errorf("after taking back: sent %+v, kept %+v", got[1], report.Needs())
	}
}

func TestTheSudoRuleEndsByItselfAndIsCheckedBeforeItIsPutInPlace(t *testing.T) {
	until := time.Date(2026, 11, 21, 9, 30, 0, 0, time.FixedZone("CET", 3600))
	script := trustScript("birk", until)
	// The time is in the rule itself, in UTC, so it ends with nothing left to run
	if !strings.Contains(script, "birk ALL=(ALL:ALL) NOTAFTER=20261121083000Z NOPASSWD: ALL\n") {
		t.Errorf("the rule:\n%s", script)
	}
	// A broken rule would lock sudo, so it is checked first and the script stops there
	check, install := strings.Index(script, "visudo -cf \"$new\""), strings.Index(script, "install -m 0440 -o root -g root \"$new\" \"$rule\"")
	if !strings.HasPrefix(script, "set -eu\n") || check < 0 || install < check {
		t.Errorf("the rule isn't checked before it is installed:\n%s", script)
	}
	if !strings.Contains(script, "rule=/etc/sudoers.d/hw-test-trust\n") {
		t.Errorf("the rule goes elsewhere:\n%s", script)
	}
}

func TestTrustTakesHoursOrDaysUpToAWeek(t *testing.T) {
	for word, want := range map[string]int{"1": 1, "24": 24, "168": 168, "1d": 24, "3d": 72, "7d": 168} {
		if got, ok := trustHours(word); !ok || got != want {
			t.Errorf("trustHours(%q) = %d, %v, want %d", word, got, ok, want)
		}
	}
	for _, word := range []string{"0", "169", "0d", "8d", "d", "-1", "2h", "soon", ""} {
		if got, ok := trustHours(word); ok {
			t.Errorf("trustHours(%q) = %d, want it turned down", word, got)
		}
	}
}

func TestTrustTakesOneHourToAWeekAndAnAgentDoesNotLengthenItsOwn(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, hours := range []string{"0", "169", "8d", "soon"} {
		if code := trust([]string{hours}, false, false, false); code != 2 {
			t.Errorf("trust %s: %d", hours, code)
		}
	}
	// With the owner's word in force, a run without a terminal changes nothing
	until := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	if err := report.SetTrust(until); err != nil {
		t.Fatal(err)
	}
	if code := trust([]string{"24"}, false, false, false); code != 0 {
		t.Errorf("trust while trusted: %d", code)
	}
	if kept, held := report.Trusted(time.Now()); !held || !kept.Equal(until) {
		t.Errorf("the time was changed without the owner: %v, %v", kept, held)
	}
	if _, err := os.Stat(filepath.Join(report.StateDir(), "trust.sh")); err == nil {
		t.Error("a rule was written without the owner")
	}
}
