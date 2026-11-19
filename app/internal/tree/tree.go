// Package tree is the way to try a changed device tree without risking the machine. It works
// from the same sources Omarchy's linux-omarchy-dtbs package builds from, with the patches
// that package applies, compiles them the same way, and puts the result into a boot image and
// a boot menu entry of its own, through the tool that manages Omarchy's boot menu. The stock
// image and its entry are never touched.
package tree

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Packages is the repository Omarchy's packages are built from, and Recipe the folder in it
// of the package that ships the device trees, with the patches it lays over the kernel's sources.
const (
	Packages = "https://github.com/omacom/omarchy-pkgs"
	Recipe   = "pkgbuilds/linux-omarchy-dtbs"
)

// Mark is a property hw-test tree build adds to the root of every tree it compiles. It means
// nothing to the kernel. After a restart it tells for certain whether the tree that was built
// here is the one the computer is running, whatever the boot menu did with the command line.
const Mark = "hw-test-tree"

// Running says whether the computer is running a tree that hw-test tree build made. root is
// where the running tree is read, /proc/device-tree on a real machine.
func Running(root string) bool {
	_, err := os.Stat(filepath.Join(root, Mark))
	return err == nil
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
	// Recipe is the folder of Omarchy's device tree package. Empty means the one in the
	// checkout of Omarchy's packages beside the sources, which Start clones when it isn't there.
	Recipe string
	// Installed is where the package manager keeps what it installed of the device tree
	// package, with each file's checksum. The trees themselves are on the boot partition,
	// which only root reads. It may hold a * for the package's version.
	Installed string
}

// Real is the running machine.
func Real() System {
	release, _ := exec.Command("uname", "-r").Output()
	kernel := strings.TrimSpace(string(release))
	compatible, _ := os.ReadFile("/proc/device-tree/compatible")
	first, _, _ := strings.Cut(string(compatible), "\x00")
	return System{Kernel: kernel, Headers: "/usr/lib/modules/" + kernel + "/build", UKIConf: "/etc/kernel/uki.conf", Compatible: first,
		Installed: "/var/lib/pacman/local/linux-omarchy-dtbs-*/mtree"}
}

// installed is the checksum of the tree the package installed for a machine, or empty when
// the package's record doesn't have it.
func (s System) installed(machine string) string {
	records, _ := filepath.Glob(s.Installed)
	if len(records) != 1 {
		return ""
	}
	file, err := os.Open(records[0])
	if err != nil {
		return ""
	}
	defer file.Close()
	var text []byte
	if packed, err := gzip.NewReader(file); err == nil {
		text, _ = io.ReadAll(io.LimitReader(packed, 4<<20))
	}
	for _, line := range strings.Split(string(text), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasSuffix(fields[0], "/qcom/"+machine+".dtb") {
			continue
		}
		for _, field := range fields[1:] {
			if sum, ok := strings.CutPrefix(field, "sha256digest="); ok {
				return sum
			}
		}
	}
	return ""
}

const qcom = "arch/arm64/boot/dts/qcom"

func (s System) sources() string { return filepath.Join(s.Headers, qcom) }
func (s System) dtc() string     { return filepath.Join(s.Headers, "scripts/dtc/dtc") }

// recipe is where the package's patches are read from, for work going on in dir.
func (s System) recipe(dir string) string {
	if s.Recipe != "" {
		return s.Recipe
	}
	return filepath.Join(dir, "omarchy-pkgs", Recipe)
}

var deviceTree = regexp.MustCompile(`(?m)^DeviceTree=(\S+)`)

// Machine names this machine's device tree source, like x1e80100-lenovo-yoga-slim7x. The boot
// image's settings name the tree the machine starts with. Without them, it is the source that
// names the machine the way the running tree does.
func (s System) Machine() (string, error) { return s.machine(s.sources()) }

// MachineIn names this machine's source among the ones being worked on in dir, which hold
// the trees Omarchy's patches add as well. Before hw-test tree start it is as Machine.
func (s System) MachineIn(dir string) (string, error) {
	worked := filepath.Join(dir, "tree", qcom)
	if _, err := os.Stat(worked); err != nil {
		return s.Machine()
	}
	return s.machine(worked)
}

func (s System) machine(sources string) (string, error) {
	if conf, err := os.ReadFile(s.UKIConf); err == nil {
		if match := deviceTree.FindSubmatch(conf); match != nil {
			name := strings.TrimSuffix(filepath.Base(string(match[1])), ".dtb")
			if _, err := os.Stat(filepath.Join(sources, name+".dts")); err == nil {
				return name, nil
			}
		}
	}
	if s.Compatible != "" {
		files, _ := filepath.Glob(filepath.Join(sources, "*.dts"))
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
	return "", fmt.Errorf("this machine's device tree source wasn't found among %s. Name it: hw-test tree build <name>, without .dts", sources)
}

// Ready says what is missing to build trees here, or nil.
func (s System) Ready() error {
	if _, err := os.Stat(s.sources()); err != nil {
		return fmt.Errorf("the device tree sources aren't installed. They come with the kernel's headers: sudo pacman -S linux-omarchy-headers")
	}
	if _, err := os.Stat(s.dtc()); err != nil {
		return fmt.Errorf("the kernel's dtc isn't at %s. Reinstall the kernel's headers: sudo pacman -S linux-omarchy-headers", s.dtc())
	}
	for _, tool := range []string{"gcc", "git", "patch"} {
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

// Started is what hw-test tree start made.
type Started struct {
	// Root is the folder with the sources, and Recipe the package folder its patches came from.
	Root, Recipe string
	// Applied counts Omarchy's patches laid over the kernel's sources, Skipped names the ones
	// the kernel's sources already had, and Misfit the ones that don't fit these sources,
	// which happens when the computer's kernel isn't the one the packages are built for now.
	Applied         int
	Skipped, Misfit []string
	// Machine is this machine's tree, when it was found among the sources.
	Machine string
	// Stock says how the sources compare with the tree this computer starts with: "same",
	// "differs", or empty when it couldn't be told.
	Stock string
}

var patchLine = regexp.MustCompile(`(?m)^\s*(\S+\.patch)\s*$`)

// patches lists the recipe's patches in the order it applies them.
func patches(recipe string) ([]string, error) {
	text, err := os.ReadFile(filepath.Join(recipe, "PKGBUILD"))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, match := range patchLine.FindAllSubmatch(text, -1) {
		names = append(names, string(match[1]))
	}
	return names, nil
}

// Start copies the sources into dir/tree and lays Omarchy's patches over them, as the package
// does, and puts the result under git, so every change is a diff against what Omarchy ships.
// It refuses to write over work in progress.
func (s System) Start(dir string) (Started, error) {
	started := Started{Root: filepath.Join(dir, "tree"), Recipe: s.recipe(dir)}
	if err := s.Ready(); err != nil {
		return started, err
	}
	root := started.Root
	if _, err := os.Stat(root); err == nil {
		return started, fmt.Errorf("%s is there already, with your changes in it. Go on with hw-test tree build, or move it away to start over", root)
	}
	// The patches come from Omarchy's packages, which is also where the fix will go
	if _, err := os.Stat(started.Recipe); err != nil {
		if s.Recipe != "" {
			return started, fmt.Errorf("there is no device tree package in %s", s.Recipe)
		}
		if _, err := run(dir, "git", "clone", "-q", "--depth", "1", Packages, filepath.Join(dir, "omarchy-pkgs")); err != nil {
			return started, fmt.Errorf("Omarchy's packages couldn't be fetched, and the device trees are built with the patches kept there: %v", err)
		}
	}
	names, err := patches(started.Recipe)
	if err != nil {
		return started, err
	}
	for _, sub := range []string{"arch/arm64/boot", "include", "scripts/dtc"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return started, err
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
			return started, err
		}
	}
	for _, name := range names {
		patch, err := filepath.Abs(filepath.Join(started.Recipe, name))
		if err != nil {
			return started, err
		}
		// One the kernel's sources already have is passed over, as the package does
		if _, err := run(root, "patch", "-Rp1", "--fuzz=0", "--dry-run", "-sf", "-i", patch); err == nil {
			started.Skipped = append(started.Skipped, name)
			continue
		}
		// One that doesn't fit is left out whole, never half applied. Whether that matters for
		// this machine shows below, where its tree is compared with the installed one
		if _, err := run(root, "patch", "-Np1", "--fuzz=0", "--dry-run", "-sf", "-i", patch); err != nil {
			started.Misfit = append(started.Misfit, name)
			continue
		}
		if _, err := run(root, "patch", "-Np1", "--fuzz=0", "-s", "-i", patch); err != nil {
			os.RemoveAll(root)
			return started, err
		}
		started.Applied++
	}
	for _, git := range [][]string{{"init", "-q", "."}, {"add", "-A"}, {"-c", "user.name=hw-test", "-c", "user.email=hw-test@localhost", "commit", "-q", "-m",
		fmt.Sprintf("The device tree sources of linux-omarchy %s, with %d of Omarchy's patches", s.Kernel, started.Applied)}} {
		if _, err := run(root, "git", git...); err != nil {
			return started, err
		}
	}
	// Whether these sources are what the computer starts with: compiled unchanged, they give
	// the installed tree byte for byte, unless the installed package is older or newer
	if machine, err := s.machine(filepath.Join(root, qcom)); err == nil {
		started.Machine = machine
		out := filepath.Join(dir, ".stock-"+machine)
		if s.compile(root, machine, out, false) == nil {
			built, _ := os.ReadFile(out + ".dtb")
			if want := s.installed(machine); want != "" && len(built) > 0 {
				sum := sha256.Sum256(built)
				started.Stock = "differs"
				if hex.EncodeToString(sum[:]) == want {
					started.Stock = "same"
				}
			}
		}
		os.Remove(out + ".dtb")
	}
	return started, nil
}

// base is the commit the sources started as: the kernel's, with Omarchy's patches.
func base(root string) (string, error) {
	out, err := run(root, "git", "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// compile turns one tree's source into out.dtb, exactly as the package does. A tree built to
// be tried carries the mark.
func (s System) compile(root, machine, out string, mark bool) error {
	source := filepath.Join(qcom, machine+".dts")
	prefixes := "scripts/dtc/include-prefixes"
	if _, err := run(root, "gcc", "-E", "-nostdinc", "-undef", "-D__DTS__", "-x", "assembler-with-cpp", "-I", prefixes, "-o", out+".pre.dts", source); err != nil {
		return err
	}
	defer os.Remove(out + ".pre.dts")
	if mark {
		// The mark goes into the compiled tree only, never into the sources or the diff
		pre, err := os.OpenFile(out+".pre.dts", os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, err = pre.WriteString("\n/ { " + Mark + "; };\n")
		pre.Close()
		if err != nil {
			return err
		}
	}
	_, err := run(root, s.dtc(), "-q", "-@", "-O", "dtb", "-b", "0", "-i", filepath.Dir(source), "-i", prefixes, "-o", out+".dtb", out+".pre.dts")
	return err
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
	if _, err := os.Stat(filepath.Join(root, qcom, machine+".dts")); err != nil {
		return Built{}, fmt.Errorf("there is no %s in %s", filepath.Join(qcom, machine+".dts"), root)
	}
	out := filepath.Join(dir, machine)
	if err := s.compile(root, machine, out, true); err != nil {
		return Built{}, err
	}
	built := Built{Machine: machine, DTB: out + ".dtb", Diff: filepath.Join(dir, "tree.diff")}
	from, err := base(root)
	if err != nil {
		return built, err
	}
	// Everything that differs from Omarchy's sources, new files included
	if _, err := run(root, "git", "add", "-A", "-N"); err != nil {
		return built, err
	}
	// With the usual a/ and b/, whatever the owner's git is set to show
	diff, err := run(root, "git", "diff", "--src-prefix=a/", "--dst-prefix=b/", from)
	if err != nil {
		return built, err
	}
	if err := os.WriteFile(built.Diff, []byte(diff), 0o644); err != nil {
		return built, err
	}
	names, _ := run(root, "git", "diff", from, "--name-only")
	built.Changed = strings.Fields(names)
	return built, nil
}

// Patched is a change written out the way Omarchy's device tree package carries it.
type Patched struct {
	// File is the patch, Recipe the package folder it goes into, and Changed the files it touches.
	File, Recipe string
	Changed      []string
	// Numbered is set when the patch's number is the next free one in the recipe. Without the
	// recipe at hand the number is a guess to be put right.
	Numbered bool
}

var patchNumber = regexp.MustCompile(`^(\d{4})-.*\.patch$`)

// Patch writes the whole change against Omarchy's sources as one patch in the kernel's
// format, named and numbered for the recipe. subject is the patch's first line, which starts
// "arm64: dts:" as every tree patch does, and body says why. The author is the owner, as
// their git names them.
func (s System) Patch(dir, subject, body string) (Patched, error) {
	root := filepath.Join(dir, "tree")
	patched := Patched{Recipe: s.recipe(dir)}
	if _, err := os.Stat(root); err != nil {
		return patched, fmt.Errorf("there are no sources in %s yet. Start with: hw-test tree start", root)
	}
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, "arm64: dts: ") || strings.Contains(subject, "\n") {
		return patched, fmt.Errorf("the patch needs a first line that names the tree it changes, like: hw-test tree patch \"arm64: dts: qcom: x1e80100-lenovo-yoga-slim7x: Route the right woofer from its own amplifier\"")
	}
	var who [2]string
	for at, key := range []string{"user.name", "user.email"} {
		out, _ := run(root, "git", "config", "--get", key)
		if who[at] = strings.TrimSpace(out); who[at] == "" {
			return patched, fmt.Errorf("git doesn't know the owner's %s, and a patch names who wrote it. The owner sets it once: git config --global %s \"...\"", strings.TrimPrefix(key, "user."), key)
		}
	}
	from, err := base(root)
	if err != nil {
		return patched, err
	}
	// The whole change as one commit over Omarchy's sources, however often this is run
	for _, git := range [][]string{{"add", "-A"}, {"reset", "-q", "--soft", from}} {
		if _, err := run(root, "git", git...); err != nil {
			return patched, err
		}
	}
	names, _ := run(root, "git", "diff", "--cached", "--name-only")
	patched.Changed = strings.Fields(names)
	if len(patched.Changed) == 0 {
		return patched, fmt.Errorf("nothing differs from Omarchy's sources, so there is nothing to make a patch of")
	}
	for _, name := range patched.Changed {
		if !strings.HasPrefix(name, "arch/arm64/boot/dts/") && !strings.HasPrefix(name, "include/dt-bindings/") {
			return patched, fmt.Errorf("%s is changed, and a device tree patch may only touch arch/arm64/boot/dts/ and include/dt-bindings/. Put it back: git -C %s checkout %s -- %s", name, root, from, name)
		}
	}
	commit := []string{"-c", "user.name=" + who[0], "-c", "user.email=" + who[1], "commit", "-q", "-m", subject}
	if body = strings.TrimSpace(body); body != "" {
		commit = append(commit, "-m", body)
	}
	if _, err := run(root, "git", commit...); err != nil {
		return patched, err
	}
	out, err := os.MkdirTemp("", "hw-test-patch-")
	if err != nil {
		return patched, err
	}
	defer os.RemoveAll(out)
	if _, err := run(root, "git", "format-patch", "-q", "-1", "--zero-commit", "-o", out); err != nil {
		return patched, err
	}
	made, _ := filepath.Glob(filepath.Join(out, "0001-*.patch"))
	if len(made) != 1 {
		return patched, fmt.Errorf("git made no patch of the change")
	}
	// The next free number among the recipe's patches
	number := 1
	if listed, err := filepath.Glob(filepath.Join(patched.Recipe, "*.patch")); err == nil && len(listed) > 0 {
		sort.Strings(listed)
		for _, file := range listed {
			if match := patchNumber.FindStringSubmatch(filepath.Base(file)); match != nil {
				if n, _ := strconv.Atoi(match[1]); n >= number {
					number = n + 1
				}
			}
		}
		patched.Numbered = true
	}
	text, err := os.ReadFile(made[0])
	if err != nil {
		return patched, err
	}
	patched.File = filepath.Join(dir, fmt.Sprintf("%04d-%s", number, strings.TrimPrefix(filepath.Base(made[0]), "0001-")))
	// An earlier writing of the same change, under whatever name, is replaced
	if old, _ := filepath.Glob(filepath.Join(dir, "[0-9][0-9][0-9][0-9]-*.patch")); len(old) > 0 {
		for _, file := range old {
			os.Remove(file)
		}
	}
	return patched, os.WriteFile(patched.File, text, 0o644)
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
  --cmdline ` + quote(strings.TrimSpace(cmdline)) + ` --devicetree "$dtb" --uname "$kernel" --output "$work/` + Entry + `.efi"

echo "Adding the entry to the boot menu..."
touch "$work/before"
limine-entry-tool --add-uki ` + quote(Entry) + ` "$work/` + Entry + `.efi" --comment "hw-test: a changed device tree to try"

` + onceScript + `
echo
if once; then
  echo "Done. Restart: the test entry starts by itself, this one time. Nothing has to be picked."
  echo "The start after that is the usual one again, also if the test entry doesn't come up:"
  echo "then hold the power button until the computer is off, and start it again."
else
  echo "Done. Restart, and in the boot menu pick ` + Entry + ` (it is listed under Omarchy, after the usual kernel)."
  echo "The usual entry is untouched and still the default. If the test entry doesn't start,"
  echo "hold the power button until the computer is off, start it again and let the usual entry run."
fi
echo "After the restart, hw-test tree status says which one is running. To take the test entry away: hw-test tree remove"
`
}

// Once is the name of the firmware's entry for starting the test image one time.
const Once = "hw-test once"

// onceScript tells the computer's firmware to start the test image at the next restart, one
// time, so nobody has to be there to pick it from the boot menu. The firmware forgets it
// after that start, whether or not the image came up, and the usual order is never changed.
const onceScript = `once() {
  command -v efibootmgr >/dev/null || return 1
  local image part number disk id
  image=$(find /boot -type f -iname '*hwtest*.efi' -newer "$work/before" 2>/dev/null | head -1)
  [ -n "$image" ] || image=$(find /boot -type f -iname '*hwtest*.efi' 2>/dev/null | head -1)
  [ -n "$image" ] || return 1
  part=$(findmnt -no SOURCE /boot) || return 1
  number=$(cat "/sys/class/block/$(basename "$part")/partition") || return 1
  disk=/dev/$(lsblk -no PKNAME "$part") || return 1
  for id in $(efibootmgr | sed -n 's/^Boot\([0-9A-Fa-f]\{4\}\)\*\{0,1\} ` + Once + `.*/\1/p'); do
    efibootmgr -q -b "$id" -B || true
  done
  efibootmgr -q --create-only --disk "$disk" --part "$number" --label '` + Once + `' --loader "$(printf '%s' "${image#/boot}" | tr / '\\')" || return 1
  id=$(efibootmgr | sed -n 's/^Boot\([0-9A-Fa-f]\{4\}\)\*\{0,1\} ` + Once + `.*/\1/p' | head -1)
  [ -n "$id" ] && efibootmgr -q --bootnext "$id"
}`

// RemoveScript takes the test entry and its image away again.
func RemoveScript() string {
	return `#!/bin/bash
# Written by hw-test tree remove. Takes the test entry "` + Entry + `" and its boot image away.
set -euo pipefail
limine-entry-tool --remove-uki '` + Entry + `'
# The firmware's entry for starting it once goes too, and its word to start it next
if command -v efibootmgr >/dev/null; then
  for id in $(efibootmgr | sed -n 's/^Boot\([0-9A-Fa-f]\{4\}\)\*\{0,1\} ` + Once + `.*/\1/p'); do
    if efibootmgr | grep -qi "^BootNext: $id"; then efibootmgr -q --delete-bootnext || true; fi
    efibootmgr -q -b "$id" -B || true
  done
fi
echo "The test entry is gone. The boot menu is as it was before."
`
}
