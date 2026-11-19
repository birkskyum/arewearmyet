package tree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// kernel is a stand-in for a kernel's build folder: two tree sources, a bindings header one of
// them includes, and a dtc that writes what it was given, so the flow is tested without a
// device tree compiler.
func kernel(t *testing.T) System {
	t.Helper()
	for _, tool := range []string{"gcc", "git", "cp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s isn't installed", tool)
		}
	}
	build := filepath.Join(t.TempDir(), "build")
	file := func(path, text string, mode os.FileMode) {
		full := filepath.Join(build, path)
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	file("arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts", "#include <dt-bindings/gpio/gpio.h>\n/ {\n\tmodel = \"Lenovo Yoga Slim 7x\";\n\tcompatible = \"lenovo,yoga-slim7x\", \"qcom,x1e80100\";\n\treset = <GPIO_ACTIVE_LOW>;\n};\n", 0o644)
	file("arch/arm64/boot/dts/qcom/x1e80100-dell-xps13-9345.dts", "/ {\n\tcompatible = \"dell,xps13-9345\", \"qcom,x1e80100\";\n};\n", 0o644)
	file("include/dt-bindings/gpio/gpio.h", "#define GPIO_ACTIVE_LOW 1\n", 0o644)
	os.MkdirAll(filepath.Join(build, "scripts/dtc/include-prefixes"), 0o755)
	if err := os.Symlink("../../../include/dt-bindings", filepath.Join(build, "scripts/dtc/include-prefixes/dt-bindings")); err != nil {
		t.Fatal(err)
	}
	// Stands in for dtc: copies the preprocessed source to where the tree should go
	file("scripts/dtc/dtc", "#!/bin/sh\nfor a in \"$@\"; do last=$a; done\nwhile [ $# -gt 0 ]; do [ \"$1\" = -o ] && out=$2; shift; done\ncp \"$last\" \"$out\"\n", 0o755)
	conf := filepath.Join(t.TempDir(), "uki.conf")
	os.WriteFile(conf, []byte("[UKI]\nDeviceTree=/boot/dtbs/linux-omarchy/qcom/x1e80100-lenovo-yoga-slim7x.dtb\nDeviceTreeAuto=/boot/dtbs/linux-omarchy/qcom/x1e80100-dell-xps13-9345.dtb\n"), 0o644)
	return System{Kernel: "7.3.0-59-omarchy", Headers: build, UKIConf: conf, Compatible: "lenovo,yoga-slim7x"}
}

func TestTheMachinesTreeIsFoundFromTheBootImageSettingsOrItsName(t *testing.T) {
	s := kernel(t)
	if got, err := s.Machine(); err != nil || got != "x1e80100-lenovo-yoga-slim7x" {
		t.Errorf("from the settings: %q, %v", got, err)
	}
	// Without the settings, by the name the running tree gives the machine
	s.UKIConf = "/nonexistent"
	s.Compatible = "dell,xps13-9345"
	if got, err := s.Machine(); err != nil || got != "x1e80100-dell-xps13-9345" {
		t.Errorf("from the name: %q, %v", got, err)
	}
	s.Compatible = "acme,unheard-of"
	if _, err := s.Machine(); err == nil || !strings.Contains(err.Error(), "hw-test tree build <name>") {
		t.Errorf("an unknown machine: %v", err)
	}
	// Without the kernel's headers there is nothing to build from, and it says what to install
	if err := (System{Headers: "/nonexistent"}).Ready(); err == nil || !strings.Contains(err.Error(), "linux-omarchy-headers") {
		t.Errorf("without headers: %v", err)
	}
}

func TestAChangedTreeIsBuiltAsThePackageDoesWithItsDiff(t *testing.T) {
	s := kernel(t)
	dir := t.TempDir()
	root, err := s.Start(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The sources are copied with their links resolved, and under git from the start
	if info, err := os.Lstat(filepath.Join(root, "include/dt-bindings/gpio/gpio.h")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("the bindings weren't copied: %v", err)
	}
	if out, err := run(root, "git", "status", "--porcelain"); err != nil || strings.TrimSpace(out) != "" {
		t.Fatalf("the copy isn't a clean start: %q, %v", out, err)
	}
	// Starting again doesn't write over work in progress
	if _, err := s.Start(dir); err == nil || !strings.Contains(err.Error(), "there already") {
		t.Errorf("a second start: %v", err)
	}

	// Unchanged, it builds, and the diff is empty
	built, err := s.Build(dir, "x1e80100-lenovo-yoga-slim7x")
	if err != nil {
		t.Fatal(err)
	}
	tree, _ := os.ReadFile(built.DTB)
	if !strings.Contains(string(tree), "reset = <1>") || len(built.Changed) != 0 {
		t.Fatalf("the stock build: changed %v\n%s", built.Changed, tree)
	}
	if diff, _ := os.ReadFile(built.Diff); len(diff) != 0 {
		t.Errorf("a diff with nothing changed:\n%s", diff)
	}

	// A change to the tree, and a new file beside it
	source := filepath.Join(root, "arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts")
	text, _ := os.ReadFile(source)
	os.WriteFile(source, []byte(strings.Replace(string(text), "GPIO_ACTIVE_LOW", "0", 1)), 0o644)
	os.WriteFile(filepath.Join(root, "arch/arm64/boot/dts/qcom/notes.dtsi"), []byte("/* a new include */\n"), 0o644)
	built, err = s.Build(dir, "x1e80100-lenovo-yoga-slim7x")
	if err != nil {
		t.Fatal(err)
	}
	tree, _ = os.ReadFile(built.DTB)
	diff, _ := os.ReadFile(built.Diff)
	if !strings.Contains(string(tree), "reset = <0>") {
		t.Errorf("the change isn't in the built tree:\n%s", tree)
	}
	for _, want := range []string{"-\treset = <GPIO_ACTIVE_LOW>;", "+\treset = <0>;", "a/arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts", "notes.dtsi"} {
		if !strings.Contains(string(diff), want) {
			t.Errorf("the diff lacks %q:\n%s", want, diff)
		}
	}
	if len(built.Changed) != 2 {
		t.Errorf("changed files: %v", built.Changed)
	}
	// Nothing of the build is left in the sources, so the diff holds only the change
	if _, err := os.Stat(filepath.Join(dir, "x1e80100-lenovo-yoga-slim7x.pre.dts")); err == nil {
		t.Error("the preprocessed source was left behind")
	}

	// A source that doesn't compile says why, and a tree that isn't there says so
	os.WriteFile(source, []byte("#include <dt-bindings/no/such.h>\n"), 0o644)
	if _, err := s.Build(dir, "x1e80100-lenovo-yoga-slim7x"); err == nil || !strings.Contains(err.Error(), "such.h") {
		t.Errorf("a broken source: %v", err)
	}
	if _, err := s.Build(dir, "no-such-tree"); err == nil {
		t.Error("a tree that isn't there built")
	}
	if _, err := s.Build(t.TempDir(), "x1e80100-lenovo-yoga-slim7x"); err == nil || !strings.Contains(err.Error(), "hw-test tree start") {
		t.Errorf("building before starting: %v", err)
	}
}

func TestTheOwnersScriptMakesAnEntryOfItsOwnAndLeavesTheStockOneAlone(t *testing.T) {
	s := kernel(t)
	script := s.Script("/home/o'brien/hw-bringup/x1e80100-lenovo-yoga-slim7x.dtb", "root=/dev/mapper/root rw quiet")
	for _, want := range []string{
		"set -euo pipefail",
		// Room on the boot partition is checked before anything is written there
		"df --output=avail -m /boot", "exit 1",
		`mkinitcpio -k "$kernel" -g "$work/initramfs.img"`,
		// The system's settings are left out, or the stock tree would be picked over the changed one
		"ukify build --config /dev/null", `--devicetree "$dtb"`, "--cmdline 'root=/dev/mapper/root rw quiet hwtest.tree=1'",
		// With Direct Boot there is no menu to pick the entry from, so it stops and says so
		"omarchy-boot-direct", "Direct Boot is on",
		// Through the boot menu's own tool, under a name that sorts after the stock kernel
		"limine-entry-tool --add-uki 'linux-omarchy-hwtest'",
		`dtb='/home/o'\''brien/hw-bringup/x1e80100-lenovo-yoga-slim7x.dtb'`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the script lacks %q", want)
		}
	}
	// It never edits the menu's file by hand, removes the stock entry or sets a default
	for _, never := range []string{"limine.conf", "--remove", "omarchy kernel set", "default_entry", "/boot/EFI/Linux/omarchy"} {
		if strings.Contains(script, never) {
			t.Errorf("the script touches %q", never)
		}
	}
	// It is a script bash accepts
	file := filepath.Join(t.TempDir(), "entry.sh")
	os.WriteFile(file, []byte(script), 0o755)
	if out, err := exec.Command("bash", "-n", file).CombinedOutput(); err != nil {
		t.Errorf("bash doesn't accept it: %v\n%s", err, out)
	}
	// The mark on the command line is how a boot of the test image is told from the usual one
	if !Running("root=/dev/mapper/root rw quiet hwtest.tree=1") || Running("root=/dev/mapper/root rw quiet") || Running("nothwtest.tree=1x") {
		t.Error("the mark isn't read right")
	}
	remove := RemoveScript()
	if !strings.Contains(remove, "limine-entry-tool --remove-uki 'linux-omarchy-hwtest'") || strings.Contains(remove, "limine.conf") {
		t.Errorf("the removal:\n%s", remove)
	}
}
