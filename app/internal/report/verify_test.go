package report

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func result(check, state, how, evidence string) Result {
	return Result{Check: check, State: state, How: how, Evidence: evidence}
}

func TestAChangeIsMeasuredAgainstTheSnapshotByWhatWasTestedAgain(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	start := time.Date(2026, 11, 19, 6, 0, 0, 0, time.UTC)

	// Where the machine stands: the speakers partly, by the owner's ear; the GPU's closer
	// test failing; Wi-Fi and the audio processor fine by the quick check
	tests := &Run{Kernel: "7.3.0-72-omarchy", Results: []Result{
		{Check: "speakers", State: Partial, How: Owner, Evidence: "left heard at -31 dB, right not heard", Note: "Only the left speaker plays."},
		result("webgpu", Partial, Measured, "WebGPU only with switches"),
		result("camera", Works, Owner, "a 1920x1080 frame"),
	}}
	if err := Remember(tests, start.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	quick := &Run{Kernel: "7.3.0-72-omarchy", Packages: map[string]string{"linux-omarchy-dtbs": "7.3-9"}, Results: []Result{
		result("wifi", Works, Measured, "wlan0 connected"), result("adsp", Broken, Measured, "adsp is offline"), result("gpu", Works, Measured, "adreno bound")}}
	quick.MarkQuick()
	_ = Remember(quick, start.Add(-time.Hour))
	snapshot, err := TakeSnapshot(quick, start)
	if err != nil || len(snapshot.Results) != 6 || snapshot.Versions.Packages["linux-omarchy-dtbs"] != "7.3-9" {
		t.Fatalf("snapshot %+v, %v", snapshot, err)
	}
	if loaded, ok := LoadSnapshot(); !ok || !loaded.When.Equal(start) || loaded.Results["speakers"].State != Partial {
		t.Fatalf("the snapshot wasn't kept: %+v", loaded)
	}
	// Nothing tested since: nothing to say about the change
	if changes := Changes(snapshot, Latest()); len(changes) != 0 {
		t.Fatalf("changes with nothing tested again: %+v", changes)
	}

	// After the change the sound test hears both speakers, and the quick check runs again
	after := &Run{Kernel: "7.3.0-72-omarchy", Results: []Result{
		result("speakers", Works, Measured, "left heard at -31 dB, right heard at -32 dB"), result("microphones", Works, Measured, "1000 Hz at -35 dB")}}
	_ = Remember(after, start.Add(10*time.Minute))
	again := &Run{Kernel: "7.3.0-72-omarchy", Results: []Result{
		result("wifi", Works, Measured, "wlan0 connected"), result("adsp", Works, Measured, "adsp is running"), result("webgpu", Works, Measured, "the GPU is there")}}
	again.MarkQuick()
	_ = Remember(again, start.Add(11*time.Minute))

	changes := Changes(snapshot, Latest())
	by := map[string]Change{}
	var order []string
	for _, change := range changes {
		by[change.Check] = change
		order = append(order, change.Check)
	}
	// What got better comes first; what was never tested before has no "before"; the camera,
	// not tested again, isn't mentioned
	if strings.Join(order, " ") != "adsp speakers microphones wifi" {
		t.Errorf("the changes, in order: %v", order)
	}
	if got := by["speakers"]; got.From.State != Partial || got.From.How != Owner || got.To.State != Works || got.To.Evidence != "left heard at -31 dB, right heard at -32 dB" {
		t.Errorf("speakers: %+v", got)
	}
	if got := by["adsp"]; got.From.State != Broken || got.To.State != Works {
		t.Errorf("a part only the quick check measures: %+v", got)
	}
	if by["microphones"].From.State != Untested {
		t.Errorf("microphones: %+v", by["microphones"])
	}
	// The quick check seeing the GPU there doesn't make the closer test's finding better
	if _, claimed := by["webgpu"]; claimed {
		t.Error("the quick check's look was taken for the closer test's")
	}
	// A closer test run after the change is what counts for its part, also when the quick
	// check looks at the same part afterwards
	closer := &Run{Kernel: "7.3.0-72-omarchy", Results: []Result{result("webgpu", Works, Measured, "Chromium offers WebGPU on qualcomm adreno-7xx")}}
	_ = Remember(closer, start.Add(12*time.Minute))
	_ = Remember(again, start.Add(13*time.Minute))
	if got := Latest()["webgpu"]; got.Evidence != "Chromium offers WebGPU on qualcomm adreno-7xx" || got.Shallow {
		t.Errorf("the quick check covered up the closer test: %+v", got)
	}
	for _, change := range Changes(snapshot, Latest()) {
		if change.Check == "webgpu" && (change.From.State != Partial || change.To.State != Works) {
			t.Errorf("webgpu after its own test: %+v", change)
		}
	}
	_ = Remember(&Run{Kernel: "7.3.0-72-omarchy", Results: []Result{result("webgpu", Partial, Measured, "WebGPU only with switches")}}, start.Add(14*time.Minute))
	// And where the machine stands keeps the closer test's failure too
	if kept := Local()["webgpu"]; kept.State != Partial {
		t.Errorf("the quick check took back a closer test's failure: %+v", kept)
	}
	// The owner's failure still stands on the status board until the owner says otherwise,
	// while the measurement that the fix is judged by is the newest one
	if Local()["speakers"].State != Partial || Latest()["speakers"].State != Works {
		t.Errorf("standing %q, last measured %q", Local()["speakers"].State, Latest()["speakers"].State)
	}
}

func TestAFixIsSentWithWhatItChangedAndTheVersionsOnBothSides(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var got Fix
	var token string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = r.Header.Get("x-machine-token")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"fix": "FIX-AAAAAAAAAA", "url": "https://arewearmyet.com/api/fixes?url=x"}`))
	}))
	defer server.Close()
	fix := Fix{Model: "lenovo-yoga-slim-7x-14q8x9", URL: "https://github.com/omacom/omarchy-pkgs/pull/3100", Note: "Right speaker reset GPIO",
		Changes: []Change{
			{Check: "speakers", From: result("speakers", Partial, Owner, "right not heard"), To: result("speakers", Works, Measured, "both heard")},
			{Check: "microphones", From: Result{Check: "microphones", State: Untested}, To: result("microphones", Works, Measured, "heard")},
		},
		Before: Versions{Kernel: "7.3.0-72-omarchy"}, After: Versions{Kernel: "7.3.0-72-omarchy"}}
	sent, err := SendFix(server.URL, fix)
	if err != nil || sent.Fix != "FIX-AAAAAAAAAA" || len(token) < 32 {
		t.Fatalf("sent %+v, %v", sent, err)
	}
	// A check with no "before" isn't something the pull request changed
	if len(got.Changes) != 1 || got.Changes[0].Check != "speakers" || got.Changes[0].To.State != Works || got.URL != fix.URL || got.Before.Kernel == "" {
		t.Errorf("the site got %+v", got)
	}
	fix.Changes = fix.Changes[1:]
	if _, err = SendFix(server.URL, fix); err == nil || !strings.Contains(err.Error(), "tested before the change") {
		t.Errorf("nothing to compare: %v", err)
	}
}
