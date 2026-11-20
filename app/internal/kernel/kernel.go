// Package kernel is the way to try a change in one kernel driver without building or
// installing a kernel: Omarchy's kernel source as the running kernel was built from it, one
// driver folder built against the running kernel's headers, and the result loaded for this
// session only. Nothing it does outlasts a restart, and nothing is written where the system
// starts from.
package kernel

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Where Omarchy's kernel recipe is, and where in that repository.
const (
	Repository = "https://github.com/omacom/omarchy-pkgs"
	Recipe     = "pkgbuilds/linux-omarchy-aarch64"
)

// Marker lists the test modules loaded since the computer started. It is under /run, which
// is emptied at every start, as the modules themselves are gone then.
const Marker = "/run/hw-test/test-modules"

// System is the computer as this package needs it.
type System struct {
	// Kernel is the running kernel, as uname -r names it, and Headers its build folder.
	Kernel, Headers string
	// Release is the kernel package's version, like 7.3-72.
	Release string
	// Repository is where the recipe comes from, and Marker where loaded test modules are
	// listed. Tests point both elsewhere.
	Repository, Marker string
}

// Real is this computer.
func Real() System {
	kernel := strings.TrimSpace(output("", "uname", "-r"))
	release := ""
	if fields := strings.Fields(output("", "pacman", "-Q", "linux-omarchy")); len(fields) == 2 {
		release = fields[1]
	}
	return System{Kernel: kernel, Headers: "/usr/lib/modules/" + kernel + "/build", Release: release, Repository: Repository, Marker: Marker}
}

func output(dir, name string, args ...string) string {
	command := exec.Command(name, args...)
	command.Dir = dir
	out, _ := command.Output()
	return string(out)
}

func run(dir, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, tail(string(out), 12))
	}
	return string(out), nil
}

// tail keeps the last lines of a long output, where a build says what went wrong.
func tail(text string, lines int) string {
	all := strings.Split(strings.TrimSpace(text), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

// Ready says what is missing to build a driver here.
func (s System) Ready() error {
	if _, err := os.Stat(filepath.Join(s.Headers, "Makefile")); err != nil {
		return fmt.Errorf("the kernel's headers aren't installed, and a driver is built against them: sudo pacman -S linux-omarchy-headers")
	}
	for _, tool := range []string{"git", "make", "makepkg", "modinfo"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s is needed to build a driver, and it isn't installed", tool)
		}
	}
	return nil
}

func root(dir string) string { return filepath.Join(dir, "kernel") }

// RecipeIn is the kernel recipe's folder under a work folder, and Source the kernel source
// that hw-test kernel start unpacked there, or "" before that.
func RecipeIn(dir string) string { return filepath.Join(root(dir), "recipe", Recipe) }

func Source(dir string) string {
	found, _ := filepath.Glob(filepath.Join(RecipeIn(dir), "src", "linux-*"))
	for _, path := range found {
		if info, err := os.Stat(filepath.Join(path, "Makefile")); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// Started is what hw-test kernel start made.
type Started struct {
	// Source is the kernel source with Omarchy's patches applied, under git.
	Source string
	// Exact is set when the recipe is the one the running kernel was built from. Without it
	// the source is Omarchy's newest, and a driver built from it may not fit this kernel.
	Exact bool
	// Patches counts the recipe's patches.
	Patches int
}

var (
	pkgver = regexp.MustCompile(`(?m)^pkgver=(\S+)$`)
	pkgrel = regexp.MustCompile(`(?m)^pkgrel=(\S+)$`)
)

// Start fetches Omarchy's kernel recipe as it was for the running kernel, lets makepkg
// fetch and patch the source without building it, and puts the result under git, so that
// every later change shows as a diff.
func (s System) Start(dir string) (Started, error) {
	var started Started
	if err := s.Ready(); err != nil {
		return started, err
	}
	if _, err := os.Stat(root(dir)); err == nil {
		return started, fmt.Errorf("%s is there already. Go on in it, or move it away to start again", root(dir))
	}
	clone := filepath.Join(root(dir), "recipe")
	if err := os.MkdirAll(root(dir), 0o755); err != nil {
		return started, err
	}
	if _, err := run(root(dir), "git", "clone", "-q", "--filter=blob:none", "--sparse", s.Repository, clone); err != nil {
		return started, err
	}
	if _, err := run(clone, "git", "sparse-checkout", "set", Recipe); err != nil {
		return started, err
	}
	// The recipe as it was when the running kernel was built: the newest commit whose
	// PKGBUILD has this version and release
	file := Recipe + "/PKGBUILD"
	version, release, _ := strings.Cut(s.Release, "-")
	commits, _ := run(clone, "git", "log", "--format=%H", "-n", "400", "--", file)
	for _, commit := range strings.Fields(commits) {
		text := output(clone, "git", "show", commit+":"+file)
		v, r := pkgver.FindStringSubmatch(text), pkgrel.FindStringSubmatch(text)
		if v != nil && r != nil && v[1] == version && r[1] == release {
			if _, err := run(clone, "git", "checkout", "-q", commit); err != nil {
				return started, err
			}
			started.Exact = true
			break
		}
	}
	recipe := RecipeIn(dir)
	listed, _ := filepath.Glob(filepath.Join(recipe, "*.patch"))
	started.Patches = len(listed)
	// Fetches the source and applies the patches, and builds nothing. The signatures are
	// the packagers' own business: the checksums in the recipe are still checked.
	if _, err := run(recipe, "makepkg", "-o", "--skippgpcheck", "--nodeps"); err != nil {
		return started, fmt.Errorf("the kernel's source couldn't be prepared: %v", err)
	}
	started.Source = Source(dir)
	if started.Source == "" {
		return started, fmt.Errorf("makepkg left no kernel source under %s", filepath.Join(recipe, "src"))
	}
	for _, git := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=hw-test", "-c", "user.email=hw-test@localhost", "commit", "-q", "-m", "Omarchy's kernel source, as linux-omarchy " + s.Release + " is built from it"},
	} {
		if _, err := run(started.Source, "git", git...); err != nil {
			return started, err
		}
	}
	return started, nil
}

// Built is what one build of driver folders gave.
type Built struct {
	// Modules are the modules that came out, by path.
	Modules []string
	// Changed names the source files that differ from Omarchy's, and Diff is the file the
	// change is written to.
	Changed []string
	Diff    string
}

// Build builds the modules of the named folders of the kernel source against the running
// kernel's headers. A file's name stands for its folder.
func (s System) Build(dir string, paths []string) (Built, error) {
	var built Built
	source := Source(dir)
	if source == "" {
		return built, fmt.Errorf("there is no kernel source in %s yet. Start with: hw-test kernel start", root(dir))
	}
	if len(paths) == 0 {
		return built, fmt.Errorf("name the driver's folder, as in: hw-test kernel build drivers/phy/qualcomm")
	}
	seen := map[string]bool{}
	for _, path := range paths {
		folder := filepath.Join(source, strings.TrimPrefix(filepath.Clean(path), source+"/"))
		if info, err := os.Stat(folder); err == nil && !info.IsDir() {
			folder = filepath.Dir(folder)
		}
		if !strings.HasPrefix(folder, source+string(filepath.Separator)) {
			return built, fmt.Errorf("%s is not inside the kernel source %s", path, source)
		}
		if _, err := os.Stat(filepath.Join(folder, "Makefile")); err != nil {
			if _, err := os.Stat(filepath.Join(folder, "Kbuild")); err != nil {
				return built, fmt.Errorf("%s has no Makefile, so there is no driver to build there", folder)
			}
		}
		if seen[folder] {
			continue
		}
		seen[folder] = true
		if _, err := run(source, "make", "-C", s.Headers, "M="+folder, "modules", "-j"+strconv.Itoa(cores())); err != nil {
			return built, fmt.Errorf("the build failed: %v", err)
		}
		modules, _ := filepath.Glob(filepath.Join(folder, "*.ko"))
		for _, module := range modules {
			// Built for another kernel it would not load, or load and misbehave
			if magic := strings.Fields(output("", "modinfo", "-F", "vermagic", module)); len(magic) == 0 || magic[0] != s.Kernel {
				return built, fmt.Errorf("%s came out for kernel %s, and %s is running", module, strings.Join(magic[:min(1, len(magic))], ""), s.Kernel)
			}
			built.Modules = append(built.Modules, module)
		}
	}
	sort.Strings(built.Modules)
	if len(built.Modules) == 0 {
		return built, fmt.Errorf("no module came out. A driver that Omarchy's kernel has built in, or switched off, can't be built this way and needs a whole kernel (see the guide's \"Trying a kernel change\")")
	}
	// New files count as part of the change. What a build leaves behind is ignored by the
	// kernel's own .gitignore
	if _, err := run(source, "git", "add", "-A", "-N", "."); err != nil {
		return built, err
	}
	names, _ := run(source, "git", "diff", "--name-only", "HEAD")
	built.Changed = strings.Fields(names)
	// With a/ and b/ in the paths, whatever the owner's git is set to
	diff, _ := run(source, "git", "-c", "diff.mnemonicPrefix=false", "-c", "diff.noprefix=false", "diff", "HEAD")
	built.Diff = filepath.Join(dir, "kernel.diff")
	return built, os.WriteFile(built.Diff, []byte(diff), 0o644)
}

func cores() int {
	if n, err := strconv.Atoi(strings.TrimSpace(output("", "nproc"))); err == nil && n > 0 {
		return n
	}
	return 4
}

func quote(text string) string { return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'" }

// LoadScript is what the owner runs, with their password, to put built modules in place of
// the kernel's own until the next restart. It is written out for them to read first. A
// module that can't be taken out while it is in use stops the script before anything
// changes, and a new module that won't load gets the kernel's own put back.
func (s System) LoadScript(modules []string) string {
	script := `#!/bin/bash
# Written by hw-test kernel load. Loads the modules built by hw-test kernel build in place of
# the kernel's own. Nothing is written to disk. After a restart the kernel's own are back.
set -u
marker=` + quote(s.Marker) + `
loaded=${HW_TEST_SYS_MODULE:-/sys/module}
mkdir -p "$(dirname "$marker")"
swap() {
  local file=$1 name
  name=$(modinfo -F name "$file" 2>/dev/null)
  [ -n "$name" ] || name=$(basename "$file" .ko)
  name=${name//-/_}
  local was=no
  if [ -d "$loaded/$name" ]; then
    was=yes
    if ! rmmod "$name"; then
      echo "$name is in use and can't be taken out while it runs. Nothing was changed for it. A driver like that needs a whole test kernel." >&2
      return 1
    fi
  fi
  # What the module builds on, from the kernel's own
  for needs in $(modinfo -F depends "$file" | tr ',' ' '); do
    modprobe "$needs" 2>/dev/null || true
  done
  if ! insmod "$file"; then
    echo "The built $name didn't load. The kernel's log says why (journalctl -k -b | tail). The kernel's own is put back where it was loaded." >&2
    if [ $was = yes ]; then
      modprobe "$name" || echo "The kernel's own $name didn't come back either. A restart brings it back." >&2
    fi
    return 1
  fi
  # With whether the kernel's own was loaded before, so that restore leaves things as they were
  echo "$name $was $file" >> "$marker"
  echo "Loaded the built $name."
}
status=0
`
	for _, module := range modules {
		script += "swap " + quote(module) + " || status=1\n"
	}
	return script + `exit $status
`
}

// RestoreScript puts the kernel's own modules back in place of every test module loaded
// since the computer started, newest first.
func (s System) RestoreScript() string {
	return `#!/bin/bash
# Written by hw-test kernel restore. Takes the test modules out and loads the kernel's own.
set -u
marker=` + quote(s.Marker) + `
loaded=${HW_TEST_SYS_MODULE:-/sys/module}
[ -s "$marker" ] || { echo "No test module is loaded."; exit 0; }
status=0
for name in $(tac "$marker" | cut -d' ' -f1 | awk '!seen[$0]++'); do
  if [ -d "$loaded/$name" ] && ! rmmod "$name"; then
    echo "$name is in use and stays until the next restart." >&2
    status=1
    continue
  fi
  # Loaded again only where the kernel's own was loaded before the first test module took its place
  if [ "$(grep -m1 "^$name " "$marker" | cut -d' ' -f2)" = yes ]; then
    modprobe "$name" && echo "The kernel's own $name is back." || { echo "The kernel's own $name didn't load. A restart brings it back." >&2; status=1; }
  else
    echo "The test $name is out. The kernel's own wasn't loaded before, so it stays out."
  fi
done
[ $status -ne 0 ] || rm -f "$marker"
exit $status
`
}

// Loaded names the test modules loaded since the computer started.
func (s System) Loaded() []string {
	file, err := os.Open(s.Marker)
	if err != nil {
		return nil
	}
	defer file.Close()
	var names []string
	seen := map[string]bool{}
	for lines := bufio.NewScanner(file); lines.Scan(); {
		if fields := strings.Fields(lines.Text()); len(fields) > 0 && !seen[fields[0]] {
			seen[fields[0]] = true
			names = append(names, fields[0])
		}
	}
	return names
}

// Patched is what hw-test kernel patch wrote.
type Patched struct {
	// File is the patch, Recipe the package folder it goes into, and Changed the files it touches.
	File, Recipe string
	Changed      []string
}

var patchNumber = regexp.MustCompile(`^(\d{4})-.*\.patch$`)

// Patch writes the whole change against Omarchy's kernel source as one patch in the
// kernel's format, numbered after the recipe's last. subject is its first line, as the
// kernel writes them ("phy: qcom-qmp-ufs: Add ..."), and body says why. The author is the
// owner, as their git names them.
func (s System) Patch(dir, subject, body string) (Patched, error) {
	patched := Patched{Recipe: RecipeIn(dir)}
	source := Source(dir)
	if source == "" {
		return patched, fmt.Errorf("there is no kernel source in %s yet. Start with: hw-test kernel start", root(dir))
	}
	subject = strings.TrimSpace(subject)
	if subject == "" || strings.Contains(subject, "\n") || !strings.Contains(subject, ": ") {
		return patched, fmt.Errorf("the patch needs a first line that names what it changes, as the kernel writes them, like: hw-test kernel patch \"phy: qcom-qmp-usb: Add the port's missing supply\" --body-file why.md")
	}
	var who [2]string
	for at, key := range []string{"user.name", "user.email"} {
		if who[at] = strings.TrimSpace(output(dir, "git", "config", "--global", "--get", key)); who[at] == "" {
			return patched, fmt.Errorf("git doesn't know the owner's %s, and a patch names who wrote it. The owner sets it once: git config --global %s \"...\"", strings.TrimPrefix(key, "user."), key)
		}
	}
	from, err := run(source, "git", "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return patched, err
	}
	// The whole change as one commit over Omarchy's source, however often this is run
	for _, git := range [][]string{{"add", "-A"}, {"reset", "-q", "--soft", strings.TrimSpace(from)}} {
		if _, err := run(source, "git", git...); err != nil {
			return patched, err
		}
	}
	names, _ := run(source, "git", "diff", "--cached", "--name-only")
	patched.Changed = strings.Fields(names)
	if len(patched.Changed) == 0 {
		return patched, fmt.Errorf("nothing differs from Omarchy's kernel source, so there is nothing to make a patch of")
	}
	commit := []string{"-c", "user.name=" + who[0], "-c", "user.email=" + who[1], "commit", "-q", "-m", subject}
	if body = strings.TrimSpace(body); body != "" {
		commit = append(commit, "-m", body)
	}
	if _, err := run(source, "git", commit...); err != nil {
		return patched, err
	}
	out, err := os.MkdirTemp("", "hw-test-patch-")
	if err != nil {
		return patched, err
	}
	defer os.RemoveAll(out)
	if _, err := run(source, "git", "-c", "diff.mnemonicPrefix=false", "-c", "diff.noprefix=false", "format-patch", "-q", "-1", "--zero-commit", "-o", out); err != nil {
		return patched, err
	}
	made, _ := filepath.Glob(filepath.Join(out, "0001-*.patch"))
	if len(made) != 1 {
		return patched, fmt.Errorf("git made no patch of the change")
	}
	number := 1
	listed, _ := filepath.Glob(filepath.Join(patched.Recipe, "*.patch"))
	for _, file := range listed {
		if match := patchNumber.FindStringSubmatch(filepath.Base(file)); match != nil {
			if n, _ := strconv.Atoi(match[1]); n >= number {
				number = n + 1
			}
		}
	}
	text, err := os.ReadFile(made[0])
	if err != nil {
		return patched, err
	}
	// An earlier writing of the same change, under whatever name, is replaced
	old, _ := filepath.Glob(filepath.Join(root(dir), "[0-9][0-9][0-9][0-9]-*.patch"))
	for _, file := range old {
		os.Remove(file)
	}
	// Beside the source and not in the work folder, where hw-test tree patch keeps its own
	patched.File = filepath.Join(root(dir), fmt.Sprintf("%04d-%s", number, strings.TrimPrefix(filepath.Base(made[0]), "0001-")))
	return patched, os.WriteFile(patched.File, text, 0o644)
}
