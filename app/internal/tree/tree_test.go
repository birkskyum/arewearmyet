package tree

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	// Omarchy's package: one patch that changes this machine's tree, one that adds a model the
	// kernel doesn't have, and one the kernel's sources already hold
	recipe := filepath.Join(t.TempDir(), "linux-omarchy-dtbs")
	os.MkdirAll(recipe, 0o755)
	patch := func(name, text string) {
		if err := os.WriteFile(filepath.Join(recipe, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	patch("0001-arm64-dts-qcom-yoga-slim7x-Name-the-model-in-full.patch", `--- a/arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts
+++ b/arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts
@@ -1,6 +1,6 @@
 #include <dt-bindings/gpio/gpio.h>
 / {
-	model = "Lenovo Yoga Slim 7x";
+	model = "Lenovo Yoga Slim 7x 14Q8X9";
 	compatible = "lenovo,yoga-slim7x", "qcom,x1e80100";
 	reset = <GPIO_ACTIVE_LOW>;
 };
`)
	patch("0002-arm64-dts-qcom-Add-the-HP-EliteBook-6-G2q.patch", `--- /dev/null
+++ b/arch/arm64/boot/dts/qcom/mahua-hp-elitebook-6-g2q.dts
@@ -0,0 +1,3 @@
+/ {
+	compatible = "hp,elitebook-6-g2q", "qcom,mahua";
+};
`)
	patch("0003-arm64-dts-qcom-xps13-Already-in-the-kernel.patch", `--- a/arch/arm64/boot/dts/qcom/x1e80100-dell-xps13-9345.dts
+++ b/arch/arm64/boot/dts/qcom/x1e80100-dell-xps13-9345.dts
@@ -1,3 +1,3 @@
 / {
-	compatible = "dell,xps13-9345";
+	compatible = "dell,xps13-9345", "qcom,x1e80100";
 };
`)
	os.WriteFile(filepath.Join(recipe, "PKGBUILD"), []byte("pkgname=linux-omarchy-dtbs\nsource=(\n  0001-arm64-dts-qcom-yoga-slim7x-Name-the-model-in-full.patch\n  0002-arm64-dts-qcom-Add-the-HP-EliteBook-6-G2q.patch\n  0003-arm64-dts-qcom-xps13-Already-in-the-kernel.patch\n  x1e80100-lenovo-yoga-slim7x.json\n)\nb2sums=(\n  abc\n)\n"), 0o644)
	return System{Kernel: "7.3.0-59-omarchy", Headers: build, UKIConf: conf, Compatible: "lenovo,yoga-slim7x", Recipe: recipe, Installed: filepath.Join(t.TempDir(), "linux-omarchy-dtbs-*", "mtree")}
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
	started, err := s.Start(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := started.Root
	// Omarchy's patches are laid over the kernel's sources, as its package does: two applied,
	// and the one the kernel already has passed over
	if started.Applied != 2 || len(started.Skipped) != 1 || !strings.Contains(started.Skipped[0], "0003-") {
		t.Errorf("patches: %d applied, passed over %v", started.Applied, started.Skipped)
	}
	if text, _ := os.ReadFile(filepath.Join(root, "arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts")); !strings.Contains(string(text), "Yoga Slim 7x 14Q8X9") {
		t.Errorf("the patch to this machine's tree isn't in the sources:\n%s", text)
	}
	if _, err := os.Stat(filepath.Join(root, "arch/arm64/boot/dts/qcom/mahua-hp-elitebook-6-g2q.dts")); err != nil {
		t.Error("the model Omarchy's patch adds isn't among the sources")
	}
	if started.Machine != "x1e80100-lenovo-yoga-slim7x" || started.Stock != "" {
		t.Errorf("machine %q, against the installed tree %q with none installed", started.Machine, started.Stock)
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
	// Every tree built here carries the mark, and neither the sources nor the diff do
	if !strings.Contains(string(tree), "/ { hw-test-tree; };") {
		t.Errorf("the built tree lacks its mark:\n%s", tree)
	}
	if out, err := run(root, "git", "grep", "-c", "hw-test-tree"); err == nil {
		t.Errorf("the mark got into the sources: %s", out)
	}
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
		"ukify build --config /dev/null", `--devicetree "$dtb"`, "--cmdline 'root=/dev/mapper/root rw quiet'",
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
	// The mark in the running tree is how a boot of the test tree is told from the usual one
	running := t.TempDir()
	if Running(running) {
		t.Error("the usual tree was taken for a test tree")
	}
	os.WriteFile(filepath.Join(running, Mark), nil, 0o444)
	if !Running(running) {
		t.Error("the test tree's mark wasn't seen")
	}
	remove := RemoveScript()
	if !strings.Contains(remove, "limine-entry-tool --remove-uki 'linux-omarchy-hwtest'") || strings.Contains(remove, "limine.conf") {
		t.Errorf("the removal:\n%s", remove)
	}
}

func TestStartSaysWhetherTheSourcesAreWhatTheComputerStartsWith(t *testing.T) {
	s := kernel(t)
	// The installed tree is what these sources compile to, without the mark
	first := t.TempDir()
	if _, err := s.Start(first); err != nil {
		t.Fatal(err)
	}
	if err := s.compile(filepath.Join(first, "tree"), "x1e80100-lenovo-yoga-slim7x", filepath.Join(first, "stock"), false); err != nil {
		t.Fatal(err)
	}
	stock, _ := os.ReadFile(filepath.Join(first, "stock.dtb"))
	if strings.Contains(string(stock), Mark) {
		t.Fatal("the stock tree carries the mark")
	}
	// The package manager's record of what it installed, as pacman keeps it
	record := func(tree []byte) {
		sum := sha256.Sum256(tree)
		file := strings.Replace(s.Installed, "*", "7.3-15", 1)
		os.MkdirAll(filepath.Dir(file), 0o755)
		var packed bytes.Buffer
		zip := gzip.NewWriter(&packed)
		fmt.Fprintf(zip, "#mtree\n/set type=file uid=0 gid=0 mode=644\n./boot/dtbs/linux-omarchy/qcom/glymur-lenovo-yoga-slim7x.dtb time=1.0 size=3 sha256digest=00\n./boot/dtbs/linux-omarchy/qcom/x1e80100-lenovo-yoga-slim7x.dtb time=1.0 size=%d sha256digest=%s\n", len(tree), hex.EncodeToString(sum[:]))
		zip.Close()
		if err := os.WriteFile(file, packed.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	record(stock)
	dir := t.TempDir()
	if started, err := s.Start(dir); err != nil || started.Stock != "same" {
		t.Errorf("with the same tree installed: %q, %v", started.Stock, err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".stock-*")); len(left) != 0 {
		t.Errorf("the comparison left files behind: %v", left)
	}
	// An older package is installed: the sources are ahead of what the computer starts with
	record([]byte("an older tree"))
	if started, err := s.Start(t.TempDir()); err != nil || started.Stock != "differs" {
		t.Errorf("with another tree installed: %q, %v", started.Stock, err)
	}

	// A model whose tree only Omarchy's patches add is found among the patched sources
	s.UKIConf, s.Compatible = "/nonexistent", "hp,elitebook-6-g2q"
	if _, err := s.Machine(); err == nil {
		t.Error("the kernel's own sources had a tree only a patch adds")
	}
	if got, err := s.MachineIn(first); err != nil || got != "mahua-hp-elitebook-6-g2q" {
		t.Errorf("among the patched sources: %q, %v", got, err)
	}
	if got, err := s.MachineIn(t.TempDir()); err == nil {
		t.Errorf("before starting, it was found as %q", got)
	}

	// A patch that doesn't fit these sources is left out whole, and named
	os.WriteFile(filepath.Join(s.Recipe, "0002-arm64-dts-qcom-Add-the-HP-EliteBook-6-G2q.patch"), []byte("--- a/arch/arm64/boot/dts/qcom/x1e80100-dell-xps13-9345.dts\n+++ b/arch/arm64/boot/dts/qcom/x1e80100-dell-xps13-9345.dts\n@@ -1,3 +1,3 @@\n / {\n-\tcompatible = \"another kernel's line\";\n+\tcompatible = \"x\";\n };\n"), 0o644)
	behind, err := s.Start(t.TempDir())
	if err != nil || len(behind.Misfit) != 1 || !strings.Contains(behind.Misfit[0], "0002-") || behind.Applied != 1 {
		t.Errorf("a patch that doesn't fit: left out %v, %d applied, %v", behind.Misfit, behind.Applied, err)
	}
	if out, _ := run(behind.Root, "git", "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("the patch left something behind: %s", out)
	}
	// Without the package there are no patches to build with
	s.Recipe = filepath.Join(t.TempDir(), "none")
	if _, err := s.Start(t.TempDir()); err == nil || !strings.Contains(err.Error(), "no device tree package") {
		t.Errorf("without the package: %v", err)
	}
}

func TestTheChangeIsWrittenAsOnePatchForThePackage(t *testing.T) {
	s := kernel(t)
	dir := t.TempDir()
	started, err := s.Start(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := started.Root
	subject := "arm64: dts: qcom: x1e80100-lenovo-yoga-slim7x: Hold the reset line high"
	// The owner's name is on the patch, so git has to know it
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "none"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	source := filepath.Join(root, "arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts")
	text, _ := os.ReadFile(source)
	os.WriteFile(source, []byte(strings.Replace(string(text), "GPIO_ACTIVE_LOW", "0", 1)), 0o644)
	if _, err := s.Patch(dir, subject, ""); err == nil || !strings.Contains(err.Error(), "git config --global user.name") {
		t.Errorf("without a name: %v", err)
	}
	global := filepath.Join(t.TempDir(), "gitconfig")
	os.WriteFile(global, []byte("[user]\n\tname = Ada Owner\n\temail = ada@example.org\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)

	// A first line that doesn't name the tree is refused, with an example
	if _, err := s.Patch(dir, "Fix the speaker", ""); err == nil || !strings.Contains(err.Error(), "arm64: dts: qcom:") {
		t.Errorf("a vague first line: %v", err)
	}
	patched, err := s.Patch(dir, subject, "The amplifier's reset is active high on this board.\n\nMeasured with hw-test speakers.")
	if err != nil {
		t.Fatal(err)
	}
	// Numbered after the package's last patch, named as git names it
	if name := filepath.Base(patched.File); !strings.HasPrefix(name, "0004-arm64-dts-qcom-x1e80100-lenovo-yoga-slim7x-Hold-the") || !patched.Numbered {
		t.Errorf("the patch's name: %s, numbered %v", name, patched.Numbered)
	}
	patch, _ := os.ReadFile(patched.File)
	for _, want := range []string{"From: Ada Owner <ada@example.org>", "Subject: [PATCH] " + subject[:40], "The amplifier's reset is active high", "-\treset = <GPIO_ACTIVE_LOW>;", "+\treset = <0>;", "From 0000000000000000000000000000000000000000"} {
		if !strings.Contains(string(patch), want) {
			t.Errorf("the patch lacks %q:\n%s", want, patch)
		}
	}
	// It holds the change alone: neither Omarchy's own patches nor the mark
	if strings.Contains(string(patch), "+\tmodel") || strings.Contains(string(patch), "elitebook") || strings.Contains(string(patch), Mark) {
		t.Errorf("the patch holds more than the change:\n%s", patch)
	}
	// It fits the package's sources: laid over a fresh start, it gives the same tree
	fresh := t.TempDir()
	again, err := s.Start(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := run(again.Root, "patch", "-Np1", "--fuzz=0", "-s", "-i", patched.File); err != nil {
		t.Fatalf("the patch doesn't fit Omarchy's sources: %v %s", err, out)
	}
	a, _ := os.ReadFile(source)
	b, _ := os.ReadFile(filepath.Join(again.Root, "arch/arm64/boot/dts/qcom/x1e80100-lenovo-yoga-slim7x.dts"))
	if string(a) != string(b) {
		t.Error("the patched fresh sources differ from the changed ones")
	}

	// Going on after the patch: the build's diff is still the whole change, and a second
	// writing replaces the first with everything in it
	built, err := s.Build(dir, "x1e80100-lenovo-yoga-slim7x")
	if err != nil || len(built.Changed) != 1 {
		t.Fatalf("building after the patch: changed %v, %v", built.Changed, err)
	}
	os.WriteFile(filepath.Join(root, "arch/arm64/boot/dts/qcom/extra.dtsi"), []byte("/* more */\n"), 0o644)
	second, err := s.Patch(dir, subject+" and more", "")
	if err != nil {
		t.Fatal(err)
	}
	patch, _ = os.ReadFile(second.File)
	if !strings.Contains(string(patch), "extra.dtsi") || !strings.Contains(string(patch), "+\treset = <0>;") || len(second.Changed) != 2 {
		t.Errorf("the second patch isn't the whole change:\n%s", patch)
	}
	if all, _ := filepath.Glob(filepath.Join(dir, "0*.patch")); len(all) != 1 {
		t.Errorf("patches left in the folder: %v", all)
	}
	if count, _ := run(root, "git", "rev-list", "--count", "HEAD"); strings.TrimSpace(count) != "2" {
		t.Errorf("commits over Omarchy's sources: %s", count)
	}

	// A change outside the device trees isn't one for this package
	os.WriteFile(filepath.Join(root, "scripts/dtc/notes"), []byte("x\n"), 0o644)
	if _, err := s.Patch(dir, subject, ""); err == nil || !strings.Contains(err.Error(), "may only touch") {
		t.Errorf("a file outside the trees: %v", err)
	}
	// And with nothing changed there is nothing to write
	if _, err := s.Patch(fresh+"-none", subject, ""); err == nil {
		t.Error("a patch was written without sources")
	}
}

func TestTheTestEntryStartsOnceByItselfAndTheUsualOrderIsLeftAlone(t *testing.T) {
	script := System{Kernel: "7.3.0-72-omarchy"}.Script("/home/owner/hw-bringup/x1e80100-lenovo-yoga-slim7x.dtb", "root=/dev/mapper/root rw")
	for _, part := range []string{
		// The firmware is told to start the test image next, one time, from an entry of its own
		`efibootmgr -q --create-only --disk "$disk" --part "$number" --label 'hw-test once'`, `efibootmgr -q --bootnext "$id"`,
		// An entry left from an earlier try goes first, and the image is the one just added
		`efibootmgr -q -b "$id" -B || true`, `-newer "$work/before"`,
		"Restart: the test entry starts by itself, this one time. Nothing has to be picked.",
		// Where the firmware can't be told, the boot menu is still the way
		"in the boot menu pick linux-omarchy-hwtest",
	} {
		if !strings.Contains(script, part) {
			t.Errorf("the boot script lacks %q", part)
		}
	}
	// The order the computer starts in is never written: only the one-time word is
	for _, never := range []string{"--bootorder", " -o ", "efibootmgr -q --create "} {
		if strings.Contains(script, never) {
			t.Errorf("the boot script changes the usual order: %q", never)
		}
	}
	// Taking the test entry away takes the firmware's entry and its one-time word too
	removal := RemoveScript()
	for _, part := range []string{"limine-entry-tool --remove-uki 'linux-omarchy-hwtest'", "--delete-bootnext", `efibootmgr -q -b "$id" -B`} {
		if !strings.Contains(removal, part) {
			t.Errorf("the remove script lacks %q", part)
		}
	}
}
