package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/guided"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// trustRule is the one file hw-test trust writes outside the owner's home folder.
const trustRule = "/etc/sudoers.d/hw-test-trust"

// The longest the owner's word holds, and how long when they name no time.
const trustMost, trustUsual = 7 * 24, 8

var trustUser = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// trustScript writes the rule that lets one user run sudo without a password until a time. The
// time is in the rule itself, so the rule stops working then with nothing left to run, also
// across restarts. It is checked before it is put in place, since a broken file there would
// lock sudo for everyone.
func trustScript(name string, until time.Time) string {
	return `set -eu
rule=` + trustRule + `
new=$(mktemp)
trap 'rm -f "$new"' EXIT
cat > "$new" <<'RULE'
# Written by hw-test trust. Until the time in the line below, in UTC, sudo asks this user for no
# password. After it the line does nothing. hw-test trust --end removes this file.
` + name + ` ALL=(ALL:ALL) NOTAFTER=` + until.UTC().Format("20060102150405") + `Z NOPASSWD: ALL
RULE
visudo -cf "$new"
install -m 0440 -o root -g root "$new" "$rule"
echo "sudo asks ` + name + ` for no password until ` + until.Local().Format("15:04 on 2 January") + `."
`
}

// trustHours reads how long the owner said, in hours ("12") or in days ("3d"), a week at most.
func trustHours(word string) (int, bool) {
	each := 1
	if days, found := strings.CutSuffix(word, "d"); found {
		word, each = days, 24
	}
	given, err := strconv.Atoi(word)
	if err != nil || given < 1 || given*each > trustMost {
		return 0, false
	}
	return given * each, true
}

// sudoFree says whether sudo runs a command now without asking for a password.
func sudoFree() bool { return exec.Command("sudo", "-n", "true").Run() == nil }

// trust is the owner's word that their agent works on its own for some hours. It needs their
// password once. An agent that runs it gets a window opened for the owner, who decides there.
func trust(words []string, show, end, asJSON bool) int {
	fail := func(text string) int {
		fmt.Fprintln(os.Stderr, "hw-test: "+text)
		return 2
	}
	until, inForce := report.Trusted(time.Now())
	said := func() string {
		return "Your agent works on its own until " + until.Local().Format("15:04 on 2 January") + ". It restarts and changes system settings without asking first, and sudo asks for no password. Your files, the disk and firmware stay off limits. hw-test trust --end takes it back."
	}
	switch {
	case show:
		if asJSON {
			state := map[string]any{"inForce": inForce, "sudo": sudoFree()}
			if inForce {
				state["until"] = until.UTC().Format(time.RFC3339)
			}
			data, _ := json.MarshalIndent(state, "", "  ")
			fmt.Println(string(data))
			return 0
		}
		if !inForce {
			fmt.Fprintln(os.Stderr, "Nothing is in force. The agent asks before a restart or a system change, and sudo asks for your password. hw-test trust changes that for some hours.")
			return 0
		}
		fmt.Fprintln(os.Stderr, said())
		return 0
	case end:
		report.EndTrust()
		// While the rule holds, it takes itself away without a password. Once it has run out it does
		// nothing any more, and its file needs the owner's password to remove
		if exec.Command("sudo", "-n", "rm", "-f", trustRule).Run() == nil {
			_ = exec.Command("sudo", "-k").Run()
			fmt.Fprintln(os.Stderr, "Taken back. The agent asks first again, and sudo asks for your password.")
			return 0
		}
		if guided.Interactive() {
			remove := exec.Command("sudo", "rm", "-f", trustRule)
			remove.Stdin, remove.Stdout, remove.Stderr = os.Stdin, os.Stderr, os.Stderr
			if remove.Run() == nil {
				fmt.Fprintln(os.Stderr, "Taken back. The agent asks first again, and sudo asks for your password.")
				return 0
			}
		}
		fmt.Fprintf(os.Stderr, "Taken back, and the agent asks first again. sudo's rule has run out or couldn't be removed from here. A rule that has run out does nothing. Its file goes with: sudo rm %s\n", trustRule)
		return 0
	}

	hours := trustUsual
	if len(words) > 0 {
		given, ok := trustHours(words[0])
		if !ok {
			return fail(fmt.Sprintf("trust takes a number of hours from 1 to %d, as in hw-test trust 12, or days from 1d to %dd, as in hw-test trust 3d. Without one it is %d hours.", trustMost, trustMost/24, trustUsual))
		}
		hours = given
	}
	// An agent doesn't lengthen its own time. The owner does that at their own terminal
	if inForce && !guided.Interactive() {
		fmt.Fprintln(os.Stderr, said())
		fmt.Fprintln(os.Stderr, "Only the owner makes it longer, with hw-test trust in their own terminal.")
		return 0
	}
	owner, err := user.Current()
	if err != nil || !trustUser.MatchString(owner.Username) || owner.Username == "root" {
		return fail("this user's name can't be written into a sudo rule, so nothing was changed")
	}
	if _, err := exec.LookPath("visudo"); err != nil {
		return fail("visudo isn't here to check the rule with, so nothing was changed")
	}
	dir := report.StateDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fail(err.Error())
	}
	until = time.Now().Add(time.Duration(hours) * time.Hour).Truncate(time.Second)
	what := "lets your agent work on its own until " + until.Local().Format("15:04 on 2 January") + ". It may then restart the computer and change system settings without asking first, and run commands as root without your password. Your files, the disk and firmware stay off limits, and hw-test trust --end takes it back"
	code, here := asOwnerRan(dir, "trust", strings.TrimSpace(strconv.Itoa(hours)), "trust.sh", trustScript(owner.Username, until), what, false)
	if code != 0 {
		return code
	}
	// The run that had the password keeps the time. An agent's run finds it kept by the owner's window
	if here {
		if err := report.SetTrust(until); err != nil {
			return fail(err.Error())
		}
	}
	if until, inForce = report.Trusted(time.Now()); inForce {
		fmt.Fprintln(os.Stderr, said())
	}
	return 0
}
