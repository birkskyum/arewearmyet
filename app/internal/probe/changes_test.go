package probe

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
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
	if said := ruleWords(got[0]) + ", " + ruleWords(got[1]) + ", " + ruleWords(got[4]); said != "has a device rule for usb that sets power/control=on, has a device rule for usb that runs a program, has a device rule that changes who may use it" {
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

// What the owner's files change is found for their agent to read, with the files' paths, and
// none of it goes into what a run sends
func TestChangesAreFoundForTheAgentAndNotSent(t *testing.T) {
	root, held := ownersComputer(t)
	check := "warning: alsa-ucm-conf: /usr/share/alsa/ucm2/x.conf (Modification time mismatch)\nalsa-ucm-conf: 1223 total files, 1 altered file\nwireplumber: 327 total files, 0 altered files\npipewire: 249 total files, 2 altered files\nerror: package 'libcamera' was not found\n"
	sys := packaged(machine.Fixture(root), held, []string{"qualcomm-adsp-nofw.conf", "qcom-firmware.conf"}, check)
	run, _, err := Quick(sys)
	if err != nil {
		t.Fatal(err)
	}
	if run.Setup != nil {
		t.Fatalf("settings files went into what a run sends: %+v", run.Setup)
	}
	found := FindChanges(sys, run)
	want := Changes{
		Settings: []SettingsFile{
			{"/etc/modprobe.d/my-fix.conf", []string{"gives kernel module snd_soc_x1e80100 the option i2s_mode=1", "gives kernel module snd_soc_x1e80100 the option serial=…", "keeps kernel module qcom_spmi_temp_alarm from loading", "keeps kernel module pcspkr from loading"}},
			{"/etc/modules-load.d/kef-speakers.conf", []string{"loads kernel module scmi-cpufreq at start"}},
			{"/etc/mkinitcpio.conf.d/mine.conf", []string{"puts kernel module panel_edp into the boot image", "puts kernel module msm into the boot image"}},
			{"/etc/udev/rules.d/99-yubikey.rules", []string{"has a device rule for usb that sets power/control=on", "has a device rule for usb that runs a program", "has a device rule for input that sets LIBINPUT_IGNORE_DEVICE", "has a device rule for input that changes who may use it", "has a device rule that changes who may use it", "has a device rule that sets serial"}},
			{"/etc/wireplumber/wireplumber.conf.d/kef-lsx-no-suspend.conf", []string{"sets the sound setting api.alsa.headroom", "sets the sound setting session.suspend-timeout-seconds"}},
		},
		Placed:  []string{"/usr/lib/firmware/qcom/x1e80100/LENOVO/83ED/qcadsp8380.mbn", "/usr/share/alsa/ucm2/Qualcomm/x1e80100/mine.conf"},
		Altered: []string{"/usr/share/alsa/ucm2/x.conf", "pipewire"},
	}
	if !reflect.DeepEqual(found, want) {
		t.Fatalf("got  %+v\nwant %+v", found, want)
	}
	if lines := found.Lines(); len(lines) != 9 || lines[1] != "/etc/modules-load.d/kef-speakers.conf, which loads kernel module scmi-cpufreq at start" {
		t.Errorf("in words: %q", lines)
	}
	// Omarchy's own files, and the copy kept beside one of the owner's, are not among them
	for _, line := range found.Lines() {
		for _, omarchys := range []string{"hid_apple", "qualcomm-adsp-nofw", "qcom-firmware.conf", "omarchy-usb-autosuspend", ".bak", "qcdxkmsuc8380", "a660_sqe"} {
			if strings.Contains(line, omarchys) {
				t.Errorf("%q is taken for the owner's: %s", omarchys, line)
			}
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
	sys := packaged(machine.Fixture(root), held, nil, "")
	run, _, err := Quick(sys)
	if err != nil {
		t.Fatal(err)
	}
	want := Changes{Placed: []string{"/usr/lib/firmware/qcom/x1e80100/LENOVO/83ED/qcadsp8380.mbn", "/usr/share/alsa/ucm2/Qualcomm/x1e80100/mine.conf"}}
	if found := FindChanges(sys, run); !reflect.DeepEqual(found, want) {
		t.Fatalf("got  %+v\nwant %+v", found, want)
	}
}

// A computer where pacman can't say which package a file belongs to is given nothing
func TestNothingIsReadWherePackagesCantBeAsked(t *testing.T) {
	root, _ := ownersComputer(t)
	sys := machine.Fixture(root)
	run, _, err := Quick(sys)
	if err != nil {
		t.Fatal(err)
	}
	if found := FindChanges(sys, run); !found.Empty() {
		t.Fatalf("got %+v", found)
	}
}
