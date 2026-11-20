package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Versions are the kernel and packages a machine ran at one moment.
type Versions struct {
	Kernel   string            `json:"kernel"`
	Packages map[string]string `json:"packages"`
}

// Snapshot is where a machine stood before a change, with every check's standing result, and the
// versions it ran, taken so that what a fix changes can be measured against it.
type Snapshot struct {
	When     time.Time       `json:"when"`
	Versions Versions        `json:"versions"`
	Results  map[string]Kept `json:"results"`
	// Used is set once a change measured against it was recorded with its pull request.
	Used bool `json:"used,omitempty"`
	// Part is the part the change is about, when the snapshot was taken for one. Each part has
	// a snapshot of its own, so that the work on one isn't measured against another's.
	Part string `json:"part,omitempty"`
	// TestImage is set when the computer was running a test boot image at the time, with a
	// device tree changed for some part's sake.
	TestImage bool `json:"testImage,omitempty"`
}

// Change is one check before and after a fix.
type Change struct {
	Check string `json:"check"`
	From  Result `json:"from"`
	To    Result `json:"to"`
}

// Fix ties a pull request to what it changed on this machine, as measured.
type Fix struct {
	Model   string   `json:"model"`
	URL     string   `json:"url"`
	Note    string   `json:"note,omitempty"`
	Changes []Change `json:"changes"`
	Before  Versions `json:"before"`
	After   Versions `json:"after"`
}

// FixSent is the site's answer to a fix.
type FixSent struct {
	Fix   string `json:"fix"`
	URL   string `json:"url"`
	Page  string `json:"page"`
	Error string `json:"error"`
}

var partName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// PartNamed says whether a word can name a part's snapshot.
func PartNamed(part string) bool { return partName.MatchString(part) }

// snapshotFile is where a part's snapshot is kept. The one taken without naming a part is
// before.json, as it always was.
func snapshotFile(part string) string {
	if part == "" {
		return filepath.Join(stateDir(), "before.json")
	}
	return filepath.Join(stateDir(), "before-"+part+".json")
}

// TakeSnapshot keeps where the machine stands now, to measure a change to a part against.
func TakeSnapshot(run *Run, when time.Time, part string, testImage bool) (Snapshot, error) {
	snapshot := Snapshot{When: when, Versions: Versions{Kernel: run.Kernel, Packages: run.Packages}, Results: Local(), Part: part, TestImage: testImage}
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return snapshot, err
	}
	data, _ := json.MarshalIndent(snapshot, "", " ")
	return snapshot, os.WriteFile(snapshotFile(part), data, 0o600)
}

// MarkSnapshotUsed notes that the snapshot's change is recorded, so that the next
// hw-test verify before for the part takes a new one without being asked twice.
func MarkSnapshotUsed(part string) error {
	snapshot, ok := LoadSnapshot(part)
	if !ok {
		return nil
	}
	snapshot.Used = true
	data, _ := json.MarshalIndent(snapshot, "", " ")
	return os.WriteFile(snapshotFile(part), data, 0o600)
}

// LoadSnapshot reads the snapshot taken before a change to a part, if there is one.
func LoadSnapshot(part string) (Snapshot, bool) {
	var snapshot Snapshot
	data, err := os.ReadFile(snapshotFile(part))
	if err != nil || json.Unmarshal(data, &snapshot) != nil || snapshot.When.IsZero() {
		return Snapshot{}, false
	}
	return snapshot, true
}

// StandingParts names the parts whose snapshot is still being measured against, in order.
func StandingParts() []string {
	var parts []string
	files, _ := filepath.Glob(filepath.Join(stateDir(), "before-*.json"))
	for _, file := range files {
		part := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(file), "before-"), ".json")
		if snapshot, ok := LoadSnapshot(part); ok && PartNamed(part) && !snapshot.Used && snapshot.Part == part {
			parts = append(parts, part)
		}
	}
	sort.Strings(parts)
	return parts
}

// Rank orders states from fine to failing. A state that says nothing of working, like
// "not on this model", has none.
var Rank = map[string]int{Works: 0, Partial: 1, Broken: 2}

// Changes lists every check that was tested again since the snapshot, with its state then and now.
// What got better comes first, then what got worse, then what stayed as it was. A check nobody
// tested since says nothing about the change, and is left out.
func Changes(snapshot Snapshot, latest map[string]Kept) []Change {
	var changes []Change
	for check, now := range latest {
		if !now.When.After(snapshot.When) {
			continue
		}
		before, had := snapshot.Results[check]
		// The quick check's look at a part says nothing against what a closer test found of it
		if had && now.Shallow && !before.Shallow {
			continue
		}
		from := before.Result
		if !had {
			from = Result{Check: check, State: Untested}
		}
		changes = append(changes, Change{Check: check, From: from, To: now.Result})
	}
	order := func(c Change) int {
		from, known := Rank[c.From.State]
		to, measured := Rank[c.To.State]
		switch {
		case !known || !measured:
			return 2
		case to < from:
			return 0
		case to > from:
			return 1
		}
		return 3
	}
	sort.Slice(changes, func(a, b int) bool {
		if order(changes[a]) != order(changes[b]) {
			return order(changes[a]) < order(changes[b])
		}
		return changes[a].Check < changes[b].Check
	})
	return changes
}

// SendFix tells the site what a pull request changed on this machine.
func SendFix(api string, fix Fix) (FixSent, error) {
	token, err := Token()
	if err != nil {
		return FixSent{}, err
	}
	// A check that was never tested before has no "before" the site can count
	sent := fix
	sent.Changes = nil
	for _, change := range fix.Changes {
		if change.From.State != Untested {
			sent.Changes = append(sent.Changes, change)
		}
	}
	if len(sent.Changes) == 0 {
		return FixSent{}, fmt.Errorf("nothing that was tested before the change was tested again after it")
	}
	data, _ := json.Marshal(sent)
	request, err := http.NewRequest(http.MethodPost, api+"/api/fixes", bytes.NewReader(data))
	if err != nil {
		return FixSent{}, err
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("x-machine-token", token)
	client := http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return FixSent{}, err
	}
	defer response.Body.Close()
	var answer FixSent
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&answer); err != nil {
		return FixSent{}, fmt.Errorf("the site answered %s", response.Status)
	}
	if response.StatusCode >= 300 {
		return answer, fmt.Errorf("%s", first(answer.Error, response.Status))
	}
	return answer, nil
}
