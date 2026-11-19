package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pending is a guided test that was under way, kept on disk while the computer sleeps for
// the sleep step. If the computer restarts instead of waking, the next run finds it: the
// restart is itself the result, and the answers given before it aren't lost.
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

// Restarted says whether the computer restarted while this test waited for it to wake.
func (p Pending) Restarted(boot string) bool {
	return p.Step == "Sleep" && p.Boot != "" && boot != "" && p.Boot != boot
}

// Restore puts a restarted test's results back into a fresh run: the owner's answers, and
// anything the new quick check didn't measure again. Sleep itself is recorded as broken.
func (p Pending) Restore(run *Run) {
	for _, saved := range p.Results {
		if _, measuredAgain := run.Get(saved.Check); !measuredAgain || saved.How == Owner {
			run.Set(saved.Check, saved.State, saved.How, saved.Evidence, saved.Note)
		}
	}
	mode := p.SleepMode
	if mode == "" {
		mode = "unknown"
	}
	run.Set("suspend", Broken, Measured, "closing the lid ended in a restart instead of a wake: the test was waiting for the computer to wake when it booted fresh (sleep modes "+mode+")", "")
}
