package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// An update is offered only where it does no harm to the work under way. Each of these was a way
// for it to break things, be it a test boot entry left with a kernel that no longer matches, a
// kernel of the owner's own, or a package they built themselves that the update would replace.
func TestAnUpdateIsNotOfferedWhereItWouldDoHarm(t *testing.T) {
	stock := updateFacts{Kernel: "7.3.0-72-omarchy", Pkgbase: "linux-omarchy", Installed: true,
		Packagers: map[string]string{"linux-omarchy": "Omarchy <pkgs@omarchy.org>", "linux-omarchy-dtbs": "Omarchy <pkgs@omarchy.org>", "linux-firmware-qcom": "Omarchy <pkgs@omarchy.org>"}}
	if why := updateBlocked(stock); len(why) != 0 {
		t.Errorf("a stock computer was refused: %v", why)
	}
	with := func(change func(*updateFacts)) updateFacts {
		facts := stock
		facts.Packagers = map[string]string{}
		for name, who := range stock.Packagers {
			facts.Packagers[name] = who
		}
		change(&facts)
		return facts
	}
	for name, c := range map[string]struct {
		facts updateFacts
		says  string
	}{
		"a test entry stands":          {with(func(f *updateFacts) { f.TestEntry = true }), "hw-test tree remove"},
		"the test image is running":    {with(func(f *updateFacts) { f.Running = true }), "test boot entry"},
		"a kernel of the owner's own":  {with(func(f *updateFacts) { f.Kernel, f.Pkgbase = "7.3.0-13-omarchy-geni3", "linux-omarchy-geni3" }), "linux-omarchy-geni3"},
		"updated without a restart":    {with(func(f *updateFacts) { f.Installed, f.Pkgbase = false, "" }), "Restart first"},
		"a device tree package of own": {with(func(f *updateFacts) { f.Packagers["linux-omarchy-dtbs"] = "Unknown Packager" }), "linux-omarchy-dtbs was built"},
	} {
		why := updateBlocked(c.facts)
		if len(why) != 1 || !strings.Contains(why[0], c.says) {
			t.Errorf("%s: %v", name, why)
		}
	}
	// A package that isn't installed has no packager and is no reason
	if why := updateBlocked(with(func(f *updateFacts) { delete(f.Packagers, "linux-firmware-qcom") })); len(why) != 0 {
		t.Errorf("a missing package was a reason: %v", why)
	}
}

// The end of the update's log comes back without colours, blank lines or what names the owner.
func TestTheUpdateLogComesBackClean(t *testing.T) {
	text := "\x1b[32m\nUpdate system packages\x1b[0m\r\n:: Synchronizing package databases...\n\n error: failed to commit transaction (conflicting files)\nlinux-omarchy: /home/somebody/x exists in filesystem\n\x1b[0;31mSomething went wrong during the update!\x1b[0m\n"
	got := logTail(text, 3)
	if len(got) != 3 || got[0] != " error: failed to commit transaction (conflicting files)" || !strings.Contains(got[1], "/home/user/x") || got[2] != "Something went wrong during the update!" {
		t.Errorf("%q", got)
	}
}

// The whole step against a stand-in update. It opens for the owner, its outcome is read the moment
// it ends, and a failed update comes back as one, with the log's last lines.
func TestTheUpdateIsOpenedForTheOwnerAndItsOutcomeComesBack(t *testing.T) {
	dir := t.TempDir()
	// Run what would open in the owner's window right here, with no window
	t.Setenv("HW_TEST_TERMINAL", "env")
	beforeCommand, beforeLog, beforeFacts, beforeWait := updateCommand, updateLog, updateFactsNow, updateWait
	defer func() {
		updateCommand, updateLog, updateFactsNow, updateWait = beforeCommand, beforeLog, beforeFacts, beforeWait
	}()
	updateLog, updateWait = filepath.Join(dir, "update.log"), 20*time.Second
	updateFactsNow = func() updateFacts {
		return updateFacts{Kernel: "7.3.0-72-omarchy", Pkgbase: "linux-omarchy", Installed: true}
	}
	run := func(script string) (int, updateOutcome, string) {
		updateCommand = filepath.Join(dir, "update")
		if err := os.WriteFile(updateCommand, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		read, write, _ := os.Pipe()
		stdout := os.Stdout
		os.Stdout = write
		code := askUpdate()
		write.Close()
		os.Stdout = stdout
		printed, _ := io.ReadAll(read)
		var outcome updateOutcome
		_ = json.Unmarshal(printed, &outcome)
		return code, outcome, string(printed)
	}

	code, outcome, printed := run("echo 'Update system packages' > " + updateLog + "; echo ' there is nothing to do' >> " + updateLog + "; exit 0")
	if code != 0 || !outcome.Asked || !outcome.Finished || outcome.Exit == nil || *outcome.Exit != 0 || outcome.Restart || len(outcome.Log) != 2 {
		t.Errorf("an update that ran through: code %d, %s", code, printed)
	}
	code, outcome, printed = run("echo 'error: failed to commit transaction' > " + updateLog + "; exit 1")
	if code != 1 || !outcome.Asked || outcome.Finished || outcome.Exit == nil || *outcome.Exit != 1 || len(outcome.Log) != 1 || !strings.Contains(outcome.Log[0], "failed to commit") {
		t.Errorf("an update that failed: code %d, %s", code, printed)
	}

	// Where it would do harm, nothing is opened, and the reason comes back as data
	opened := filepath.Join(dir, "opened")
	updateFactsNow = func() updateFacts {
		return updateFacts{TestEntry: true, Kernel: "7.3.0-72-omarchy", Pkgbase: "linux-omarchy", Installed: true}
	}
	code, outcome, printed = run("touch " + opened)
	if _, err := os.Stat(opened); err == nil {
		t.Error("the update was started although a test entry stands")
	}
	if code != 1 || outcome.Asked || len(outcome.Why) != 1 || !strings.Contains(outcome.Why[0], "hw-test tree remove") {
		t.Errorf("refused: code %d, %s", code, printed)
	}
}
