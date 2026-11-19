// Package machine reads the computer the app runs on: files under /sys, /proc and /etc, and the
// output of a few read-only commands. Tests point it at a copy of another machine's files
// (test/probe/fixtures in the repository, made with scripts/probe-fixture.sh), where the
// commands are answered from the same copy.
package machine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// System is one machine: the real one when Root is empty, or a fixture folder.
type System struct {
	// Root is put in front of every path read.
	Root string
	// Command runs a program and returns what it printed. Nil runs the real program.
	Command func(name string, args ...string) (string, error)
}

// Real is the machine the app runs on.
func Real() System { return System{} }

func (s System) path(p string) string { return filepath.Join(s.Root, p) }

// Read returns a small file's first line, or "" when it can't be read.
func (s System) Read(p string) string {
	line, _, _ := strings.Cut(s.ReadAll(p), "\n")
	return strings.TrimSpace(line)
}

// readLimit is how long one file may take. A file under /sys is answered by a driver, and a
// driver whose hardware or firmware is broken can leave a read hanging for good.
const readLimit = 2 * time.Second

// ReadAll returns a small file's text with NUL bytes turned into newlines, as the device
// tree's string lists need, or "" when it can't be read or the read hangs.
func (s System) ReadAll(p string) string {
	return strings.TrimRight(strings.ReplaceAll(string(s.Bytes(p)), "\x00", "\n"), "\n")
}

// Resolve follows the links in a path and returns where it really is, or "" when it isn't there.
func (s System) Resolve(p string) string {
	real, err := filepath.EvalSymlinks(s.path(p))
	if err != nil {
		return ""
	}
	root := s.Root
	if root != "" {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
	}
	return strings.TrimPrefix(real, filepath.Clean(root))
}

// Bytes returns a small file as it is, as a device tree's numbers need, or nothing when it
// can't be read or the read hangs.
func (s System) Bytes(p string) []byte {
	type answer struct {
		data []byte
		err  error
	}
	done := make(chan answer, 1)
	go func() {
		data, err := os.ReadFile(s.path(p))
		done <- answer{data, err}
	}()
	var data []byte
	select {
	case got := <-done:
		if got.err != nil {
			return nil
		}
		data = got.data
	case <-time.After(readLimit):
		// The read stays stuck in its own goroutine; the check goes on without the value
		return nil
	}
	if len(data) > 64*1024 {
		data = data[:64*1024]
	}
	return data
}

// Int returns a file's number, and whether it held one.
func (s System) Int(p string) (int64, bool) {
	var n int64
	text := s.Read(p)
	if text == "" {
		return 0, false
	}
	for i, c := range text {
		if c == '-' && i == 0 {
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	if strings.HasPrefix(text, "-") {
		n = -n
	}
	return n, true
}

// Glob lists the paths a pattern matches, without the root, sorted.
func (s System) Glob(pattern string) []string {
	matches, _ := filepath.Glob(s.path(pattern))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if filepath.Base(m) == ".keep" {
			continue
		}
		out = append(out, strings.TrimPrefix(m, filepath.Clean(s.Root)))
	}
	sort.Strings(out)
	return out
}

// Exists says whether a path is there, as a file, a folder or a link.
func (s System) Exists(p string) bool {
	_, err := os.Lstat(s.path(p))
	return err == nil
}

// IsDir says whether a path is a folder.
func (s System) IsDir(p string) bool {
	info, err := os.Stat(s.path(p))
	return err == nil && info.IsDir()
}

// LinkName returns the last part of what a link points to, like the driver's name behind a
// device's driver link, or "" when it isn't a link.
func (s System) LinkName(p string) string {
	target, err := filepath.EvalSymlinks(s.path(p))
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

// Run gives a command's output, or "" when it fails, isn't installed or takes over ten seconds.
func (s System) Run(name string, args ...string) string {
	if s.Command != nil {
		out, _ := s.Command(name, args...)
		return out
	}
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	// Not held up either by something the program started that keeps its output open
	command.WaitDelay = time.Second
	out, err := command.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// Has says whether a program is installed.
func (s System) Has(name string) bool {
	if s.Command != nil {
		_, err := s.Command("which", name)
		return err == nil
	}
	_, err := exec.LookPath(name)
	return err == nil
}

// Fixture is a machine copied by scripts/probe-fixture.sh: its files under root, and its
// commands answered from root/cmd and root/udev.
func Fixture(root string) System {
	read := func(parts ...string) (string, error) {
		data, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
		return string(data), err
	}
	return System{
		Root: root,
		Command: func(name string, args ...string) (string, error) {
			switch name {
			case "which":
				switch args[0] {
				case "cam", "fwupdtool":
					return "", os.ErrNotExist
				}
				return args[0], nil
			case "uname":
				if len(args) > 0 && args[0] == "-m" {
					return read("cmd", "uname-m")
				}
				return read("cmd", "uname-r")
			case "journalctl":
				return read("cmd", "klog")
			case "udevadm":
				return read("udev", filepath.Base(args[len(args)-1]))
			case "bluetoothctl":
				return read("cmd", "bluetoothctl-show")
			case "systemd-analyze":
				return read("cmd", "chids")
			case "pacman":
				text, _ := read("cmd", "packages")
				want := args[len(args)-1]
				for _, line := range strings.Split(text, "\n") {
					fields := strings.Fields(line)
					if len(fields) == 2 && (fields[0] == want || (want == "omarchy" && fields[0] == "omarchy-dev")) {
						return line, nil
					}
				}
				return "", os.ErrNotExist
			}
			return "", os.ErrNotExist
		},
	}
}
