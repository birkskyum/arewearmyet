package main

import (
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
