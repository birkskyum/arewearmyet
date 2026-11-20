package probe

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// The command line of an encrypted Yoga Slim 7x as Omarchy installs it, with the disk's ID changed
const stockCmdline = "cryptdevice=PARTUUID=11111111-2222-3333-4444-555555555555:root root=/dev/mapper/root zswap.enabled=0 rootflags=subvol=@ rw rootfstype=btrfs console=tty0 initcall_blacklist=simpledrm_platform_driver_init resume=/dev/mapper/root resume_offset=1872623 iommu.passthrough=0 clk_ignore_unused pd_ignore_unused arm64.nopauth systemd.tpm2_wait=0 systemd.tty.term.console=dumb initramfs_async=0 quiet splash loglevel=0 systemd.show_status=false rd.udev.log_level=0 vt.global_cursor_default=0"

func TestStockOptionsAreNotTheOwners(t *testing.T) {
	if extra := ExtraOptions(stockCmdline); len(extra) != 0 {
		t.Fatalf("Omarchy's own options were taken for the owner's: %v", extra)
	}
	live := "archisobasedir=arch archisosearchuuid=2026-11-20-05-00-00-00 copytoram=n initramfs_options=size=90% quiet splash clk_ignore_unused pd_ignore_unused arm64.nopauth systemd.tpm2_wait=0 iommu.passthrough=0 modprobe.blacklist=qcom_q6v5_pas omarchy.qcom_early=1"
	if extra := ExtraOptions(live); len(extra) != 0 {
		t.Fatalf("the live image's own options were taken for the owner's: %v", extra)
	}
}

func TestOwnersOptionsAreNamed(t *testing.T) {
	got := ExtraOptions(stockCmdline + " gpiolib_acpi.ignore_wake=VEN_0488:00@355 regulator_ignore_unused module_blacklist=qcom_q6v5_pas console=ttyMSM0,115200 regulator_ignore_unused")
	// By name alone. console is Omarchy's with another value, and here it is the owner's
	want := []string{"gpiolib_acpi.ignore_wake", "regulator_ignore_unused", "module_blacklist", "console"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// No value is sent, so nothing that names a disk, a card or an address can be in one
func TestOptionValuesStayHome(t *testing.T) {
	got := ExtraOptions("rd.luks.uuid=11111111-2222-3333-4444-555555555555 ip=192.168.1.20 netconsole=6666@10.0.0.2/aa:bb:cc:dd:ee:ff luks.key=/home/birk/key some\"thing=1 dm-mod.create=0123456789abcdef0123")
	want := []string{"rd.luks.uuid", "ip", "netconsole", "luks.key", "dm-mod.create"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// The lists here and in data/kernel-options.yaml, which the site reads, are the same
func TestOwnOptionsMatchTheSites(t *testing.T) {
	data, err := os.ReadFile("../../../data/kernel-options.yaml")
	if err != nil {
		t.Fatal(err)
	}
	lists := map[string][]string{}
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "":
		case strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " "):
			section = strings.TrimSuffix(line, ":")
		case strings.HasPrefix(line, "  - "):
			lists[section] = append(lists[section], strings.TrimPrefix(line, "  - "))
		default:
			t.Fatalf("a line of data/kernel-options.yaml that this test can't read: %q", line)
		}
	}
	if !reflect.DeepEqual(lists["keys"], ownKeys) {
		t.Errorf("keys differ.\nfile %v\nhere %v", lists["keys"], ownKeys)
	}
	if !reflect.DeepEqual(lists["files"], ownFiles) {
		t.Errorf("files differ.\nfile %v\nhere %v", lists["files"], ownFiles)
	}
	if !reflect.DeepEqual(lists["exact"], ownOptions) {
		t.Errorf("exact options differ.\nfile %v\nhere %v", lists["exact"], ownOptions)
	}
}

// commands answers pacman and modinfo the way a computer with the owner's own packages does
func commands(sys machine.System, answers map[string]string) machine.System {
	fixture := sys.Command
	sys.Command = func(name string, args ...string) (string, error) {
		if answer, ok := answers[name+" "+strings.Join(args, " ")]; ok {
			return answer, nil
		}
		if name == "pacman" && len(args) > 0 && args[0] != "-Q" {
			return "", os.ErrNotExist
		}
		if name == "modinfo" {
			return "", os.ErrNotExist
		}
		return fixture(name, args...)
	}
	return sys
}

func TestStockComputerHasNoSetup(t *testing.T) {
	run, _ := fixture(t, func(root string) {
		write(t, root, "proc/cmdline", stockCmdline)
		write(t, root, "proc/modules", "phy_qcom_qmp_ufs 77824 0 - Live 0x0000000000000000\ncdc_wdm 32768 1 cdc_mbim, Live 0x0000000000000000\n")
	})
	if run.Setup != nil {
		t.Fatalf("a stock computer was given a setup: %+v", run.Setup)
	}
	if strings.Contains(string(run.JSON()), "setup") {
		t.Fatal("a run without a setup still sends one")
	}
}

func TestSetupNamesWhatOmarchyDidNotInstall(t *testing.T) {
	root := copied(t)
	kernel := kernelOf(t, root)
	write(t, root, "proc/cmdline", stockCmdline+" regulator_ignore_unused")
	// One module of the owner's own, one that comes in a package from Omarchy's repository (as
	// NVIDIA's do), and one that came with a package built on the computer
	write(t, root, "proc/modules", strings.Join([]string{
		"phy_qcom_qmp_ufs 77824 0 - Live 0x0000000000000000",
		"omnibook_fix 16384 0 - Live 0x0000000000000000 (OE)",
		"nvidia 9000000 12 nvidia_modeset, Live 0x0000000000000000 (O)",
		"cam_fix 16384 0 - Live 0x0000000000000000 (OE)",
		"staging_thing 16384 0 - Live 0x0000000000000000 (C)",
	}, "\n")+"\n")
	write(t, root, "usr/lib/modules/"+kernel+"/pkgbase", "linux-omarchy\n")
	sys := commands(machine.Fixture(root), map[string]string{
		"pacman -Qm":              "cam-fix-dkms 1-1\nslack 3.0.4-1\nlinux-other 7.3-1\nmy-ucm 1-1\n",
		"modinfo -n omnibook_fix": "/home/owner/omnibook_fix.ko\n",
		"modinfo -n nvidia":       "/usr/lib/modules/" + kernel + "/extramodules/nvidia.ko.zst\n",
		"modinfo -n cam_fix":      "/usr/lib/modules/" + kernel + "/updates/dkms/cam_fix.ko.zst\n",
		"pacman -Qqo /usr/lib/modules/" + kernel + "/extramodules/nvidia.ko.zst":  "linux-omarchy-nvidia-open\n",
		"pacman -Qqo /usr/lib/modules/" + kernel + "/updates/dkms/cam_fix.ko.zst": "cam-fix-dkms\n",
		"pacman -Ql cam-fix-dkms linux-other my-ucm slack": strings.Join([]string{
			"cam-fix-dkms /usr/lib/modules/",
			"cam-fix-dkms /usr/lib/modules/" + kernel + "/updates/dkms/cam_fix.ko.zst",
			"slack /usr/bin/slack",
			"slack /usr/lib/slack/resources/firmware.bin",
			"linux-other /usr/lib/modules/7.3.0-1-other/vmlinuz",
			"linux-other /boot/dtbs/linux-other/qcom/x1e80100-lenovo-yoga-slim7x.dtb",
			"my-ucm /usr/share/alsa/ucm2/",
			"my-ucm /usr/share/alsa/ucm2/conf.d/x1e80100/x1e80100.conf",
		}, "\n") + "\n",
	})
	run, _, err := Quick(sys)
	if err != nil {
		t.Fatal(err)
	}
	want := &report.Setup{
		Options: []string{"regulator_ignore_unused"},
		Modules: []string{"omnibook_fix", "cam_fix"},
	}
	if !reflect.DeepEqual(run.Setup, want) {
		t.Fatalf("got %+v\nwant %+v", run.Setup, want)
	}
	// The packages that hold hardware files are for the owner's agent to read, and none is sent
	if found := FindChanges(sys, run); !reflect.DeepEqual(found.Packages, []string{"cam-fix-dkms", "my-ucm"}) {
		t.Errorf("packages %v", found.Packages)
	}
	sent := string(run.JSON())
	for _, private := range []string{"slack", "/home/owner", "11111111", "linux-other", "cam-fix-dkms", "my-ucm"} {
		if strings.Contains(sent, private) {
			t.Errorf("the run sends %q, which is the owner's own business", private)
		}
	}
}

func TestSetupNamesAnotherKernelAndARebuiltPackage(t *testing.T) {
	root := copied(t)
	kernel := kernelOf(t, root)
	write(t, root, "usr/lib/modules/"+kernel+"/pkgbase", "linux-omarchy-mine\n")
	// The fixture's linux-omarchy is 7.3-59 and its linux-firmware-qcom 20261019-1.1
	write(t, root, "var/lib/pacman/local/linux-omarchy-7.3-59/desc", "%NAME%\nlinux-omarchy\n\n%PACKAGER%\nUnknown Packager\n\n")
	write(t, root, "var/lib/pacman/local/linux-firmware-qcom-20261019-1.1/desc", "%NAME%\nlinux-firmware-qcom\n\n%PACKAGER%\nOmarchy <build@example.org>\n\n")
	run, _, err := Quick(machine.Fixture(root))
	if err != nil {
		t.Fatal(err)
	}
	want := &report.Setup{Kernel: "linux-omarchy-mine", Rebuilt: []string{"linux-omarchy"}}
	if !reflect.DeepEqual(run.Setup, want) {
		t.Fatalf("got %+v\nwant %+v", run.Setup, want)
	}
}

func kernelOf(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "cmd", "uname-r"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}
