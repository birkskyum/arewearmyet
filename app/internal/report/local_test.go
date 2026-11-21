package report

import (
	"testing"
	"time"
)

func TestTheMachineKeepsItsLatestResultForEachCheck(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if len(Local()) != 0 {
		t.Fatal("results before any test")
	}
	monday := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	first := &Run{Kernel: "7.3.0-59-omarchy"}
	first.Set("wifi", Works, Measured, "wlan0 connected", "")
	first.Set("suspend", Partial, Owner, "slept and woke", "The second sleep ended in a restart.")
	first.Set("camera", Broken, Measured, "no sensor", "")
	first.Set("usb", Untested, Owner, "", "")
	if err := Remember(first, monday); err != nil {
		t.Fatal(err)
	}

	// The next day on a newer kernel, the camera is fixed, and a quick check counts one good sleep
	second := &Run{Kernel: "7.3.0-72-omarchy"}
	second.Set("camera", Works, Owner, "a 1920x1092 frame", "")
	second.Set("suspend", Works, Measured, "1 sleeps since boot", "")
	if err := Remember(second, monday.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	kept := Local()
	if len(kept) != 3 {
		t.Fatalf("kept %d checks: %+v", len(kept), kept)
	}
	if camera := kept["camera"]; camera.State != Works || camera.Kernel != "7.3.0-72-omarchy" || !camera.When.Equal(monday.Add(24*time.Hour)) {
		t.Errorf("camera: %+v", camera)
	}
	// What the owner saw fail stays until the owner tests it again
	if sleep := kept["suspend"]; sleep.State != Partial || sleep.Note != "The second sleep ended in a restart." || sleep.Kernel != "7.3.0-59-omarchy" {
		t.Errorf("suspend: %+v", sleep)
	}
	if kept["wifi"].State != Works {
		t.Errorf("wifi: %+v", kept["wifi"])
	}

	third := &Run{Kernel: "7.3.0-75-omarchy"}
	third.Set("suspend", Works, Owner, "slept and woke", "")
	Remember(third, monday.Add(48*time.Hour))
	if Local()["suspend"].State != Works {
		t.Error("the owner's own retest didn't replace the failure")
	}
}

func TestTheOwnersWordHoldsUntilItsTimeAndCanBeTakenBack(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Date(2026, 11, 21, 1, 0, 0, 0, time.UTC)
	if _, held := Trusted(now); held {
		t.Fatal("trusted with nothing said")
	}
	until := now.Add(8 * time.Hour)
	if err := SetTrust(until); err != nil {
		t.Fatal(err)
	}
	if got, held := Trusted(now.Add(7 * time.Hour)); !held || !got.Equal(until) {
		t.Errorf("an hour before its time: %v, %v", got, held)
	}
	if _, held := Trusted(until); held {
		t.Error("still trusted at its time")
	}
	_ = SetTrust(until)
	EndTrust()
	if _, held := Trusted(now); held {
		t.Error("still trusted after it was taken back")
	}
}
