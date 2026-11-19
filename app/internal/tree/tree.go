// Package tree is the way to try a changed device tree without risking the machine. It works
// from the same sources Omarchy's linux-omarchy-dtbs package builds from, compiles them the
// same way, and puts the result into a boot image and a boot menu entry of its own, through
// the tool that manages Omarchy's boot menu. The stock image and its entry are never touched.
package tree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Mark is put on the test image's kernel command line, so that after a restart it can be told
// for certain whether the test image is what started.
const Mark = "hwtest.tree=1"

// Running says whether the machine started from the test image.
func Running(cmdline string) bool {
	for _, word := range strings.Fields(cmdline) {
		if word == Mark {
			return true
		}
	}
	return false
}

// Entry is the name of the test entry in the boot menu. It starts like Omarchy's own kernel
// entries, which the boot menu's order puts after the stock kernel, so it never becomes the default.
const Entry = "linux-omarchy-hwtest"

// System is where the pieces are found. Tests point it at folders of their own.
type System struct {
	// Kernel is the running kernel's release, like 7.3.0-59-omarchy.
	Kernel string
	// Headers is the kernel's build folder, which linux-omarchy-headers fills with the device
	// tree sources and the kernel's own dtc.
	Headers string
	// UKIConf is the settings Omarchy writes for the boot image, which name this machine's tree.
	UKIConf string
	// Compatible is the device tree's first name for the machine, as booted.
	Compatible string
}

// Real is the running machine.
func Real() System {
	release, _ := exec.Command("uname", "-r").Output()
	kernel := strings.TrimSpace(string(release))
	compatible, _ := os.ReadFile("/proc/device-tree/compatible")
	first, _, _ := strings.Cut(string(compatible), "\x00")
	return System{Kernel: kernel, Headers: "/usr/lib/modules/" + kernel + "/build", UKIConf: "/etc/kernel/uki.conf", Compatible: first}
}

func (s System) sources() string { return filepath.Join(s.Headers, "arch/arm64/boot/dts/qcom") }
func (s System) dtc() string     { return filepath.Join(s.Headers, "scripts/dtc/dtc") }

var deviceTree = regexp.MustCompile(`(?m)^DeviceTree=(\S+)`)

// Machine names this machine's device tree source, like x1e80100-lenovo-yoga-slim7x. The boot
// image's settings name the tree the machine starts with. Without them, it is the source that
// names the machine the way the running tree does.
func (s System) Machine() (string, error) {
	if conf, err := os.ReadFile(s.UKIConf); err == nil {
		if match := deviceTree.FindSubmatch(conf); match != nil {
			name := strings.TrimSuffix(filepath.Base(string(match[1])), ".dtb")
			if _, err := os.Stat(filepath.Join(s.sources(), name+".dts")); err == nil {
				return name, nil
			}
		}
	}
	if s.Compatible != "" {
		files, _ := filepath.Glob(filepath.Join(s.sources(), "*.dts"))
		var found []string
		for _, file := range files {
			if text, err := os.ReadFile(file); err == nil && strings.Contains(string(text), `compatible = "`+s.Compatible+`"`) {
				found = append(found, strings.TrimSuffix(filepath.Base(file), ".dts"))
			}
		}
		if len(found) == 1 {
			return found[0], nil
		}
	}
	return "", fmt.Errorf("this machine's device tree source wasn't found among %s. Name it: hw-test tree build <name>, without .dts", s.sources())
}

// Ready says what is missing to build trees here, or nil.
func (s System) Ready() error {
	if _, err := os.Stat(s.sources()); err != nil {
		return fmt.Errorf("the device tree sources aren't installed. They come with the kernel's headers: sudo pacman -S linux-omarchy-headers")
	}
	if _, err := os.Stat(s.dtc()); err != nil {
		return fmt.Errorf("the kernel's dtc isn't at %s. Reinstall the kernel's headers: sudo pacman -S linux-omarchy-headers", s.dtc())
	}
	for _, tool := range []string{"gcc", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s is needed to build a tree: sudo pacman -S %s", tool, tool)
		}
	}
	return nil
}

func run(dir, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Start copies the sources into dir/tree as the package does, and puts them under git, so
// every change is a diff against what Omarchy ships. It refuses to write over work in progress.
func (s System) Start(dir string) (string, error) {
	if err := s.Ready(); err != nil {
		return "", err
	}
	root := filepath.Join(dir, "tree")
	if _, err := os.Stat(root); err == nil {
		return root, fmt.Errorf("%s is there already, with your changes in it. Go on with hw-test tree build, or move it away to start over", root)
	}
	for _, sub := range []string{"arch/arm64/boot", "include", "scripts/dtc"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return root, err
		}
	}
	// With their links resolved, as the package copies them: dt-bindings/input links into include/uapi
	copies := [][]string{
		{"cp", "-rL", filepath.Join(s.Headers, "arch/arm64/boot/dts"), filepath.Join(root, "arch/arm64/boot") + "/"},
		{"cp", "-rL", filepath.Join(s.Headers, "include/dt-bindings"), filepath.Join(root, "include") + "/"},
		{"cp", "-a", filepath.Join(s.Headers, "scripts/dtc/include-prefixes"), filepath.Join(root, "scripts/dtc") + "/"},
	}
	for _, copy := range copies {
		if _, err := run(dir, copy[0], copy[1:]...); err != nil {
			return root, err
		}
	}
	for _, git := range [][]string{{"init", "-q", "."}, {"add", "-A"}, {"-c", "user.name=hw-test", "-c", "user.email=hw-test@localhost", "commit", "-q", "-m", "The device tree sources of linux-omarchy " + s.Kernel}} {
		if _, err := run(root, "git", git...); err != nil {
			return root, err
		}
	}
	return root, nil
}

// Built is a compiled tree and what changed to make it.
type Built struct {
	// Machine is the tree's name, DTB the compiled file, and Diff the change against Omarchy's sources.
	Machine, DTB, Diff string
	// Changed lists the files that differ.
	Changed []string
}

// Build compiles one tree from dir/tree, exactly as the package does, and writes the change
// against Omarchy's sources beside it.
func (s System) Build(dir, machine string) (Built, error) {
	root := filepath.Join(dir, "tree")
	if _, err := os.Stat(root); err != nil {
		return Built{}, fmt.Errorf("there are no sources in %s yet. Start with: hw-test tree start", root)
	}
	source := filepath.Join("arch/arm64/boot/dts/qcom", machine+".dts")
	if _, err := os.Stat(filepath.Join(root, source)); err != nil {
		return Built{}, fmt.Errorf("there is no %s in %s", source, root)
	}
	out := filepath.Join(dir, machine)
	prefixes := "scripts/dtc/include-prefixes"
	if _, err := run(root, "gcc", "-E", "-nostdinc", "-undef", "-D__DTS__", "-x", "assembler-with-cpp", "-I", prefixes, "-o", out+".pre.dts", source); err != nil {
		return Built{}, err
	}
	defer os.Remove(out + ".pre.dts")
	if _, err := run(root, s.dtc(), "-q", "-@", "-O", "dtb", "-b", "0", "-i", filepath.Dir(source), "-i", prefixes, "-o", out+".dtb", out+".pre.dts"); err != nil {
		return Built{}, err
	}
	built := Built{Machine: machine, DTB: out + ".dtb", Diff: filepath.Join(dir, "tree.diff")}
	// Everything that differs from Omarchy's sources, new files included
	if _, err := run(root, "git", "add", "-A", "-N"); err != nil {
		return built, err
	}
	diff, err := run(root, "git", "diff", "HEAD")
	if err != nil {
		return built, err
	}
	if err := os.WriteFile(built.Diff, []byte(diff), 0o644); err != nil {
		return built, err
	}
	names, _ := run(root, "git", "diff", "HEAD", "--name-only")
	built.Changed = strings.Fields(names)
	return built, nil
}

// Script is what the owner runs, with their password, to put a compiled tree into a boot
// image and a menu entry of its own. It is written out for them to read first. It builds an
// initramfs for the running kernel, packs the stock kernel with it and the changed tree, and
// hands the image to the boot menu's own tool, which keeps the menu's checksums right.
// Nothing in it touches the stock image or its entry.
func (s System) Script(dtb, cmdline string) string {
	quote := func(text string) string { return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'" }
	return `#!/bin/bash
# Written by hw-test tree boot. Puts a changed device tree into a boot image and a boot menu
# entry of its own, "` + Entry + `". The stock image and its entry are not touched.
set -euo pipefail
kernel=` + quote(s.Kernel) + `
dtb=` + quote(dtb) + `
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# With Omarchy's Direct Boot the firmware starts the stock image itself and no boot menu
# shows, so there would be no way to pick the test entry.
if command -v omarchy-boot-direct >/dev/null && omarchy-boot-direct; then
  echo "Direct Boot is on, so no boot menu shows at start. Turn it off first: Omarchy menu > Setup > Direct Boot." >&2
  exit 1
fi

# A boot image is about as large as the kernel and its initramfs together. A full boot
# partition is what breaks a later, ordinary update, so stop well short of it.
free=$(df --output=avail -m /boot | tail -1)
if (( free < 400 )); then
  echo "The boot partition has only ${free} MB free. Free some first: old test entries, old kernels." >&2
  exit 1
fi

echo "Building an initramfs for $kernel..."
mkinitcpio -k "$kernel" -g "$work/initramfs.img" >"$work/mkinitcpio.log" 2>&1 || { tail -20 "$work/mkinitcpio.log" >&2; exit 1; }

# The system's settings for boot images are left out on purpose: they would add every
# machine's tree, and the stock one would be picked over the changed one.
echo "Packing the kernel, the initramfs and the changed tree..."
ukify build --config /dev/null --linux "/usr/lib/modules/$kernel/vmlinuz" --initrd "$work/initramfs.img" \
  --cmdline ` + quote(strings.TrimSpace(cmdline)+" "+Mark) + ` --devicetree "$dtb" --uname "$kernel" --output "$work/` + Entry + `.efi"

echo "Adding the entry to the boot menu..."
limine-entry-tool --add-uki ` + quote(Entry) + ` "$work/` + Entry + `.efi" --comment "hw-test: a changed device tree to try"

echo
echo "Done. Restart, and in the boot menu pick ` + Entry + ` (it is listed under Omarchy, after the usual kernel)."
echo "The usual entry is untouched and still the default. If the test entry doesn't start,"
echo "hold the power button until the computer is off, start it again and let the usual entry run."
echo "After the restart, hw-test tree status says which one is running. To take the test entry away: hw-test tree remove"
`
}

// RemoveScript takes the test entry and its image away again.
func RemoveScript() string {
	return `#!/bin/bash
# Written by hw-test tree remove. Takes the test entry "` + Entry + `" and its boot image away.
set -euo pipefail
limine-entry-tool --remove-uki '` + Entry + `'
echo "The test entry is gone. The boot menu is as it was before."
`
}
