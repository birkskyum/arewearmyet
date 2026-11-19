package probe

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// A Yoga Slim 7x booted with its GPU off, copied by scripts/probe-fixture.sh
const yoga = "../../../test/probe/fixtures/yoga-slim-7x-gpu-off"

// fixture copies a machine to a folder of the test's own, changes it and measures it.
func fixture(t *testing.T, change func(root string)) (*report.Run, Facts) {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir(yoga, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(yoga, path)
		target := filepath.Join(root, relative)
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, 0o755)
		case entry.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if change != nil {
		change(root)
	}
	run, facts, err := Quick(machine.Fixture(root))
	if err != nil {
		t.Fatal(err)
	}
	return run, facts
}

func write(t *testing.T, root, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func expect(t *testing.T, run *report.Run, want map[string]string) {
	t.Helper()
	for check, state := range want {
		if got := run.State(check); got != state {
			result, _ := run.Get(check)
			t.Errorf("%s: got %q, want %q (%s)", check, got, state, result.Evidence)
		}
	}
}

func TestNamesTheMachineWithoutAnythingPersonal(t *testing.T) {
	run, facts := fixture(t, nil)
	if run.Compatible != "lenovo,yoga-slim7x" || run.Kernel != "7.3.0-59-omarchy" || run.OS != "Omarchy" {
		t.Errorf("got %q, %q, %q", run.Compatible, run.Kernel, run.OS)
	}
	want := map[string]string{"maker": "LENOVO", "product": "83ED", "family": "Yoga Slim 7 14Q8X9", "sku": "LENOVO_MT_83ED_BU_idea_FM_Yoga Slim 7 14Q8X9", "board": "LNVNB161216", "bios": "NHCN62WW"}
	for key, value := range want {
		if run.Hardware[key] != value {
			t.Errorf("hardware %s: got %q, want %q", key, run.Hardware[key], value)
		}
	}
	if run.Packages["linux-omarchy"] != "7.3-59" || run.Packages["omarchy"] != "4.0.0.r7344.ga001044-1" {
		t.Errorf("packages: %v", run.Packages)
	}
	if len(run.Chids) != 18 || run.Chids[0] != "0:5315f3e1-1fa9-5592-bf05-75e8124d1f8a" || run.Chids[17] != "ext2:89a6ea33-85de-52aa-ad21-64eba70c0823" {
		t.Errorf("hardware IDs: %v", run.Chids)
	}
	if regexp.MustCompile(`(?i)([0-9a-f]{2}:){5}[0-9a-f]{2}`).Match(run.JSON()) {
		t.Error("the run holds an address")
	}
	if !facts.Laptop || facts.Keyboard == "" || facts.Touchpad == "" || facts.Touchscreen == "" || facts.Lid == "" || facts.Wifi != "wlan0" || facts.Bluetooth != "hci0" || facts.Camera != "ov02c10 sensor" || facts.Backlight == "" || facts.SoundCard == "" {
		t.Errorf("facts: %+v", facts)
	}
}

func TestMeasuresAYogaBootedWithItsGPUOff(t *testing.T) {
	run, _ := fixture(t, nil)
	expect(t, run, map[string]string{
		"display": report.Works, "gpu": report.Broken, "wifi": report.Works, "wifi-address": report.Works, "bluetooth": report.Works,
		"adsp": report.Works, "cdsp": report.Works, "battery": report.Works, "charging": report.Works, "hibernate": report.Broken,
		// What needs a person is left out, even though the hardware is there
		"keyboard": "", "touchpad": "", "speakers": "", "microphones": "", "camera": "", "backlight": "", "lid": "",
	})
	for _, result := range run.Results {
		if result.How != report.Measured {
			t.Errorf("%s is %s, and a quick check only measures", result.Check, result.How)
		}
	}
	for check, text := range map[string]string{"display": "eDP-1 connected, 2944x1840", "adsp": "qcadsp8380.mbn", "charging": "PD"} {
		if result, _ := run.Get(check); !strings.Contains(result.Evidence, text) {
			t.Errorf("%s evidence %q lacks %q", check, result.Evidence, text)
		}
	}
}

func TestSeesTheGPU(t *testing.T) {
	bound := func(root string) {
		os.MkdirAll(filepath.Join(root, "sys/bus/platform/drivers/adreno/3d00000.gpu"), 0o755)
		os.WriteFile(filepath.Join(root, "cmd/klog"), nil, 0o644)
	}
	run, _ := fixture(t, bound)
	expect(t, run, map[string]string{"gpu": report.Works})

	// Software rendering forced in /etc/environment makes it only partly working
	run, _ = fixture(t, func(root string) {
		bound(root)
		write(t, root, "etc/environment", "LIBGL_ALWAYS_SOFTWARE=1\n")
	})
	expect(t, run, map[string]string{"gpu": report.Partial})
}

func TestReportsMissingAndFailedHardwareAsBroken(t *testing.T) {
	run, _ := fixture(t, func(root string) {
		os.RemoveAll(filepath.Join(root, "sys/class/net/wlan0"))
		os.RemoveAll(filepath.Join(root, "sys/class/bluetooth"))
		os.Remove(filepath.Join(root, "proc/asound/cards"))
		processors, _ := filepath.Glob(filepath.Join(root, "sys/class/remoteproc/remoteproc*/state"))
		for _, state := range processors {
			os.WriteFile(state, []byte("crashed\n"), 0o644)
		}
	})
	expect(t, run, map[string]string{"wifi": report.Broken, "wifi-address": "", "bluetooth": report.Broken, "speakers": report.Broken, "microphones": report.Broken, "adsp": report.Broken})
}

func TestTellsARandomWifiAddressFromAFixedOne(t *testing.T) {
	run, _ := fixture(t, func(root string) { write(t, root, "sys/class/net/wlan0/addr_assign_type", "1\n") })
	expect(t, run, map[string]string{"wifi-address": report.Broken})
}

func TestCallsChargingAtFiveVoltsPartlyWorking(t *testing.T) {
	run, _ := fixture(t, func(root string) {
		supply := "sys/class/power_supply/ucsi-source-psy-pmic_glink.ucsi.02"
		write(t, root, supply+"/usb_type", "[C] PD PD_PPS\n")
		write(t, root, supply+"/voltage_now", "5000000\n")
	})
	expect(t, run, map[string]string{"charging": report.Partial})
}

func TestCountsSleepsSinceBoot(t *testing.T) {
	run, _ := fixture(t, func(root string) { write(t, root, "sys/power/suspend_stats/success", "4\n") })
	expect(t, run, map[string]string{"suspend": report.Works})
	run, _ = fixture(t, func(root string) {
		write(t, root, "sys/power/suspend_stats/success", "4\n")
		write(t, root, "sys/power/suspend_stats/fail", "2\n")
	})
	expect(t, run, map[string]string{"suspend": report.Partial})
}

func TestExpectsNoBatteryLidOrCameraOnADesktop(t *testing.T) {
	run, facts := fixture(t, func(root string) {
		write(t, root, "sys/class/dmi/id/chassis_type", "3\n")
		os.RemoveAll(filepath.Join(root, "sys/class/power_supply"))
		os.RemoveAll(filepath.Join(root, "sys/class/video4linux"))
		os.RemoveAll(filepath.Join(root, "sys/class/backlight"))
	})
	if facts.Laptop {
		t.Error("a desktop was taken for a laptop")
	}
	expect(t, run, map[string]string{"battery": "", "camera": "", "backlight": "", "video-decode": report.Broken})
}

func TestFindsTheCameraByItsSensor(t *testing.T) {
	run, _ := fixture(t, func(root string) {
		subdevs, _ := filepath.Glob(filepath.Join(root, "sys/class/video4linux/v4l-subdev*"))
		for _, subdev := range subdevs {
			os.RemoveAll(subdev)
		}
	})
	expect(t, run, map[string]string{"camera": report.Broken})
	if result, _ := run.Get("camera"); !strings.Contains(result.Evidence, "no sensor") {
		t.Errorf("camera evidence: %q", result.Evidence)
	}
}

func TestRefusesAMachineThatIsNotARM(t *testing.T) {
	root := t.TempDir()
	write(t, root, "cmd/uname-m", "x86_64\n")
	if _, _, err := Quick(machine.Fixture(root)); err == nil {
		t.Error("an x86 machine was measured")
	}
}

func TestARadioTheOwnerSwitchedOffIsNotAFault(t *testing.T) {
	off := func(root string) { write(t, root, "cmd/bluetoothctl-show", "\tPowered: no\n") }
	run, _ := fixture(t, off)
	expect(t, run, map[string]string{"bluetooth": report.Partial})
	run, _ = fixture(t, func(root string) {
		off(root)
		write(t, root, "sys/class/rfkill/rfkill0/soft", "1\n")
	})
	expect(t, run, map[string]string{"bluetooth": ""})
}
