package guided

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// hinge is a machine whose lid the test moves, with what the system says of it now, and what it has
// written down about it.
type hinge struct {
	down  string // "b true", "b false", or "" when it can't be asked
	log   string
	nolog bool
	clock time.Time
}

func (h *hinge) lid(run *report.Run) *lid {
	sys := machine.Fixture("/nonexistent")
	sys.Command = func(name string, args ...string) (string, error) {
		switch name {
		case "busctl":
			if h.down == "" {
				return "", errors.New("no bus")
			}
			return h.down + "\n", nil
		case "journalctl":
			if h.nolog {
				return "", errors.New("not allowed")
			}
			return h.log, nil
		}
		return "", errors.New("no such program")
	}
	h.clock = time.Date(2026, 11, 19, 5, 0, 0, 0, time.UTC)
	l := newLid(run, sys)
	l.now = func() time.Time { return h.clock }
	return l
}

func (h *hinge) look(l *lid, after time.Duration) {
	h.clock = h.clock.Add(after)
	l.update(lidTick{})
}

func TestTheLidIsWatchedClosingAndOpening(t *testing.T) {
	run := &report.Run{}
	h := &hinge{down: "b false"}
	l := h.lid(run)
	if l.start() == nil || l.done() {
		t.Fatal("it didn't start watching")
	}
	h.look(l, 300*time.Millisecond)
	if l.phase != "closing" || !strings.Contains(l.view(90, 30), "Waiting for the lid to close") {
		t.Fatalf("phase %q:\n%s", l.phase, l.view(90, 30))
	}
	// An open lid alone, however long, is nothing
	if l.done() || len(run.Results) != 0 {
		t.Fatalf("an open lid counted: %+v", run.Results)
	}
	h.down = "b true"
	h.look(l, 300*time.Millisecond)
	if l.phase != "opening" || !strings.Contains(l.view(90, 30), "Now open it") {
		t.Fatalf("phase %q:\n%s", l.phase, l.view(90, 30))
	}
	h.down = "b false"
	h.look(l, 300*time.Millisecond)
	result, _ := run.Get("lid")
	if !l.done() || result.State != report.Works || result.How != report.Measured || strings.Contains(result.Evidence, "slept") {
		t.Errorf("done %v, result %+v", l.done(), result)
	}
}

func TestALidThatPutTheComputerToSleepIsReadFromTheLog(t *testing.T) {
	run := &report.Run{}
	h := &hinge{down: "b false"}
	l := h.lid(run)
	l.start()
	h.look(l, 300*time.Millisecond)
	// The computer slept the moment the lid went down, so the test saw neither state, and is back
	// twenty seconds later with the lid open again
	h.log = "Lid closed.\nSuspending...\nLid opened.\n"
	h.look(l, 20*time.Second)
	result, _ := run.Get("lid")
	if !l.done() || result.State != report.Works || !strings.Contains(result.Evidence, "slept 20 s while it was down") {
		t.Errorf("done %v, result %+v", l.done(), result)
	}

	// An opening from before the closing doesn't count as the lid coming back up
	run = &report.Run{}
	h = &hinge{down: "", log: "Lid opened.\nLid closed.\n"}
	l = h.lid(run)
	l.start()
	h.look(l, 300*time.Millisecond)
	if l.done() || l.phase != "opening" {
		t.Errorf("done %v, phase %q", l.done(), l.phase)
	}
}

func TestALidThatSaysNothingIsTheOwnersWord(t *testing.T) {
	// Closed and opened, and the switch never moved
	run := &report.Run{}
	h := &hinge{down: "b false"}
	l := h.lid(run)
	l.start()
	h.look(l, 300*time.Millisecond)
	l.update(press('n'))
	if result, _ := run.Get("lid"); !l.done() || result.State != report.Broken || result.How != report.Owner {
		t.Errorf("silent: %+v", result)
	}
	// It reports closing, and never opening
	run = &report.Run{}
	h = &hinge{down: "b false"}
	l = h.lid(run)
	l.start()
	h.down = "b true"
	h.look(l, 300*time.Millisecond)
	l.update(press('n'))
	if result, _ := run.Get("lid"); result.State != report.Partial || !strings.Contains(result.Note, "opening it isn't") {
		t.Errorf("stuck closed: %+v", result)
	}
	// Skipped, so nothing is said about the lid
	run = &report.Run{}
	l = (&hinge{down: "b false"}).lid(run)
	l.start()
	l.update(press('s'))
	if !l.done() || len(run.Results) != 0 {
		t.Errorf("skipped: %+v", run.Results)
	}
	// A system that can be asked nothing about its lid has no test to run, and reports nothing
	run = &report.Run{}
	l = (&hinge{down: "", nolog: true}).lid(run)
	if l.start() != nil || !l.done() || len(run.Results) != 0 {
		t.Errorf("blind: done %v, %+v", l.done(), run.Results)
	}
}

// lids is the lid step in each of its phases, for the tests that draw every step.
func lids(run *report.Run) []step {
	waiting := (&hinge{down: "b false"}).lid(run)
	waiting.start()
	h := &hinge{down: "b true"}
	closed := h.lid(run)
	closed.start()
	h.look(closed, 300*time.Millisecond)
	return []step{waiting, closed}
}
