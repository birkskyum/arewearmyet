package resume

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func places(t *testing.T) Places {
	dir := t.TempDir()
	return Places{State: filepath.Join(dir, "state"), Units: filepath.Join(dir, "config", "systemd", "user")}
}

func TestAWordLeftBeforeARestartOpensTheAgentOnceAfterIt(t *testing.T) {
	p := places(t)
	set := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)
	word := Word{Dir: "/home/owner/Work/hw-bringup", Prompt: "Booted the test entry with the speaker GPIO changed. Run hw-test speakers.", Boot: "boot-one", Set: set}
	if _, found := p.Left(); found {
		t.Fatal("a word before any was left")
	}
	if err := p.Leave(word, "/home/owner/.local/bin/hw-test"); err != nil {
		t.Fatal(err)
	}
	left, found := p.Left()
	if !found || left.Prompt != word.Prompt || left.Boot != "boot-one" {
		t.Fatalf("left %+v, %v", left, found)
	}
	// The service is in place for the next login, pointing at the program
	unit, err := os.ReadFile(p.unit())
	if err != nil || !strings.Contains(string(unit), "WantedBy=graphical-session.target") || !strings.Contains(string(unit), "'/home/owner/.local/bin/hw-test' resume --run") || !strings.Contains(string(unit), "Type=oneshot") {
		t.Fatalf("the service: %v\n%s", err, unit)
	}
	if target, err := os.Readlink(p.link()); err != nil || target != p.unit() {
		t.Errorf("the service isn't wanted at login: %q, %v", target, err)
	}

	// A login in the same boot is the owner logging out and in, so the word waits
	if got := At(left, found, "boot-one", set.Add(10*time.Minute)); got != Wait {
		t.Errorf("same boot: %v", got)
	}
	// After a restart, it opens
	if got := At(left, found, "boot-two", set.Add(10*time.Minute)); got != Open {
		t.Errorf("after a restart: %v", got)
	}
	// Days later it would only be a surprise
	if got := At(left, found, "boot-two", set.Add(25*time.Hour)); got != Stale {
		t.Errorf("a day later: %v", got)
	}
	// A clock set back to before the word was left can't be trusted either
	if got := At(left, found, "boot-two", set.Add(-time.Hour)); got != Stale {
		t.Errorf("before it was left: %v", got)
	}
	// With the boot unknown it doesn't guess
	if got := At(left, found, "", set.Add(time.Minute)); got != Wait {
		t.Errorf("an unknown boot: %v", got)
	}
	if got := At(Word{}, false, "boot-two", set); got != Nothing {
		t.Errorf("no word: %v", got)
	}

	// What the agent is told has its own words in it, and where to look first
	if said := Said(left); !strings.Contains(said, "Read BRIEF.md and PROGRESS.md") || !strings.Contains(said, "Run hw-test speakers.") {
		t.Errorf("said: %s", said)
	}

	// Cleared, nothing is left to run again
	p.Clear()
	if _, found := p.Left(); found {
		t.Error("the word outlived its use")
	}
	for _, path := range []string{p.unit(), p.link()} {
		if _, err := os.Lstat(path); err == nil {
			t.Errorf("%s is still there", path)
		}
	}
	// Clearing twice, or with nothing there, is harmless
	p.Clear()
}

func TestAProgramPathWithOddLettersStaysOneArgument(t *testing.T) {
	unit := Unit(`/home/o'brien/my tools/hw-test`)
	if !strings.Contains(unit, `'/home/o'\''brien/my tools/hw-test' resume --run`) {
		t.Errorf("the path isn't kept whole:\n%s", unit)
	}
	// A damaged word file is no word
	p := places(t)
	os.MkdirAll(p.State, 0o700)
	os.WriteFile(p.word(), []byte("{not json"), 0o600)
	if _, found := p.Left(); found {
		t.Error("a damaged file counted as a word")
	}
}
