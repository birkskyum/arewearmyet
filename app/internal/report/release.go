package report

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Releases is where the newest release of the app is asked for. Tests point it elsewhere.
var Releases = "https://api.github.com/repos/birkskyum/arewearmyet/releases/latest"

// How long an answer about the newest release is trusted before asking again.
const releaseAge = 12 * time.Hour

type release struct {
	Latest  string    `json:"latest"`
	Checked time.Time `json:"checked"`
}

func releaseFile() string { return filepath.Join(stateDir(), "release.json") }

// parts splits a version like 0.12.1 or v0.12.1 into its numbers. It fails on anything else,
// like a build that has no version.
func parts(version string) ([3]int, bool) {
	var numbers [3]int
	fields := strings.Split(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	if len(fields) != 3 {
		return numbers, false
	}
	for at, field := range fields {
		number, err := strconv.Atoi(field)
		if err != nil || number < 0 {
			return numbers, false
		}
		numbers[at] = number
	}
	return numbers, true
}

// After says whether one version is newer than another.
func After(version, than string) bool {
	a, ok := parts(version)
	b, known := parts(than)
	if !ok || !known {
		return false
	}
	for at := range a {
		if a[at] != b[at] {
			return a[at] > b[at]
		}
	}
	return false
}

// Newer returns the newest release when it is newer than the running version, and "" when it
// isn't or can't be told. It asks at most twice a day and waits two seconds at most, so a
// machine without a network starts as fast as one with. Nothing about the machine is sent.
func Newer(current string, now time.Time) string {
	if _, ok := parts(current); !ok {
		return ""
	}
	var kept release
	if data, err := os.ReadFile(releaseFile()); err == nil {
		_ = json.Unmarshal(data, &kept)
	}
	if kept.Latest == "" || now.Sub(kept.Checked) > releaseAge || now.Before(kept.Checked) {
		if latest := newest(); latest != "" {
			kept.Latest = latest
		}
		// A failed try also waits for the next one, so a machine that is offline isn't held up each time
		kept.Checked = now
		if os.MkdirAll(stateDir(), 0o700) == nil {
			data, _ := json.Marshal(kept)
			_ = os.WriteFile(releaseFile(), data, 0o600)
		}
	}
	if After(kept.Latest, current) {
		return strings.TrimPrefix(kept.Latest, "v")
	}
	return ""
}

// ForgetRelease drops what was kept about the newest release, after an update.
func ForgetRelease() { os.Remove(releaseFile()) }

func newest() string {
	client := http.Client{Timeout: 2 * time.Second}
	request, err := http.NewRequest(http.MethodGet, Releases, nil)
	if err != nil {
		return ""
	}
	request.Header.Set("accept", "application/vnd.github+json")
	request.Header.Set("user-agent", "hw-test")
	response, err := client.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	var answer struct {
		Tag string `json:"tag_name"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&answer) != nil {
		return ""
	}
	if _, ok := parts(answer.Tag); !ok {
		return ""
	}
	return answer.Tag
}
