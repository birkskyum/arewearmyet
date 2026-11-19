package main

import (
	"strings"
	"testing"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

func TestBeforeALogGoesTheOwnerIsToldWhatGoesAndCanSayNo(t *testing.T) {
	said := attached([]report.Log{{Name: "previous.log", Text: "one\ntwo\nthree"}, {Name: "kernel.log", Text: "one"}})
	for _, part := range []string{"previous.log (3 lines), kernel.log (1 line)", "disk and install IDs", "public on the model's page", "--json"} {
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
