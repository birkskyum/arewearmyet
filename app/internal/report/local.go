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
	// Shallow is set for a result of the quick check, which sees that a part is there and
	// not that it does its work.
	Shallow bool `json:"shallow,omitempty"`
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
// one, with two exceptions. A measurement which merely finds a thing working doesn't take
// back a failure the owner reported: that stands until the owner tests the thing again, and
// the site counts results the same way. And the quick check's "works" doesn't take back a
// failure that a closer test found.
func Remember(run *Run, when time.Time) error {
	kept := Local()
	for _, result := range run.Results {
		if result.State == Untested {
			continue
		}
		before, had := kept[result.Check]
		failing := had && (before.State == Partial || before.State == Broken)
		ownerSawItFail := failing && before.How == Owner
		if ownerSawItFail && result.How == Measured && result.State == Works {
			continue
		}
		// Nor does the quick check, seeing only that the part is there, take back a failure
		// that a test which used the part found. That stands until that test runs again.
		shallow := run.Shallow(result)
		if failing && !before.Shallow && shallow && result.State == Works {
			continue
		}
		kept[result.Check] = Kept{Result: result, Kernel: run.Kernel, When: when, Shallow: shallow}
	}
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	// Beside what stands, the last thing each test found, whatever stood before: that is what
	// a fix is measured by
	last := Latest()
	for _, result := range run.Results {
		if result.State == Untested {
			continue
		}
		// The quick check's look doesn't cover up what a closer test of the part found last
		shallow := run.Shallow(result)
		if before, had := last[result.Check]; had && shallow && !before.Shallow {
			continue
		}
		last[result.Check] = Kept{Result: result, Kernel: run.Kernel, When: when, Shallow: shallow}
	}
	if data, err := json.MarshalIndent(last, "", " "); err == nil {
		_ = os.WriteFile(latestFile(), data, 0o600)
	}
	data, _ := json.MarshalIndent(kept, "", " ")
	return os.WriteFile(localFile(), data, 0o600)
}

func latestFile() string { return filepath.Join(stateDir(), "measured.json") }

// Latest is the last result each check's test gave on this machine, with nothing held back:
// where Local keeps an owner's failure standing until the owner tests again, this is simply
// what was found last.
func Latest() map[string]Kept {
	last := map[string]Kept{}
	if data, err := os.ReadFile(latestFile()); err == nil {
		_ = json.Unmarshal(data, &last)
	}
	return last
}

// handedFile holds which checks the owner gave to an agent, and when.
func handedFile() string { return filepath.Join(stateDir(), "handed.json") }

// HandedFor is how long the owner's handing a part to an agent stands.
const HandedFor = 14 * 24 * time.Hour

// Hand notes that the owner gave these checks to an agent. That is what lets the agent run
// the tests of those parts itself, also one that plays tones and listens to them.
func Hand(checks []string, when time.Time) error {
	handed := map[string]time.Time{}
	if data, err := os.ReadFile(handedFile()); err == nil {
		_ = json.Unmarshal(data, &handed)
	}
	for _, check := range checks {
		handed[check] = when
	}
	data, _ := json.MarshalIndent(handed, "", " ")
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(handedFile(), data, 0o600)
}

// Handed says whether the owner gave a check to an agent, and not too long ago.
func Handed(check string, now time.Time) bool {
	handed := map[string]time.Time{}
	if data, err := os.ReadFile(handedFile()); err == nil {
		_ = json.Unmarshal(data, &handed)
	}
	when, ok := handed[check]
	return ok && now.Sub(when) < HandedFor && !when.After(now.Add(time.Hour))
}

// threadFile holds, per folder, the agent conversation hw-test started for the work there.
func threadFile() string { return filepath.Join(stateDir(), "threads.json") }

// Thread is a conversation hw-test started: the program that holds it and its name for it.
type Thread struct {
	Kind string    `json:"kind"`
	ID   string    `json:"id"`
	When time.Time `json:"when"`
	// Checks are the parts the owner last handed to it, for saying what the agent is on.
	Checks []string `json:"checks,omitempty"`
	// Title is what its program calls it, where that is known, for telling the owner which it is.
	Title string `json:"title,omitempty"`
}

func threads() map[string]Thread {
	kept := map[string]Thread{}
	if data, err := os.ReadFile(threadFile()); err == nil {
		_ = json.Unmarshal(data, &kept)
	}
	return kept
}

// ThreadFor is the conversation going for the work in a folder, when there is one that was
// used not too long ago.
func ThreadFor(dir string) (Thread, bool) {
	thread, ok := threads()[dir]
	return thread, ok && thread.ID != "" && time.Since(thread.When) < HandedFor
}

// NoteSession notes which conversation the agent working in a folder is in, as its own
// program names it, each time that agent calls hw-test from there. That is how hw-test
// knows a conversation it didn't start. A thread hw-test started in T3 Code stays the one
// kept: the agent inside it names only its own, inner conversation.
func NoteSession(dir, kind, id string, when time.Time) error {
	kept := threads()[dir]
	if kept.Kind == "t3" && kind != "t3" && kept.ID != "" && when.Sub(kept.When) < HandedFor {
		return nil
	}
	return KeepThread(dir, Thread{Kind: kind, ID: id, When: when, Checks: kept.Checks})
}

// NoteWork notes which parts the owner handed to the agent working in a folder.
func NoteWork(dir string, checks []string, when time.Time) error {
	kept := threads()[dir]
	kept.Checks, kept.When = checks, when
	return KeepThread(dir, kept)
}

// KeepThread notes the conversation started for the work in a folder, so the next part, a
// new result and the word after a restart all go into the same one.
func KeepThread(dir string, thread Thread) error {
	threads := map[string]Thread{}
	if data, err := os.ReadFile(threadFile()); err == nil {
		_ = json.Unmarshal(data, &threads)
	}
	threads[dir] = thread
	data, _ := json.MarshalIndent(threads, "", " ")
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(threadFile(), data, 0o600)
}

func testEntryFile() string { return filepath.Join(stateDir(), "test-entry.json") }

// TestEntry is the test boot entry hw-test tree boot installed, noted here because the boot
// partition can't be read without the owner's password. Kernel is the kernel its image was
// built from: an update to another kernel leaves the entry with one that no longer matches.
type TestEntry struct {
	Kernel string    `json:"kernel"`
	When   time.Time `json:"when"`
}

// NoteTestEntry records that a test boot entry was installed.
func NoteTestEntry(kernel string, when time.Time) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(TestEntry{Kernel: kernel, When: when}, "", " ")
	return os.WriteFile(testEntryFile(), data, 0o600)
}

// ClearTestEntry records that the test boot entry was taken away.
func ClearTestEntry() { os.Remove(testEntryFile()) }

// InstalledTestEntry is the test boot entry that was noted, if one stands.
func InstalledTestEntry() (TestEntry, bool) {
	var entry TestEntry
	data, err := os.ReadFile(testEntryFile())
	if err != nil || json.Unmarshal(data, &entry) != nil || entry.When.IsZero() {
		return TestEntry{}, false
	}
	return entry, true
}
