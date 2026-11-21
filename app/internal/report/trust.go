package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Trust is the owner's word that their agent works on its own for a while: it restarts and
// changes system settings without asking first, and sudo asks it for no password. It is kept on
// the computer and never sent.
type Trust struct {
	Until time.Time `json:"until"`
}

func trustFile() string { return filepath.Join(stateDir(), "trust.json") }

// Trusted says until when the owner's word holds, when it holds now.
func Trusted(now time.Time) (time.Time, bool) {
	var trust Trust
	data, err := os.ReadFile(trustFile())
	if err != nil || json.Unmarshal(data, &trust) != nil || !now.Before(trust.Until) {
		return time.Time{}, false
	}
	return trust.Until, true
}

// SetTrust keeps the owner's word until the given time.
func SetTrust(until time.Time) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(Trust{Until: until}, "", " ")
	return os.WriteFile(trustFile(), data, 0o600)
}

// EndTrust takes the owner's word back.
func EndTrust() { os.Remove(trustFile()) }
