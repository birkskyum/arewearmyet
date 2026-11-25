package report

import "testing"

func TestARestartDuringSleepIsFoundAndKeepsTheAnswers(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, found := LoadPending(); found {
		t.Fatal("found a test that was never saved")
	}

	before := &Run{}
	before.Set("wifi", Works, Measured, "wlan0 connected", "")
	before.Set("speakers", Partial, Owner, "tones heard", "Only the left speaker plays.")
	before.Set("keyboard", Works, Measured, "60 keys registered", "")
	if err := SavePending("Sleep", "s2idle [deep]", before); err != nil {
		t.Fatal(err)
	}
	pending, found := LoadPending()
	if !found || len(pending.Results) != 3 {
		t.Fatalf("found=%v, %+v", found, pending)
	}
	// The same boot means the test was only stopped, and another boot means the computer restarted
	if pending.Restarted(pending.Boot) || !(Pending{Step: "Sleep", Boot: "a"}).Restarted("b") || (Pending{Step: "Camera"}).Restarted("b") {
		t.Error("restart detection is off")
	}

	// The next run measures afresh. Wi-Fi is off this time, and it didn't get to the keyboard
	after := &Run{}
	after.Set("wifi", Broken, Measured, "no Wi-Fi interface", "")
	pending.Restore(after)
	pending.Crashed(after, "suspend")
	if after.State("wifi") != Broken {
		t.Error("an old measurement replaced a new one")
	}
	if speakers, _ := after.Get("speakers"); speakers.State != Partial || speakers.Note != "Only the left speaker plays." {
		t.Errorf("the owner's answer was lost: %+v", speakers)
	}
	if after.State("keyboard") != Works {
		t.Error("a result the new run didn't measure again was lost")
	}
	if sleep, _ := after.Get("suspend"); sleep.State != Broken || sleep.How != Measured {
		t.Errorf("sleep: %+v", sleep)
	}

	// Hibernating marks the saved test, so a fresh start after it is known for what it is, and
	// coming back takes the mark off again
	if MarkBegan(true) != nil {
		t.Fatal("couldn't mark the saved test")
	}
	if marked, _ := LoadPending(); !marked.Began || len(marked.Results) != 3 {
		t.Errorf("marked: %+v", marked)
	}
	MarkBegan(false)
	if unmarked, _ := LoadPending(); unmarked.Began {
		t.Error("the mark stayed after coming back")
	}

	ClearPending()
	if _, found := LoadPending(); found {
		t.Error("the saved test wasn't cleared")
	}
	// With no saved test there is nothing to mark
	MarkBegan(true)
	if _, found := LoadPending(); found {
		t.Error("marking made a saved test")
	}
}

func TestACrashInAnyStepIsThatStepsResult(t *testing.T) {
	run := &Run{}
	Pending{Step: "Camera"}.Crashed(run, "camera")
	camera, _ := run.Get("camera")
	if camera.State != Broken || camera.Evidence != "the computer restarted by itself during the test's Camera step, before the step finished" {
		t.Errorf("camera: %+v", camera)
	}
}
