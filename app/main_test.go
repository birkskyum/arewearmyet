package main

import (
	"bufio"
	"strings"
	"testing"

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
	typed := func(lines ...string) *bufio.Reader {
		return bufio.NewReader(strings.NewReader(strings.Join(lines, "\n") + "\n"))
	}

	// hw-test note: the part, what happened, and whether it was before the last restart
	var asked strings.Builder
	args, written := askNote(typed("sleep", "Slept about five minutes. Wi-Fi was gone on wake and needed a restart.", "y"), &asked, checks, noteArgs{kind: "problem"}, "")
	if !written || args.check != "suspend" || args.title != "Slept about five minutes. Wi-Fi was gone on wake and needed a restart" || strings.Join(args.logs, ",") != "previous" || !strings.HasPrefix(args.body, "Slept about five") {
		t.Errorf("asked for everything: %+v", args)
	}
	for _, question := range []string{"Which part is it about?", "What happened?", "restarted since it happened? [y/N]"} {
		if !strings.Contains(asked.String(), question) {
			t.Errorf("it didn't ask %q:\n%s", question, asked.String())
		}
	}
	// hw-test note wifi: the part is given, and Enter on the last question means this start's log
	asked.Reset()
	args, written = askNote(typed("It restarts by itself a few minutes after joining the network", ""), &asked, checks, noteArgs{kind: "problem"}, "wifi")
	if !written || args.check != "wifi" || strings.Join(args.logs, ",") != "kernel" || strings.Contains(asked.String(), "Which part") || args.title != "It restarts by itself a few minutes after joining the network" {
		t.Errorf("with the part given: %+v\n%s", args, asked.String())
	}
	// A word that names no part gets the list, and a note that fits none has no part
	asked.Reset()
	args, _ = askNote(typed("flux", "", "The clock is an hour off after every restart of the laptop.", "n"), &asked, checks, noteArgs{kind: "note"}, "")
	if args.check != "" || !strings.Contains(asked.String(), `No part is called "flux" here. The parts are: camera, lid, suspend, wifi.`) || args.logs[0] != "kernel" {
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
