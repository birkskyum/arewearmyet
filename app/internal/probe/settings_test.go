package probe

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// packaged answers pacman and grep the way a computer does where the given files belong to
// packages and Omarchy's scripts name the given settings files.
func packaged(sys machine.System, held, omarchys []string, check string) machine.System {
	fixture := sys.Command
	sys.Command = func(name string, args ...string) (string, error) {
		switch {
		case name == "pacman" && args[0] == "-Qqo" && strings.HasPrefix(args[1], "/etc/modprobe.d"):
			return "error: No package owns /etc/wireplumber\nkmod\nlinux-firmware-qcom\nomarchy-settings\n", os.ErrInvalid
		case name == "pacman" && args[0] == "-Qql":
			if got := strings.Join(args[1:], " "); got != "kmod linux-firmware-qcom omarchy-settings" {
				return "", os.ErrNotExist
			}
			return "/etc/modprobe.d/\n" + strings.Join(held, "\n") + "\n", nil
		case name == "pacman" && args[0] == "-Qkk":
			return check, os.ErrInvalid
		case name == "pacman" && args[0] != "-Q":
			return "", os.ErrNotExist
		case name == "grep":
			return strings.Join(omarchys, "\n") + "\n", os.ErrInvalid
		case name == "modinfo":
			return "", os.ErrNotExist
		}
		return fixture(name, args...)
	}
	return sys
}

const ownersRules = `# A key the owner has, a speaker they own and a keyboard
SUBSYSTEM=="usb", ATTRS{idVendor}=="1050", ATTRS{idProduct}=="0407", \
  ATTR{power/control}="on", RUN+="/home/owner/bin/fix %k"
ACTION=="add", SUBSYSTEM=="input", ATTRS{name}=="KEF LSX", ENV{LIBINPUT_IGNORE_DEVICE}="1", MODE="0660"
KERNEL=="hidraw*", TAG+="uaccess", ATTR{serial}="0123456789abcdef0123"
`

const ownersSound = `monitor.alsa.rules = [
  {
    matches = [ { node.name = "~alsa_output.usb-KEF_LSX*" } ]
    actions = { update-props = { session.suspend-timeout-seconds = 0, api.alsa.headroom = 1024 } }
  }
]
`

func TestRulesSayWhatTheyDoAndNotToWhat(t *testing.T) {
	got := Rules(ownersRules)
	want := []string{"usb sets power/control=on", "usb runs", "input env LIBINPUT_IGNORE_DEVICE", "input permissions", "any permissions", "any sets serial"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if said := report.Rule(got[0]) + ", " + report.Rule(got[1]) + ", " + report.Rule(got[4]); said != "a device rule for usb that sets power/control=on, a device rule for usb that runs a program, a device rule that changes who may use it" {
		t.Errorf("in words: %s", said)
	}
}

func TestSoundSettingsAreNamedWithoutTheDevice(t *testing.T) {
	got := SoundSettings(ownersSound)
	if want := []string{"api.alsa.headroom", "session.suspend-timeout-seconds"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ownersComputer is the fixture with settings and firmware as an owner who got parts working
// leaves them, beside Omarchy's own.
func ownersComputer(t *testing.T) (string, []string) {
	root := copied(t)
	write(t, root, "proc/cmdline", stockCmdline)
	write(t, root, "proc/modules", "phy_qcom_qmp_ufs 77824 0 - Live 0x0000000000000000\n")
	// Omarchy's, in a package, written by its scripts, and left by an earlier version
	write(t, root, "etc/modprobe.d/omarchy-usb-autosuspend.conf", "options usbcore autosuspend=-1\n")
	write(t, root, "etc/modprobe.d/qualcomm-adsp-nofw.conf", "blacklist qcom_q6v5_pas\n")
	write(t, root, "etc/modprobe.d/hid_apple.conf", "options hid_apple fnmode=2\n")
	write(t, root, "etc/mkinitcpio.conf.d/qcom-firmware.conf", "FILES+=(/usr/lib/firmware/qcom/x1e80100/LENOVO/83ED/qcdxkmsuc8380.mbn)\n")
	write(t, root, "usr/lib/firmware/qcom/a660_sqe.fw.zst", "x")
	write(t, root, "usr/lib/firmware/updates/qcom/x1e80100/LENOVO/83ED/qcdxkmsuc8380.mbn", "x")
	write(t, root, "var/lib/omarchy/qcom-firmware/manifest", "qcom/x1e80100/LENOVO/83ED/qcdxkmsuc8380.mbn 5e884898da28 /mnt/windows/qcdxkmsuc8380.mbn\n")
	write(t, root, "usr/share/omarchy/install/hardware/qualcomm/firmware.sh", "cat >/etc/modprobe.d/qualcomm-adsp-nofw.conf\n")
	// The owner's
	write(t, root, "etc/modprobe.d/my-fix.conf", "# for the speakers\noptions snd_soc_x1e80100 i2s_mode=1 serial=0123456789abcdef0123\nblacklist qcom_spmi_temp_alarm\ninstall pcspkr /bin/false\n")
	write(t, root, "etc/modules-load.d/kef-speakers.conf", "scmi-cpufreq\n")
	// A copy kept beside a settings file isn't in use
	write(t, root, "etc/modules-load.d/kef-speakers.conf.bak", "pcspkr\n")
	write(t, root, "etc/mkinitcpio.conf.d/mine.conf", "MODULES+=(panel_edp msm?)\n")
	write(t, root, "etc/udev/rules.d/99-yubikey.rules", ownersRules)
	write(t, root, "etc/wireplumber/wireplumber.conf.d/kef-lsx-no-suspend.conf", ownersSound)
	write(t, root, "usr/lib/firmware/qcom/x1e80100/LENOVO/83ED/qcadsp8380.mbn", "x")
	write(t, root, "usr/share/alsa/ucm2/Qualcomm/x1e80100/mine.conf", "x")
	return root, []string{"/etc/modprobe.d/omarchy-usb-autosuspend.conf", "/usr/lib/firmware/qcom/a660_sqe.fw.zst"}
}

func TestSetupReadsTheOwnersSettingsFiles(t *testing.T) {
	root, held := ownersComputer(t)
	check := "warning: alsa-ucm-conf: /usr/share/alsa/ucm2/x.conf (Modification time mismatch)\nalsa-ucm-conf: 1223 total files, 1 altered file\nwireplumber: 327 total files, 0 altered files\nerror: package 'libcamera' was not found\n"
	sys := packaged(machine.Fixture(root), held, []string{"qualcomm-adsp-nofw.conf", "qcom-firmware.conf"}, check)
	run, _, err := Quick(sys)
	if err != nil {
		t.Fatal(err)
	}
	want := &report.Setup{
		ModuleOptions: []string{"snd_soc_x1e80100 i2s_mode=1", "snd_soc_x1e80100 serial=…"},
		Blocked:       []string{"qcom_spmi_temp_alarm", "pcspkr"},
		Loaded:        []string{"scmi-cpufreq", "panel_edp", "msm"},
		Rules:         []string{"usb sets power/control=on", "usb runs", "input env LIBINPUT_IGNORE_DEVICE", "input permissions", "any permissions", "any sets serial"},
		Files:         []string{"firmware/qcom/x1e80100/LENOVO/83ED/qcadsp8380.mbn", "alsa/ucm2/Qualcomm/x1e80100/mine.conf"},
		Altered:       []string{"alsa-ucm-conf"},
		Sound:         []string{"api.alsa.headroom", "session.suspend-timeout-seconds"},
	}
	if !reflect.DeepEqual(run.Setup, want) {
		t.Fatalf("got  %+v\nwant %+v", run.Setup, want)
	}
	// A file's name, the devices a rule picks out and the program it runs say what the owner owns
	sent := strings.ToLower(string(run.JSON()))
	for _, private := range []string{"kef", "yubikey", "1050", "0407", "/home/owner", "0123456789abcdef", "my-fix", "hid_apple", "qcom_q6v5_pas", "qcdxkmsuc8380"} {
		if strings.Contains(sent, private) {
			t.Errorf("the run sends %q", private)
		}
	}
}

// Where Omarchy's scripts aren't on the computer, its own settings files can't be told from the
// owner's, so none are read. What a package would hold is still told apart.
func TestSettingsFilesAreLeftAloneWithoutOmarchysScripts(t *testing.T) {
	root, held := ownersComputer(t)
	if err := os.RemoveAll(root + "/usr/share/omarchy"); err != nil {
		t.Fatal(err)
	}
	run, _, err := Quick(packaged(machine.Fixture(root), held, nil, ""))
	if err != nil {
		t.Fatal(err)
	}
	want := &report.Setup{Files: []string{"firmware/qcom/x1e80100/LENOVO/83ED/qcadsp8380.mbn", "alsa/ucm2/Qualcomm/x1e80100/mine.conf"}}
	if !reflect.DeepEqual(run.Setup, want) {
		t.Fatalf("got  %+v\nwant %+v", run.Setup, want)
	}
}

// A computer where pacman can't say which package a file belongs to is given nothing
func TestNothingIsReadWherePackagesCantBeAsked(t *testing.T) {
	root, _ := ownersComputer(t)
	run, _, err := Quick(machine.Fixture(root))
	if err != nil {
		t.Fatal(err)
	}
	if run.Setup != nil {
		t.Fatalf("got %+v", run.Setup)
	}
}
