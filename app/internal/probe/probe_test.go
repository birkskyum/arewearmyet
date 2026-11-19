package probe

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

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

func TestRefusesASystemItCannotRead(t *testing.T) {
	root := t.TempDir()
	write(t, root, "cmd/uname-m", "aarch64\n")
	if _, _, err := Quick(machine.Fixture(root)); err == nil {
		t.Error("a system without /sys was measured, and would have shown every device as missing")
	}
}

// A driver whose hardware is broken can leave a read of its file hanging for good. A pipe
// nobody writes to does the same.
func TestAFileThatNeverAnswersDoesNotHoldUpTheCheck(t *testing.T) {
	started := time.Now()
	run, _ := fixture(t, func(root string) {
		supply := filepath.Join(root, "sys/class/power_supply/qcom-battmgr-bat")
		for _, file := range []string{"energy_now", "status"} {
			os.Remove(filepath.Join(supply, file))
			if err := syscall.Mkfifo(filepath.Join(supply, file), 0o644); err != nil {
				t.Skip("no named pipes here:", err)
			}
		}
	})
	if took := time.Since(started); took > 15*time.Second {
		t.Errorf("the check took %s", took)
	}
	// The battery couldn't be read, and everything else still was
	expect(t, run, map[string]string{"battery": report.Broken, "wifi": report.Works, "adsp": report.Works, "charging": report.Works})
}

// Odd values where numbers and names are expected, as half-working drivers give.
func TestOddValuesInSysDoNotStopTheCheck(t *testing.T) {
	run, _ := fixture(t, func(root string) {
		write(t, root, "sys/power/suspend_stats/success", "many\n")
		write(t, root, "sys/class/power_supply/qcom-battmgr-bat/energy_full", "0\n")
		write(t, root, "sys/class/net/wlan0/addr_assign_type", "\x00\xff\n")
		write(t, root, "sys/class/dmi/id/chassis_type", "")
		write(t, root, "cmd/chids", "garbage\n\n   0 MFPS not-an-id\n")
		write(t, root, "cmd/packages", "linux-omarchy\nmesa 1 2 3\n")
		inputs, _ := filepath.Glob(filepath.Join(root, "sys/class/input/event*/device/capabilities/sw"))
		for _, file := range inputs {
			os.WriteFile(file, []byte("zz qq\n"), 0o644)
		}
		write(t, root, "sys/class/remoteproc/remoteproc0/state", "")
	})
	if len(run.Chids) != 0 || len(run.Packages) > 9 {
		t.Errorf("chids %v, packages %v", run.Chids, run.Packages)
	}
	expect(t, run, map[string]string{"display": report.Works, "wifi": report.Works, "suspend": ""})
}

// nvidia turns the copied machine into one of NVIDIA's ARM PCs: started through ACPI, so with
// no device tree and none of Qualcomm's devices, and with NVIDIA's GPU on the PCI bus.
func nvidia(t *testing.T, root, device, driver string) {
	t.Helper()
	for _, gone := range []string{"proc/device-tree", "sys/bus/platform/drivers/adreno", "sys/class/video4linux", "sys/class/remoteproc"} {
		if err := os.RemoveAll(filepath.Join(root, gone)); err != nil {
			t.Fatal(err)
		}
	}
	card := "sys/bus/pci/devices/0000:01:00.0"
	write(t, root, card+"/vendor", "0x10de\n")
	write(t, root, card+"/class", "0x030000\n")
	write(t, root, card+"/device", device+"\n")
	if driver != "" {
		if err := os.MkdirAll(filepath.Join(root, "sys/bus/pci/drivers", driver), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../drivers/"+driver, filepath.Join(root, card, "driver")); err != nil {
			t.Fatal(err)
		}
	}
	write(t, root, "sys/class/dmi/id/sys_vendor", "ASUSTeK COMPUTER INC.\n")
	write(t, root, "sys/class/dmi/id/product_name", "ProArt P16 H7607BA\n")
}

// An NVIDIA laptop is known for what it is although nothing on it says "NVIDIA" in its name,
// and it isn't faulted for lacking a Snapdragon's parts. Before this it had no family, and the
// quick check called its video decoding broken for want of Qualcomm's decoder.
func TestAnNvidiaLaptopIsKnownAndNotMeasuredAsASnapdragon(t *testing.T) {
	var root string
	run, _ := fixture(t, func(r string) { root = r; nvidia(t, r, "0x2e06", "nvidia") })
	sys := machine.Fixture(root)
	if got := Platform(sys); got != "n1x" {
		t.Errorf("platform %q", got)
	}
	if got := Family(sys); got != Nvidia {
		t.Errorf("family %q", got)
	}
	if gpu, _ := run.Get("gpu"); gpu.State != report.Works || gpu.Evidence != "NVIDIA GPU (10de:2e06) driven by nvidia" {
		t.Errorf("gpu: %+v", gpu)
	}
	for _, check := range []string{"video-decode", "adsp", "cdsp"} {
		if result, found := run.Get(check); found {
			t.Errorf("%s was measured on an NVIDIA machine: %+v", check, result)
		}
	}

	// The GPU with no driver bound is a fault, said as one
	run, _ = fixture(t, func(r string) { nvidia(t, r, "0x2e06", "") })
	if gpu, _ := run.Get("gpu"); gpu.State != report.Broken || !strings.Contains(gpu.Evidence, "no driver bound") {
		t.Errorf("gpu without a driver: %+v", gpu)
	}

	// A GB10 desktop by its own GPU, and an N1x whose GPU isn't on the bus by its I2C controllers
	fixture(t, func(r string) { root = r; nvidia(t, r, "0x2e12", "nvidia") })
	if got := Platform(machine.Fixture(root)); got != "gb10" {
		t.Errorf("a GB10 desktop: %q", got)
	}
	fixture(t, func(r string) {
		root = r
		nvidia(t, r, "0x2e06", "")
		if err := os.RemoveAll(filepath.Join(r, "sys/bus/pci")); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(r, "sys/bus/acpi/devices/NVDA0200:00"), 0o755); err != nil {
			t.Fatal(err)
		}
		// With nothing else to go by, the processor's maker would have named it a Snapdragon
		write(t, r, "proc/cpuinfo", "CPU implementer\t: 0x41\n")
	})
	if got := Platform(machine.Fixture(root)); got != "n1x" {
		t.Errorf("an N1x known by its I2C controllers: %q", got)
	}

	// Omarchy's own detector is believed where it answers
	asked := machine.Fixture(root)
	inner := asked.Command
	asked.Command = func(name string, args ...string) (string, error) {
		if name == "omarchy-hw-platform" {
			return "aarch64-gb10\n", nil
		}
		return inner(name, args...)
	}
	if got := Platform(asked); got != "gb10" {
		t.Errorf("Omarchy's detector said gb10, and the platform is %q", got)
	}
}

// The Yoga is still a Snapdragon PC that came with Windows, and still faulted when its video
// decoder is missing.
func TestASnapdragonIsStillMeasuredAsOne(t *testing.T) {
	var root string
	run, _ := fixture(t, func(r string) {
		root = r
		if err := os.RemoveAll(filepath.Join(r, "sys/class/video4linux")); err != nil {
			t.Fatal(err)
		}
	})
	sys := machine.Fixture(root)
	if Platform(sys) != "qualcomm" || Family(sys) != DragonMsft {
		t.Errorf("platform %q, family %q", Platform(sys), Family(sys))
	}
	if video, _ := run.Get("video-decode"); video.State != report.Broken {
		t.Errorf("video decoding without a decoder: %+v", video)
	}
}

// socket gives the machine a wired network interface on the named bus, with or without a cable in it.
func socket(t *testing.T, root, name, bus, carrier string) {
	t.Helper()
	net := "sys/class/net/" + name
	write(t, root, net+"/type", "1\n")
	write(t, root, net+"/carrier", carrier+"\n")
	write(t, root, net+"/speed", "2500\n")
	write(t, root, "sys/bus/"+bus+"/.keep", "")
	write(t, root, "sys/bus/"+bus+"/drivers/r8169/.keep", "")
	write(t, root, "sys/devices/"+name+"/.keep", "")
	for link, target := range map[string]string{"sys/devices/" + name + "/subsystem": "../../bus/" + bus, "sys/devices/" + name + "/driver": "../../bus/" + bus + "/drivers/r8169", net + "/device": "../../../devices/" + name} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheNetworkSocketIsADesktopsOwnAndALaptopsOnlyWhenBuiltIn(t *testing.T) {
	desktop := func(root string) { write(t, root, "sys/class/dmi/id/chassis_type", "35\n") }

	// A mini PC with a cable in its socket
	run, facts := fixture(t, func(root string) { desktop(root); socket(t, root, "enp1s0", "pci", "1") })
	if result, _ := run.Get("ethernet"); facts.Ethernet != "enp1s0" || result.State != report.Works || result.Evidence != "enp1s0 (r8169) connected at 2500 Mb/s" {
		t.Errorf("with a cable: %q, %+v", facts.Ethernet, result)
	}
	// Without a cable the socket is known and nothing is said about it yet
	run, facts = fixture(t, func(root string) { desktop(root); socket(t, root, "enp1s0", "pci", "0") })
	if facts.Ethernet != "enp1s0" || run.State("ethernet") != "" {
		t.Errorf("without a cable: %q, %q", facts.Ethernet, run.State("ethernet"))
	}
	// Of two sockets, the one with the cable is the one to test
	_, facts = fixture(t, func(root string) {
		desktop(root)
		socket(t, root, "enp1s0", "pci", "0")
		socket(t, root, "enp2s0", "pci", "1")
	})
	if facts.Ethernet != "enp2s0" {
		t.Errorf("of two sockets it took %q", facts.Ethernet)
	}
	// A desktop's socket can be wired through USB inside
	run, _ = fixture(t, func(root string) { desktop(root); socket(t, root, "enu1", "usb", "1") })
	expect(t, run, map[string]string{"ethernet": report.Works})
	// A desktop with no wired interface at all is missing its driver
	run, _ = fixture(t, desktop)
	expect(t, run, map[string]string{"ethernet": report.Broken})

	// A laptop is expected to have none, and an adapter or dock on USB isn't its own
	run, facts = fixture(t, nil)
	if facts.Ethernet != "" || run.State("ethernet") != "" {
		t.Errorf("a laptop without a socket: %q, %q", facts.Ethernet, run.State("ethernet"))
	}
	run, facts = fixture(t, func(root string) { socket(t, root, "enu1", "usb", "1") })
	if facts.Ethernet != "" || run.State("ethernet") != "" {
		t.Errorf("a laptop with an adapter: %q, %q", facts.Ethernet, run.State("ethernet"))
	}
	// A socket of the laptop's own counts
	run, _ = fixture(t, func(root string) { socket(t, root, "enp1s0", "pci", "1") })
	expect(t, run, map[string]string{"ethernet": report.Works})
}
