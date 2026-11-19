package kernel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stubs puts stand-ins for the module tools first on the PATH. Each writes what it was asked
// to a log, and fails when a file named after it and its last argument exists in the fail folder.
func stubs(t *testing.T) (dir, log string) {
	t.Helper()
	dir = t.TempDir()
	log = filepath.Join(dir, "calls")
	stub := `#!/bin/bash
echo "$(basename "$0") $*" >> ` + log + `
[ ! -e "` + dir + `/fail/$(basename "$0")-$(basename "${!#}")" ] || exit 1
case "$(basename "$0")" in
  modinfo) case "$2" in
    name) basename "$3" .ko | tr - _ ;;
    depends) cat "` + dir + `/depends" 2>/dev/null ;;
  esac ;;
esac
exit 0
`
	for _, name := range []string{"rmmod", "insmod", "modprobe", "modinfo"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(dir, "fail"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return dir, log
}

func script(t *testing.T, text string) (string, error) {
	t.Helper()
	out, err := exec.Command("bash", "-c", text).CombinedOutput()
	return string(out), err
}

func calls(t *testing.T, log string) string {
	data, _ := os.ReadFile(log)
	os.Remove(log)
	return strings.TrimSpace(string(data))
}

func TestABuiltModuleIsLoadedInPlaceOfTheKernelsOwnAndNoted(t *testing.T) {
	dir, log := stubs(t)
	system := System{Marker: filepath.Join(dir, "run", "test-modules")}
	os.WriteFile(filepath.Join(dir, "depends"), []byte("phy_core,usb_common\n"), 0o644)
	module := filepath.Join(dir, "phy-qcom-qmp-usb.ko")

	// Not loaded before: nothing to take out, what it builds on comes first
	if out, err := script(t, system.LoadScript([]string{module})); err != nil {
		t.Fatalf("load: %v\n%s", err, out)
	}
	if got := calls(t, log); !strings.Contains(got, "modprobe phy_core\nmodprobe usb_common\ninsmod "+module) || strings.Contains(got, "rmmod") {
		t.Errorf("a module that wasn't loaded:\n%s", got)
	}
	if loaded := system.Loaded(); len(loaded) != 1 || loaded[0] != "phy_qcom_qmp_usb" {
		t.Errorf("noted as loaded: %v", loaded)
	}

	// A new module that won't load gets the kernel's own put back, and isn't noted
	os.Remove(system.Marker)
	os.WriteFile(filepath.Join(dir, "fail", "insmod-"+filepath.Base(module)), nil, 0o644)
	out, err := script(t, system.LoadScript([]string{module}))
	if err == nil || !strings.Contains(out, "Putting the kernel's own back") {
		t.Errorf("a module that won't load: %v\n%s", err, out)
	}
	if got := calls(t, log); !strings.HasSuffix(got, "modprobe phy_qcom_qmp_usb") {
		t.Errorf("the kernel's own wasn't put back:\n%s", got)
	}
	if loaded := system.Loaded(); len(loaded) != 0 {
		t.Errorf("a module that didn't load is noted: %v", loaded)
	}
}

func TestTheKernelsOwnModulesComeBackNewestFirst(t *testing.T) {
	dir, log := stubs(t)
	system := System{Marker: filepath.Join(dir, "run", "test-modules")}
	os.MkdirAll(filepath.Dir(system.Marker), 0o755)
	os.WriteFile(system.Marker, []byte("phy_a /x/phy-a.ko\nsnd_b /x/snd-b.ko\nphy_a /x/phy-a.ko\n"), 0o644)
	if loaded := system.Loaded(); strings.Join(loaded, ",") != "phy_a,snd_b" {
		t.Errorf("loaded: %v", loaded)
	}
	if out, err := script(t, system.RestoreScript()); err != nil {
		t.Fatalf("restore: %v\n%s", err, out)
	}
	if got := calls(t, log); got != "modprobe phy_a\nmodprobe snd_b" {
		t.Errorf("restored in this order:\n%s", got)
	}
	if _, err := os.Stat(system.Marker); err == nil {
		t.Error("the note of loaded modules is still there")
	}

	// With nothing loaded there is nothing to do
	if out, _ := script(t, system.RestoreScript()); !strings.Contains(out, "No test module is loaded") {
		t.Errorf("with nothing loaded: %s", out)
	}
}

func TestTheChangeIsWrittenAsOnePatchNumberedAfterTheRecipesLast(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	git := func(at string, args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = at
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	recipe := RecipeIn(dir)
	source := filepath.Join(recipe, "src", "linux-7.3")
	os.MkdirAll(filepath.Join(source, "drivers", "phy"), 0o755)
	os.WriteFile(filepath.Join(source, "Makefile"), []byte("# kernel\n"), 0o644)
	os.WriteFile(filepath.Join(source, "drivers", "phy", "phy.c"), []byte("int supply;\n"), 0o644)
	os.WriteFile(filepath.Join(recipe, "2921-clk-something.patch"), nil, 0o644)
	os.WriteFile(filepath.Join(recipe, "0010-base.patch"), nil, 0o644)
	git(source, "init", "-q")
	git(source, "add", "-A")
	git(source, "-c", "user.name=hw-test", "-c", "user.email=hw-test@localhost", "commit", "-q", "-m", "base")
	system := System{}

	if Source(dir) != source {
		t.Fatalf("the source wasn't found: %q", Source(dir))
	}
	// Without a name for the owner, a patch can't say who wrote it
	os.WriteFile(filepath.Join(source, "drivers", "phy", "phy.c"), []byte("int supply = 1;\n"), 0o644)
	if _, err := system.Patch(dir, "phy: Add the supply", "It was missing."); err == nil || !strings.Contains(err.Error(), "git config --global") {
		t.Fatalf("without the owner's name: %v", err)
	}
	git(dir, "config", "--global", "user.name", "An Owner")
	git(dir, "config", "--global", "user.email", "owner@example.org")
	if _, err := system.Patch(dir, "add the supply", ""); err == nil {
		t.Error("a first line that names no subsystem was taken")
	}
	patched, err := system.Patch(dir, "phy: Add the supply", "It was missing.")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(patched.File) != "2922-phy-Add-the-supply.patch" || filepath.Dir(patched.File) != filepath.Join(dir, "kernel") || strings.Join(patched.Changed, ",") != "drivers/phy/phy.c" {
		t.Errorf("patch: %+v", patched)
	}
	text, _ := os.ReadFile(patched.File)
	for _, want := range []string{"From: An Owner <owner@example.org>", "Subject: [PATCH] phy: Add the supply", "It was missing.", "+int supply = 1;"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the patch lacks %q:\n%s", want, text)
		}
	}
	// Written again after another change, it is still one patch over Omarchy's source
	os.WriteFile(filepath.Join(source, "drivers", "phy", "phy.c"), []byte("int supply = 2;\n"), 0o644)
	again, err := system.Patch(dir, "phy: Add the supply", "It was missing.")
	if err != nil {
		t.Fatal(err)
	}
	text, _ = os.ReadFile(again.File)
	left, _ := filepath.Glob(filepath.Join(dir, "kernel", "*.patch"))
	if len(left) != 1 || !strings.Contains(string(text), "+int supply = 2;") || strings.Contains(string(text), "supply = 1") {
		t.Errorf("written again: %v\n%s", left, text)
	}
}

func TestBuildingNeedsTheSourceAndAFolder(t *testing.T) {
	dir := t.TempDir()
	system := System{Kernel: "7.3.0-72-omarchy", Headers: filepath.Join(dir, "headers")}
	if _, err := system.Build(dir, []string{"drivers/phy"}); err == nil || !strings.Contains(err.Error(), "hw-test kernel start") {
		t.Errorf("without the source: %v", err)
	}
	source := filepath.Join(RecipeIn(dir), "src", "linux-7.3")
	os.MkdirAll(filepath.Join(source, "drivers", "phy"), 0o755)
	os.WriteFile(filepath.Join(source, "Makefile"), []byte("# kernel\n"), 0o644)
	if _, err := system.Build(dir, nil); err == nil || !strings.Contains(err.Error(), "name the driver's folder") {
		t.Errorf("without a folder: %v", err)
	}
	if _, err := system.Build(dir, []string{"../../outside"}); err == nil || !strings.Contains(err.Error(), "not inside the kernel source") {
		t.Errorf("a folder outside the source: %v", err)
	}
	if _, err := system.Build(dir, []string{"drivers/phy"}); err == nil || !strings.Contains(err.Error(), "no Makefile") {
		t.Errorf("a folder without a Makefile: %v", err)
	}
}
