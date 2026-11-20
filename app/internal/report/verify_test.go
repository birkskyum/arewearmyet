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

	// Where the machine stands. The speakers are partly working, by the owner's ear, the GPU's closer
	// test is failing, and Wi-Fi and the audio processor are fine by the quick check
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
	snapshot, err := TakeSnapshot(quick, start, "", false)
	if err != nil || len(snapshot.Results) != 6 || snapshot.Versions.Packages["linux-omarchy-dtbs"] != "7.3-9" {
		t.Fatalf("snapshot %+v, %v", snapshot, err)
	}
	// Once its change is recorded the snapshot says so, and is still there to measure against
	if loaded, _ := LoadSnapshot(""); loaded.Used {
		t.Fatal("a fresh snapshot counts as used")
	}
	if err := MarkSnapshotUsed(""); err != nil {
		t.Fatal(err)
	}
	if loaded, ok := LoadSnapshot(""); !ok || !loaded.Used || len(loaded.Results) != 6 {
		t.Fatalf("the used snapshot: %+v", loaded)
	}
	if snapshot, err = TakeSnapshot(quick, start, "", false); err != nil || snapshot.Used {
		t.Fatalf("a new snapshot: used %v, %v", snapshot.Used, err)
	}
	if loaded, ok := LoadSnapshot(""); !ok || !loaded.When.Equal(start) || loaded.Results["speakers"].State != Partial {
		t.Fatalf("the snapshot wasn't kept: %+v", loaded)
	}
	// Nothing tested since, so nothing to say about the change
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
	// What got better comes first. What was never tested before has no "before". The camera, not
	// tested again, isn't mentioned
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
	// Where the machine stands keeps the closer test's "works" too, with what that test saw
	if kept := Local()["webgpu"]; kept.Evidence != "Chromium offers WebGPU on qualcomm adreno-7xx" || kept.Shallow {
		t.Errorf("the quick check's works replaced a closer test's works: %+v", kept)
	}
	// A part the quick check finds gone is news, whatever a closer test found before
	gone := &Run{Kernel: "7.3.0-72-omarchy", Results: []Result{result("webgl", Broken, Measured, "no GPU")}}
	gone.MarkQuick()
	_ = Remember(&Run{Kernel: "7.3.0-72-omarchy", Results: []Result{result("webgl", Works, Measured, "WebGL 2.0 on Adreno X1-85")}}, start.Add(12*time.Minute))
	_ = Remember(gone, start.Add(13*time.Minute))
	if kept := Local()["webgl"]; kept.State != Broken {
		t.Errorf("the quick check's failure didn't replace a closer test's works: %+v", kept)
	}
	// What is sent says of each result how close it looked
	var sent struct {
		Results []struct{ Check, Depth string }
	}
	mixed := &Run{Results: []Result{result("gpu", Works, Measured, "adreno bound")}}
	mixed.MarkQuick()
	mixed.Set("webgpu", Works, Measured, "Chromium offers WebGPU on qualcomm adreno-7xx", "")
	if err := json.Unmarshal(mixed.JSON(), &sent); err != nil || len(sent.Results) != 2 || sent.Results[0].Depth != Look || sent.Results[1].Depth != Test {
		t.Errorf("depths sent: %+v, %v", sent, err)
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

func TestTheOwnersHandingAPartOverStandsForTwoWeeks(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC)
	if Handed("speakers", now) {
		t.Fatal("handed before anything was")
	}
	if err := Hand([]string{"speakers", "camera"}, now); err != nil {
		t.Fatal(err)
	}
	Hand([]string{"webgpu"}, now.Add(time.Hour))
	if !Handed("speakers", now.Add(13*24*time.Hour)) || !Handed("camera", now) || !Handed("webgpu", now.Add(2*time.Hour)) || Handed("wifi", now) {
		t.Error("what was handed over isn't remembered as it was")
	}
	if Handed("speakers", now.Add(15*24*time.Hour)) {
		t.Error("the owner's yes never runs out")
	}
}

func TestTheConversationStartedForAFolderIsKept(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, ok := ThreadFor("/home/owner/Work/hw-bringup"); ok {
		t.Fatal("a thread before any was started")
	}
	now := time.Now()
	KeepThread("/home/owner/Work/hw-bringup", Thread{Kind: "t3", ID: "mcp:351f", When: now})
	KeepThread("/home/owner/other", Thread{Kind: "t3", ID: "mcp:9", When: now})
	if kept, ok := ThreadFor("/home/owner/Work/hw-bringup"); !ok || kept.ID != "mcp:351f" || kept.Kind != "t3" {
		t.Errorf("kept %+v, %v", kept, ok)
	}
	// One that wasn't used for weeks is no longer gone on in
	KeepThread("/home/owner/old", Thread{Kind: "codex", ID: "01a1", When: now.Add(-30 * 24 * time.Hour)})
	if _, ok := ThreadFor("/home/owner/old"); ok {
		t.Error("a conversation from weeks ago is still the one to go on in")
	}

	// An agent in a terminal says which conversation it is in each time it calls hw-test, and
	// the parts the owner handed it are kept beside that
	dir := "/home/owner/hw-bringup"
	NoteWork(dir, []string{"speakers"}, now)
	if _, ok := ThreadFor(dir); ok {
		t.Error("a conversation before the agent named one")
	}
	NoteSession(dir, "codex", "01a1f404", now)
	if kept, ok := ThreadFor(dir); !ok || kept.Kind != "codex" || kept.ID != "01a1f404" || len(kept.Checks) != 1 || kept.Checks[0] != "speakers" {
		t.Errorf("the agent's own conversation: %+v, %v", kept, ok)
	}
	// A new conversation of the same agent takes its place
	NoteSession(dir, "codex", "01a1f999", now)
	if kept, _ := ThreadFor(dir); kept.ID != "01a1f999" || len(kept.Checks) != 1 {
		t.Errorf("a newer conversation: %+v", kept)
	}
	// The thread hw-test started in T3 Code stays the one kept, though the agent inside it
	// names its own, inner conversation
	NoteSession("/home/owner/Work/hw-bringup", "codex", "01a1inner", now)
	if kept, _ := ThreadFor("/home/owner/Work/hw-bringup"); kept.Kind != "t3" || kept.ID != "mcp:351f" {
		t.Errorf("the T3 Code thread was replaced: %+v", kept)
	}
}

// Each part has a snapshot of its own. On the Yoga an agent working on WebGPU was measured
// against the snapshot taken hours earlier for the speakers, and its block described the wrong
// change.
func TestEachPartHasASnapshotOfItsOwn(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	morning := time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC)
	run := &Run{Kernel: "7.3.0-72-omarchy"}
	run.Set("speakers", Partial, Measured, "left -47 dB, right not heard", "")
	if err := Remember(run, morning.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := TakeSnapshot(run, morning, "speakers", false); err != nil {
		t.Fatal(err)
	}
	// Two hours on, on the test image made for the speakers, the work on WebGPU starts
	later := morning.Add(2 * time.Hour)
	if _, err := TakeSnapshot(run, later, "webgpu", true); err != nil {
		t.Fatal(err)
	}
	speakers, _ := LoadSnapshot("speakers")
	webgpu, ok := LoadSnapshot("webgpu")
	if !ok || !webgpu.When.Equal(later) || !webgpu.TestImage || webgpu.Part != "webgpu" || !speakers.When.Equal(morning) || speakers.TestImage {
		t.Errorf("speakers %+v, webgpu %+v", speakers.When, webgpu)
	}
	if _, plain := LoadSnapshot(""); plain {
		t.Error("a snapshot without a part appeared")
	}
	if got := StandingParts(); len(got) != 2 || got[0] != "speakers" || got[1] != "webgpu" {
		t.Errorf("standing: %v", got)
	}
	// What was tested for the speakers before the WebGPU work began is no part of its change
	tested := &Run{Kernel: "7.3.0-72-omarchy"}
	tested.Set("speakers", Works, Measured, "left -47 dB, right -54 dB", "")
	_ = Remember(tested, later.Add(-time.Minute))
	graphics := &Run{Kernel: "7.3.0-72-omarchy"}
	graphics.Set("webgpu", Works, Measured, "Chrome offers WebGPU on qualcomm adreno-7xx", "")
	_ = Remember(graphics, later.Add(time.Minute))
	if changes := Changes(webgpu, Latest()); len(changes) != 1 || changes[0].Check != "webgpu" {
		t.Errorf("against the WebGPU snapshot: %+v", changes)
	}
	if changes := Changes(speakers, Latest()); len(changes) != 2 {
		t.Errorf("against the speakers snapshot: %+v", changes)
	}
	// One recorded, the other still stands
	if err := MarkSnapshotUsed("webgpu"); err != nil {
		t.Fatal(err)
	}
	if got := StandingParts(); len(got) != 1 || got[0] != "speakers" {
		t.Errorf("standing after one was recorded: %v", got)
	}
	for word, want := range map[string]bool{"speakers": true, "suspend-drain": true, "../etc": false, "": false, "Speakers": false, "a b": false} {
		if PartNamed(word) != want {
			t.Errorf("%q as a part's name: %v", word, !want)
		}
	}
}
