package report

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVersionsAreComparedByTheirNumbers(t *testing.T) {
	for _, pair := range [][2]string{{"0.12.1", "0.12.0"}, {"0.12.0", "0.9.9"}, {"v1.0.0", "0.99.99"}, {"0.10.0", "0.9.0"}} {
		if !After(pair[0], pair[1]) || After(pair[1], pair[0]) {
			t.Errorf("%s should be newer than %s", pair[0], pair[1])
		}
	}
	// The same version, a build without one, or nonsense is never newer
	for _, pair := range [][2]string{{"0.12.0", "0.12.0"}, {"0.12.1", "dev"}, {"dev", "0.12.0"}, {"0.12", "0.11.0"}, {"0.12.x", "0.11.0"}, {"", "0.1.0"}} {
		if After(pair[0], pair[1]) {
			t.Errorf("%q counted as newer than %q", pair[0], pair[1])
		}
	}
}

func TestANewerReleaseIsNoticedAndAskedAboutAtMostTwiceADay(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	asked, tag, status := 0, "v0.12.1", http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"tag_name": "` + tag + `"}`))
	}))
	defer server.Close()
	before := Releases
	Releases = server.URL
	defer func() { Releases = before }()
	now := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)

	if got := Newer("0.12.0", now); got != "0.12.1" || asked != 1 {
		t.Fatalf("newer %q after asking %d times", got, asked)
	}
	// Again within the day: the answer is remembered, and nobody is asked
	if got := Newer("0.12.0", now.Add(time.Hour)); got != "0.12.1" || asked != 1 {
		t.Errorf("newer %q after asking %d times", got, asked)
	}
	// The newest version itself has nothing to say
	if got := Newer("0.12.1", now.Add(2*time.Hour)); got != "" {
		t.Errorf("the newest version was told of %q", got)
	}
	// The next day it asks again
	tag = "v0.13.0"
	if got := Newer("0.12.1", now.Add(13*time.Hour)); got != "0.13.0" || asked != 2 {
		t.Errorf("newer %q after asking %d times", got, asked)
	}
	// A build without a version never asks
	if got := Newer("dev", now.Add(48*time.Hour)); got != "" || asked != 2 {
		t.Errorf("a build without a version: %q after asking %d times", got, asked)
	}
	// With nobody answering, what was known stands, and the failed try isn't repeated at once
	status = http.StatusForbidden
	if got := Newer("0.12.1", now.Add(30*time.Hour)); got != "0.13.0" || asked != 3 {
		t.Errorf("after a refusal: %q, asked %d times", got, asked)
	}
	if Newer("0.12.1", now.Add(31*time.Hour)); asked != 3 {
		t.Errorf("a failed try was repeated within the day: asked %d times", asked)
	}
	// After an update the next start asks afresh
	status = http.StatusOK
	ForgetRelease()
	if got := Newer("0.13.0", now.Add(32*time.Hour)); got != "" || asked != 4 {
		t.Errorf("after an update: %q, asked %d times", got, asked)
	}
}

func TestAMachineWithoutANetworkIsNotHeldUp(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	before := Releases
	// Nothing listens here, so the try fails at once
	Releases = "http://127.0.0.1:1/releases/latest"
	defer func() { Releases = before }()
	began := time.Now()
	if got := Newer("0.12.0", began); got != "" {
		t.Errorf("a release was made up: %q", got)
	}
	if waited := time.Since(began); waited > 3*time.Second {
		t.Errorf("the start was held up for %s", waited)
	}
}

func TestNothingAboutTheReleaseCheckCanStopTheStart(t *testing.T) {
	now := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)
	before := Releases
	defer func() { Releases = before }()
	answer := func(handler http.HandlerFunc) func() {
		server := httptest.NewServer(handler)
		Releases = server.URL
		return server.Close
	}

	// A server that takes the connection and never answers: the start waits two seconds, no more
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	release := make(chan struct{})
	stop := answer(func(w http.ResponseWriter, r *http.Request) { <-release })
	began := time.Now()
	if got := Newer("0.12.0", now); got != "" {
		t.Errorf("a hanging server gave %q", got)
	}
	if waited := time.Since(began); waited > 4*time.Second {
		t.Errorf("the start hung for %s", waited)
	}
	close(release)
	stop()

	// A login page in place of an answer, as a hotel's or a train's network gives
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	stop = answer(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body>Sign in to the network</body></html>"))
	})
	if got := Newer("0.12.0", now); got != "" {
		t.Errorf("a login page gave %q", got)
	}
	stop()

	// Answers that are JSON and still not a version
	for _, body := range []string{`{}`, `{"tag_name": 12}`, `{"tag_name": "nightly"}`, `{"tag_name": "v1.2"}`, `[]`, `null`, ``} {
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		stop = answer(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
		if got := Newer("0.12.0", now); got != "" {
			t.Errorf("%q gave %q", body, got)
		}
		stop()
	}

	// What was kept from before is damaged, or isn't what it should be
	stop = answer(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"tag_name": "v0.12.1"}`)) })
	defer stop()
	for _, kept := range []string{"{not json", `{"latest": 5, "checked": "yesterday"}`, `{"latest": "banana", "checked": "2026-11-19T06:00:00Z"}`, "\x00\x00\x00"} {
		dir := t.TempDir()
		t.Setenv("XDG_STATE_HOME", dir)
		if err := os.MkdirAll(stateDir(), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(releaseFile(), []byte(kept), 0o600); err != nil {
			t.Fatal(err)
		}
		// It may or may not ask again, but it answers with a real version or nothing
		if got := Newer("0.12.0", now); got != "" && got != "0.12.1" {
			t.Errorf("a damaged file %q gave %q", kept, got)
		}
	}

	// A state folder that can't be written to: the answer still comes, it just isn't kept
	locked := t.TempDir()
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o700)
	t.Setenv("XDG_STATE_HOME", locked)
	if got := Newer("0.12.0", now); got != "0.12.1" {
		t.Errorf("with nowhere to keep it: %q", got)
	}
	// And one where the place for the file is taken by something that isn't a folder
	blocked := t.TempDir()
	t.Setenv("XDG_STATE_HOME", blocked)
	if err := os.WriteFile(filepath.Join(blocked, "arewearmyet"), []byte("a file where the folder should be"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Newer("0.12.0", now); got != "0.12.1" {
		t.Errorf("with a file in the folder's place: %q", got)
	}
}
