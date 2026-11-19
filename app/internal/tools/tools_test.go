package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ppm is a whole frame of one grey.
func ppm(grey byte) []byte {
	pixels := make([]byte, 64*36*3)
	for i := range pixels {
		pixels[i] = grey
	}
	return append([]byte("P6\n64 36\n255\n"), pixels...)
}

func TestBrightnessReadsAWholeFrameAndRefusesATornOne(t *testing.T) {
	if mean, whole := brightness(ppm(77)); !whole || mean != 77 {
		t.Errorf("a whole frame: %v, %v", mean, whole)
	}
	frame := ppm(77)
	for _, torn := range [][]byte{frame[:len(frame)-10], frame[:20], nil, []byte("P6\n")} {
		if _, whole := brightness(torn); whole {
			t.Errorf("a frame of %d bytes counted as whole", len(torn))
		}
	}
}

func TestThePictureHasSettledOnlyWhenItStoppedChangingForASecond(t *testing.T) {
	series := func(means ...float64) []sample {
		var samples []sample
		for i, mean := range means {
			samples = append(samples, sample{time.Duration(i) * 150 * time.Millisecond, mean})
		}
		return samples
	}
	// As measured on a Yoga Slim 7x from a cold start: about five of 255 a second, for seconds
	var ramp []float64
	for i := 0; i < 40; i++ {
		ramp = append(ramp, 17+float64(i)*0.75)
	}
	for i := 4; i <= len(ramp); i++ {
		if settled(series(ramp[:i]...)) {
			t.Fatalf("a picture still brightening counted as settled after %d samples", i)
		}
	}
	// Dark first frames, then steady: settled once a second of steady frames is in
	steady := []float64{33, 71, 77, 77, 77, 76, 77, 77, 77, 77, 77}
	if settled(series(steady[:8]...)) {
		t.Error("settled before a second of steady frames")
	}
	if !settled(series(steady...)) {
		t.Error("a steady picture never settled")
	}
	// The ramp's end: steady after brightening
	if !settled(series(append(ramp, 47, 47, 47, 46, 47, 47, 47, 47, 47)...)) {
		t.Error("a picture that stopped brightening never settled")
	}
}

// fakeCam puts a cam on the PATH that writes a frame every 50 ms as its script says, and stops
// when interrupted.
func fakeCam(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	body := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in --file=*) pattern=${a#--file=};; esac; done\n" + script
	if err := os.WriteFile(filepath.Join(bin, "cam"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

func TestFrameWaitsForTheCameraToSettleAndStopsItThen(t *testing.T) {
	dir := t.TempDir()
	for grey := 0; grey < 256; grey++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("grey-%d", grey)), ppm(byte(grey)), 0o600)
	}
	// Brightens by two a frame for forty frames, then holds at 100
	fakeCam(t, `trap 'exit 0' INT
i=0
while [ $i -lt 2000 ]; do
  grey=$((20 + i * 2)); [ $grey -gt 100 ] && grey=100
  cp "`+dir+`/grey-$grey" "$(printf '%s' "$pattern" | sed "s/#/cam0-stream0-$(printf %06d $i)/")"
  i=$((i + 1)); sleep 0.05
done
`)
	out := t.TempDir()
	began := time.Now()
	shot, err := Frame(out)
	if err != nil {
		t.Fatal(err)
	}
	mean, whole := brightness(shot.PPM)
	if !whole || mean != 100 || !shot.Settled {
		t.Errorf("brightness %v, whole %v, settled %v", mean, whole, shot.Settled)
	}
	// Forty frames of brightening are two seconds, and a second of steady ones follows
	if took := time.Since(began); took < 2500*time.Millisecond || took > 8*time.Second {
		t.Errorf("it took %v", took)
	}
	if left, _ := filepath.Glob(filepath.Join(out, "*")); len(left) != 0 {
		t.Errorf("frames were left behind: %v", left)
	}
}

func TestFrameSaysWhenAnotherProgramHoldsTheCamera(t *testing.T) {
	fakeCam(t, "echo \"ERROR MediaDevice /dev/media0[qcom-camss]: Failed to setup link 'msm_csiphy4'[1] -> 'msm_csid0'[0]: Device or resource busy\" >&2\necho 'Camera 1 not found' >&2\nexit 1\n")
	if _, err := Frame(t.TempDir()); !errors.Is(err, ErrCameraBusy) {
		t.Errorf("a busy camera gave %v", err)
	}
	fakeCam(t, "echo 'Camera 1 not found' >&2\nexit 1\n")
	if _, err := Frame(t.TempDir()); err == nil || errors.Is(err, ErrCameraBusy) {
		t.Errorf("a missing camera gave %v", err)
	}
}

func TestTheAgentsLauncherIsWaitedForWhereTheCallerEndsAtOnce(t *testing.T) {
	dir := t.TempDir()
	launcher := filepath.Join(dir, "launcher")
	os.WriteFile(launcher, []byte("#!/bin/sh\nsleep 1\necho \"$1\" > "+dir+"/opened\n"), 0o755)
	t.Setenv("HW_TEST_AGENT", launcher)
	opened := func() bool { _, err := os.Stat(filepath.Join(dir, "opened")); return err == nil }
	// From the test's own screen nothing is waited for
	if err := Agent(dir, "read the brief"); err != nil || opened() {
		t.Fatalf("Agent: %v, opened at once %v", err, opened())
	}
	time.Sleep(1500 * time.Millisecond)
	os.Remove(filepath.Join(dir, "opened"))
	// At login the launcher has done its part by the time this returns
	if err := AgentHandedOver(dir, "read the brief"); err != nil || !opened() {
		t.Errorf("AgentHandedOver: %v, opened %v", err, opened())
	}
	// And a launcher that fails is reported
	os.WriteFile(launcher, []byte("#!/bin/sh\nexit 3\n"), 0o755)
	if err := AgentHandedOver(dir, "read the brief"); err == nil {
		t.Error("a launcher that failed wasn't reported")
	}
}
