package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pending is a guided test that is under way, kept on disk from step to step. These tests run on
// machines where a step can take the whole computer down, like sleep that never wakes, or a camera
// or sound driver that crashes the kernel. The next run then finds the test where it stopped. The
// answers given so far aren't lost, and a restart in the middle of a step is itself what that step
// found.
type Pending struct {
	// Boot is the kernel's id for the boot the test ran in. A different one now means a restart.
	Boot      string    `json:"boot"`
	Step      string    `json:"step"`
	SleepMode string    `json:"sleepMode"`
	Saved     time.Time `json:"saved"`
	Results   []Result  `json:"results"`
}

func pendingFile() string { return filepath.Join(stateDir(), "pending.json") }

// BootID is the kernel's id for this boot, new on every start.
func BootID() string {
	data, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(data))
}

// SavePending writes the run's results so far, with the step that is about to start.
func SavePending(step, sleepMode string, run *Run) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.Marshal(Pending{Boot: BootID(), Step: step, SleepMode: sleepMode, Saved: time.Now(), Results: run.Results})
	return os.WriteFile(pendingFile(), data, 0o600)
}

// LoadPending returns the test that was under way, if one was saved in the last week.
func LoadPending() (Pending, bool) {
	var pending Pending
	data, err := os.ReadFile(pendingFile())
	if err != nil || json.Unmarshal(data, &pending) != nil || time.Since(pending.Saved) > 7*24*time.Hour {
		return Pending{}, false
	}
	return pending, true
}

// ClearPending forgets the saved test.
func ClearPending() { os.Remove(pendingFile()) }

// Restarted says whether the computer has restarted since this test was saved.
func (p Pending) Restarted(boot string) bool {
	return p.Boot != "" && boot != "" && p.Boot != boot
}

// Restore puts an interrupted test's results back into a fresh run, which are the owner's answers,
// and anything the new quick check didn't measure again.
func (p Pending) Restore(run *Run) {
	for _, saved := range p.Results {
		if _, measuredAgain := run.Get(saved.Check); !measuredAgain || saved.How == Owner {
			run.Set(saved.Check, saved.State, saved.How, saved.Evidence, saved.Note)
		}
	}
}

// Crashed records that the computer went down during a step, as that step's result. The owner
// has confirmed it restarted by itself.
func (p Pending) Crashed(run *Run, check string) {
	evidence := "the computer restarted by itself during the test's " + p.Step + " step, before the step finished"
	if p.Step == "Sleep" {
		mode := p.SleepMode
		if mode == "" {
			mode = "unknown"
		}
		evidence = "closing the lid ended in a restart instead of a wake, as the test was waiting for the computer to wake when it booted fresh (sleep modes " + mode + ")"
	}
	run.Set(check, Broken, Measured, evidence, "")
}
