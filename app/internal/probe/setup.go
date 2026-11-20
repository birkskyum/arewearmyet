package probe

import (
	"regexp"
	"sort"
	"strings"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// Nearly every computer that runs the test has something on it that Omarchy didn't install, which
// is how its owner got a part working. So a run doesn't say whether the computer is stock. It
// names what differs, where the machine itself can tell, and a result is read against that.
//
// What can't be told this way (a rule or settings file the owner wrote, a changed device tree)
// is asked about at the end of the guided test.

// ownKeys and ownOptions are the kernel options Omarchy sets by itself, as
// data/kernel-options.yaml in the repository lists them. An option in ownKeys is Omarchy's
// whatever its value, and most of those name a disk, so they are never sent.
var ownKeys = []string{"root", "rootflags", "rootfstype", "cryptdevice", "cryptkey", "resume", "resume_offset",
	"archisobasedir", "archisosearchuuid", "archisolabel", "archisodevice", "img_dev", "img_loop", "initrd", "BOOT_IMAGE", "omarchy.cmdline_room"}

var ownOptions = []string{"rw", "quiet", "splash", "loglevel=0", "systemd.show_status=false", "rd.udev.log_level=0", "vt.global_cursor_default=0",
	"initramfs_async=0", "systemd.tty.term.console=dumb", "zswap.enabled=0", "clk_ignore_unused", "pd_ignore_unused", "arm64.nopauth",
	"systemd.tpm2_wait=0", "iommu.passthrough=0", "iommu.passthrough=1", "console=tty0", "initcall_blacklist=simpledrm_platform_driver_init",
	"cma=256M", "acpi=nospcr", "plymouth.enable=0", "efi_reclaim_reserved=58G@8G", "efi_reclaim_reserved=58G@8G,3968M@67712M",
	"mem_sleep_default=s2idle", "pci=hpbussize=0x20", "pci=hpbussize=0x80,hpmmiosize=32M,hpmmioprefsize=32G", "power_wrap_drv.usb4_release=0",
	"qcom_aoss.prevent_aoss_sleep=1", "rtc_cmos.use_acpi_alarm=1", "copytoram=n", "initramfs_options=size=90%", "accessibility=on",
	"modprobe.blacklist=qcom_q6v5_pas", "omarchy.qcom_early=1"}

// rebuildable are the packages a locally built copy of which changes what the hardware does
var rebuildable = []string{"linux-omarchy", "linux-omarchy-dtbs", "linux-firmware-qcom"}

var (
	optionKey   = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,60}$`)
	optionValue = regexp.MustCompile(`^[A-Za-z0-9_.,:@%+\-]{0,60}$`)
	// A value that names one disk, network card or address says who the computer is, not what it runs
	identity = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-|([0-9a-f]{2}[:-]){5}[0-9a-f]{2}|\d{1,3}(\.\d{1,3}){3}|[0-9a-f]{16}`)
	// The marks the kernel puts on a loaded module that isn't its own, which are P for a closed
	// one, O for one built outside the kernel, E for an unsigned one and F for one loaded by force
	marked     = regexp.MustCompile(`\(([A-Z+]+)\)\s*$`)
	simpleName = regexp.MustCompile(`^[A-Za-z0-9_.+@\-]{1,60}$`)
)

// setup reads what the computer runs that Omarchy didn't install.
func setup(sys machine.System, run *report.Run) *report.Setup {
	found := &report.Setup{Options: ExtraOptions(sys.ReadAll("/proc/cmdline"))}
	foreign := foreignPackages(sys)

	// The kernel that is running, by the package it came from
	if base := sys.Read("/usr/lib/modules/" + run.Kernel + "/pkgbase"); base != "" && base != "linux-omarchy" && simpleName.MatchString(base) {
		found.Kernel = base
	}
	for _, name := range rebuildable {
		version := run.Packages[name]
		if version == "" {
			continue
		}
		_, after, _ := strings.Cut(sys.ReadAll("/var/lib/pacman/local/"+name+"-"+version+"/desc"), "%PACKAGER%\n")
		if builder, _, _ := strings.Cut(after, "\n"); builder != "" && !strings.HasPrefix(builder, "Omarchy") {
			found.Rebuilt = append(found.Rebuilt, name)
		}
	}

	for _, line := range strings.Split(sys.ReadAll("/proc/modules"), "\n") {
		mark := marked.FindStringSubmatch(line)
		name, _, _ := strings.Cut(line, " ")
		if mark == nil || !strings.ContainsAny(mark[1], "POEF") || !simpleName.MatchString(name) || len(found.Modules) >= 20 {
			continue
		}
		// Omarchy ships such modules too, like NVIDIA's, and those come in a package from its repository
		owner := ""
		if file := strings.TrimSpace(sys.Run("modinfo", "-n", name)); strings.HasPrefix(file, "/") {
			owner = strings.TrimSpace(sys.Run("pacman", "-Qqo", file))
		}
		if owner == "" || foreign[owner] {
			found.Modules = append(found.Modules, name)
		}
	}

	found.Packages = hardwarePackages(sys, foreign, run.Kernel)
	if found.Empty() {
		return nil
	}
	return found
}

// ExtraOptions are the kernel options in a command line beyond the ones Omarchy sets. An option
// whose value names a disk or an address, or holds anything but plain characters, is given by its
// name alone.
func ExtraOptions(cmdline string) []string {
	own := map[string]bool{}
	for _, option := range ownOptions {
		own[option] = true
	}
	keys := map[string]bool{}
	for _, key := range ownKeys {
		keys[key] = true
	}
	var extra []string
	seen := map[string]bool{}
	for _, option := range strings.Fields(cmdline) {
		key, value, valued := strings.Cut(option, "=")
		if own[option] || keys[key] || !optionKey.MatchString(key) {
			continue
		}
		if valued && (!optionValue.MatchString(value) || identity.MatchString(value)) {
			option = key + "=…"
		}
		if !seen[option] && len(extra) < 24 {
			seen[option] = true
			extra = append(extra, option)
		}
	}
	return extra
}

// foreignPackages are the installed packages that are in no repository, which were built on the
// computer or installed from a file.
func foreignPackages(sys machine.System) map[string]bool {
	foreign := map[string]bool{}
	for _, line := range strings.Split(sys.Run("pacman", "-Qm"), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && simpleName.MatchString(fields[0]) {
			foreign[fields[0]] = true
		}
	}
	return foreign
}

// hardwarePackages are the packages from no repository that hold kernel modules for the running
// kernel, firmware or sound profiles. The owner's other packages are nobody's business and are
// never listed.
func hardwarePackages(sys machine.System, foreign map[string]bool, kernel string) []string {
	if len(foreign) == 0 {
		return nil
	}
	names := make([]string, 0, len(foreign))
	for name := range foreign {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 300 {
		names = names[:300]
	}
	places := []string{"/usr/lib/firmware/", "/usr/share/alsa/ucm2/", "/usr/share/libcamera/"}
	if kernel != "" {
		places = append(places, "/usr/lib/modules/"+kernel+"/")
	}
	holds := map[string]bool{}
	for _, line := range strings.Split(sys.Run("pacman", append([]string{"-Ql"}, names...)...), "\n") {
		name, file, _ := strings.Cut(line, " ")
		if !foreign[name] || strings.HasSuffix(file, "/") {
			continue
		}
		for _, place := range places {
			if strings.HasPrefix(file, place) {
				holds[name] = true
			}
		}
	}
	var list []string
	for _, name := range names {
		if holds[name] && len(list) < 20 {
			list = append(list, name)
		}
	}
	return list
}
