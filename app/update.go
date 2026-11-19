package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/guided"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
	"github.com/birkskyum/arewearmyet/app/internal/tree"
)

// hw-test ask update: the system update as a step for the owner. An agent that finds the
// computer behind its channel used to ask the owner to run the update and tell it how it went,
// and on the Yoga three of the owner's messages in one session were that relay. Here the
// update opens in a window for the owner, who confirms it as always, and the agent gets the
// outcome: whether it finished, what moved, whether a restart is due, and the log's last lines.
//
// An update is never the agent's to decide, and there are times it does harm, so those are
// checked first and refused with the reason.

// The command that updates the system, the log it writes, and how long the owner gets.
var (
	updateCommand = "omarchy-update"
	updateLog     = "/tmp/omarchy-update.log"
	updateWait    = 90 * time.Minute
)

// The packages a locally built copy of which an update would replace.
var carried = []string{"linux-omarchy", "linux-omarchy-dtbs", "linux-firmware-qcom"}

// updateFacts is what is looked at before an update is offered.
type updateFacts struct {
	// TestEntry is set while a test boot entry stands, and Running while its image is what started
	TestEntry, Running bool
	// Kernel is the running kernel and Pkgbase the package it comes from. Installed says whether
	// that kernel's modules are still there
	Kernel, Pkgbase string
	Installed       bool
	// Packagers is who built each carried package that is installed
	Packagers map[string]string
}

// updateBlocked gives the reasons an update should not be started from here, in plain words
// for the owner. No reason means it can be offered.
func updateBlocked(f updateFacts) []string {
	var why []string
	if f.TestEntry || f.Running {
		why = append(why, "A test boot entry ("+tree.Entry+") is installed. It was built from the kernel that is installed now, so an update that brings a new kernel leaves it with one that no longer matches. Finish the test and take the entry away with hw-test tree remove first.")
	}
	switch {
	case f.Kernel != "" && !f.Installed:
		why = append(why, "The kernel that is running ("+f.Kernel+") is no longer installed, so an update was done without a restart. Restart first.")
	case f.Pkgbase != "" && f.Pkgbase != "linux-omarchy":
		why = append(why, "This computer is running the kernel package "+f.Pkgbase+", not Omarchy's own linux-omarchy. Whether linux-omarchy is behind its channel says nothing about it, so updating is the owner's call here.")
	}
	for _, name := range carried {
		if who := f.Packagers[name]; who != "" && !strings.HasPrefix(who, "Omarchy") {
			why = append(why, name+" was built on this computer or by someone else ("+who+"), not by Omarchy. An update replaces it once Omarchy has a newer version, and what it carried is lost without a word.")
		}
	}
	return why
}

var packagerLine = regexp.MustCompile(`(?m)^Packager\s*:\s*(.+)$`)

// gatherUpdateFacts reads the facts from this computer.
func gatherUpdateFacts() updateFacts {
	facts := updateFacts{Running: tree.Running("/proc/device-tree"), Packagers: map[string]string{}}
	_, facts.TestEntry = report.InstalledTestEntry()
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		facts.Kernel = strings.TrimSpace(string(out))
		modules := filepath.Join("/usr/lib/modules", facts.Kernel)
		if _, err := os.Stat(modules); err == nil {
			facts.Installed = true
			if base, err := os.ReadFile(filepath.Join(modules, "pkgbase")); err == nil {
				facts.Pkgbase = strings.TrimSpace(string(base))
			}
		}
	}
	for _, name := range carried {
		command := exec.Command("pacman", "-Qi", name)
		command.Env = append(os.Environ(), "LC_ALL=C")
		if out, err := command.Output(); err == nil {
			if found := packagerLine.FindSubmatch(out); found != nil {
				facts.Packagers[name] = strings.TrimSpace(string(found[1]))
			}
		}
	}
	return facts
}

// updateFactsNow is how the facts are read, so a test can give its own.
var updateFactsNow = gatherUpdateFacts

// updateOutcome is what the agent gets back.
type updateOutcome struct {
	// Asked is false when the update was not offered, and Why says why not
	Asked bool     `json:"asked"`
	Why   []string `json:"why,omitempty"`
	// Finished is true when the update ran through. Exit is what it ended with
	Finished bool `json:"finished"`
	Exit     *int `json:"exit,omitempty"`
	// Moved lists the packages whose version changed, and Restart says a restart is due for them
	Moved   []string `json:"moved,omitempty"`
	Restart bool     `json:"restart"`
	// Log is the end of the update's own log, cleaned like every log
	Log []string `json:"log,omitempty"`
}

func versionsNow() map[string]string {
	versions := map[string]string{}
	for _, name := range []string{"linux-omarchy", "linux-omarchy-dtbs", "linux-firmware", "linux-firmware-qcom", "mesa", "omarchy", "alsa-ucm-conf", "pipewire", "libcamera"} {
		if out, err := exec.Command("pacman", "-Q", name).Output(); err == nil {
			if fields := strings.Fields(string(out)); len(fields) == 2 {
				versions[name] = fields[1]
			}
		}
	}
	return versions
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[()][A-Za-z0-9]|\r`)

// logTail is the last lines of the update's log that say something, without colours and
// cleaned of what names the owner.
func logTail(text string, lines int) []string {
	host, account, names := owner()
	var kept []string
	for _, line := range strings.Split(report.ScrubLog(ansi.ReplaceAllString(text, ""), host, account, names...), "\n") {
		if line = strings.TrimRight(line, " \t"); strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	if len(kept) > lines {
		kept = kept[len(kept)-lines:]
	}
	return kept
}

func askUpdate() int {
	say := func(outcome updateOutcome) {
		data, _ := json.MarshalIndent(outcome, "", "  ")
		fmt.Println(string(data))
	}
	if !tools.Have(updateCommand) {
		fmt.Fprintln(os.Stderr, "hw-test: this computer has no "+updateCommand+", so there is no Omarchy update to open.")
		return 1
	}
	if why := updateBlocked(updateFactsNow()); len(why) > 0 {
		fmt.Fprintln(os.Stderr, "hw-test: the update was not opened. Tell the owner why, and leave the decision with them:")
		for _, reason := range why {
			fmt.Fprintln(os.Stderr, "  - "+reason)
		}
		say(updateOutcome{Asked: false, Why: why})
		return 1
	}
	before := versionsNow()
	file, err := os.CreateTemp("", "hw-test-updated-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		return 1
	}
	answered := file.Name()
	file.Close()
	os.Remove(answered)
	defer os.Remove(answered)
	quote := func(text string) string { return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'" }
	// The update asks for its own confirmation and the owner's password in the window. How it
	// ended is written down the moment it ends, so the agent isn't kept waiting on the window
	line := fmt.Sprintf("%s; code=$?; echo $code > %s.part && mv %s.part %s; echo; read -n1 -rsp 'Press a key to close this window.'", updateCommand, quote(answered), quote(answered), quote(answered))
	if guided.Interactive() {
		fmt.Fprintln(os.Stderr, "hw-test ask update is for an agent without a terminal. In a terminal, run the update yourself: "+updateCommand)
		return 2
	}
	if err := tools.Terminal("bash", "-c", line); err != nil {
		fmt.Fprintln(os.Stderr, "hw-test: the update couldn't be opened for the owner:", err)
		return 1
	}
	fmt.Fprintln(os.Stderr, "Omarchy's update is open for the owner in a window of its own. They confirm it and type their password there. Waiting until it ends.")
	code, ended := 0, false
	for waited := time.Duration(0); waited < updateWait; waited += 500 * time.Millisecond {
		if data, err := os.ReadFile(answered); err == nil && strings.TrimSpace(string(data)) != "" {
			_, _ = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &code)
			ended = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	outcome := updateOutcome{Asked: true}
	if !ended {
		fmt.Fprintln(os.Stderr, "hw-test: the update didn't end in time, or the owner didn't get to it.")
		say(outcome)
		return 1
	}
	outcome.Exit, outcome.Finished = &code, code == 0
	after := versionsNow()
	for _, name := range []string{"linux-omarchy", "linux-omarchy-dtbs", "linux-firmware", "linux-firmware-qcom", "mesa", "omarchy", "alsa-ucm-conf", "pipewire", "libcamera"} {
		if before[name] != after[name] && after[name] != "" {
			outcome.Moved = append(outcome.Moved, name+" "+before[name]+" to "+after[name])
			if strings.HasPrefix(name, "linux-") {
				outcome.Restart = true
			}
		}
	}
	if text, err := os.ReadFile(updateLog); err == nil {
		outcome.Log = logTail(string(text), 25)
	}
	say(outcome)
	if !outcome.Finished {
		return 1
	}
	return 0
}
