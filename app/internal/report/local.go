package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Kept is a check's latest result on this machine, with when it was found and on which kernel.
type Kept struct {
	Result
	Kernel string    `json:"kernel"`
	When   time.Time `json:"when"`
}

func localFile() string { return filepath.Join(stateDir(), "results.json") }

// Local is this machine's latest result for each check, kept on the machine whether or not
// anything is sent, so its owner sees where it stands and what is left to get working.
func Local() map[string]Kept {
	kept := map[string]Kept{}
	if data, err := os.ReadFile(localFile()); err == nil {
		_ = json.Unmarshal(data, &kept)
	}
	return kept
}

// Remember keeps a run's results as the machine's latest. A newer result replaces an older
// one, except that a measurement which merely finds a thing working doesn't take back a
// failure the owner reported: that stands until the owner tests the thing again. The site
// counts results the same way.
func Remember(run *Run, when time.Time) error {
	kept := Local()
	for _, result := range run.Results {
		if result.State == Untested {
			continue
		}
		before, had := kept[result.Check]
		ownerSawItFail := had && before.How == Owner && (before.State == Partial || before.State == Broken)
		if ownerSawItFail && result.How == Measured && result.State == Works {
			continue
		}
		kept[result.Check] = Kept{Result: result, Kernel: run.Kernel, When: when}
	}
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(kept, "", " ")
	return os.WriteFile(localFile(), data, 0o600)
}
