// Package resume brings the owner's agent back after a restart. Hardware work means
// restarting into a changed kernel or device tree, and an agent's session ends with the
// restart. Before it restarts, the agent leaves word of what to go on with. At the owner's
// next login a service that runs once opens the agent again with that word, and takes itself
// away.
package resume

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Expires is how long a word left before a restart stays good. An agent turning up days
// later, unasked, would only be a surprise.
const Expires = 24 * time.Hour

// Word is what was left before a restart: where the agent worked, what to tell it, and the
// boot it was left in, by which the next boot is told from the same one.
type Word struct {
	Dir    string    `json:"dir"`
	Prompt string    `json:"prompt"`
	Boot   string    `json:"boot"`
	Set    time.Time `json:"set"`
}

// Places are the files the word and its service live in. Tests point them at a folder of their own.
type Places struct {
	// State is the app's state folder, and Units the user's folder of systemd units.
	State, Units string
}

// Home is where the files live for the user running the app.
func Home() Places {
	home, _ := os.UserHomeDir()
	state := filepath.Join(home, ".local", "state", "arewearmyet")
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		state = filepath.Join(dir, "arewearmyet")
	}
	config := filepath.Join(home, ".config")
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		config = dir
	}
	return Places{State: state, Units: filepath.Join(config, "systemd", "user")}
}

const unitName = "hw-test-resume.service"

func (p Places) word() string { return filepath.Join(p.State, "resume.json") }
func (p Places) unit() string { return filepath.Join(p.Units, unitName) }
func (p Places) link() string {
	return filepath.Join(p.Units, "graphical-session.target.wants", unitName)
}

// quote puts a path in single quotes for a shell, whatever it holds.
func quote(text string) string { return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'" }

// Unit is the service that runs once at login. It goes through a login shell, so the agent
// starts with the same programs on its path as in the owner's terminal, and waits a moment
// for the desktop to be up.
func Unit(program string) string {
	return `[Unit]
Description=hw-test: bring the agent back after a restart
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=oneshot
ExecStart=/bin/bash -lc "sleep 5; exec ` + strings.ReplaceAll(quote(program), `"`, `\"`) + ` resume --run"

[Install]
WantedBy=graphical-session.target
`
}

// Leave keeps the word and puts the service in place for the next login.
func (p Places) Leave(word Word, program string) error {
	for _, dir := range []string{p.State, filepath.Dir(p.link())} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	data, _ := json.MarshalIndent(word, "", " ")
	if err := os.WriteFile(p.word(), data, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(p.unit(), []byte(Unit(program)), 0o644); err != nil {
		return err
	}
	os.Remove(p.link())
	if err := os.Symlink(p.unit(), p.link()); err != nil {
		return err
	}
	reload()
	return nil
}

// Left reads the word, if one was left.
func (p Places) Left() (Word, bool) {
	var word Word
	data, err := os.ReadFile(p.word())
	if err != nil || json.Unmarshal(data, &word) != nil || word.Dir == "" || word.Set.IsZero() {
		return Word{}, false
	}
	return word, true
}

// Clear takes the word and the service away.
func (p Places) Clear() {
	os.Remove(p.word())
	os.Remove(p.link())
	os.Remove(p.unit())
	reload()
}

// reload tells the user's systemd about the change. It doesn't matter when that fails: the
// files are read anew at the next login.
func reload() {
	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
}

// Decision is what to do with a word at login.
type Decision int

const (
	// Nothing was left, or it isn't time.
	Nothing Decision = iota
	// Wait: the computer hasn't restarted since the word was left, so it stays for when it has.
	Wait
	// Open the agent with the word. The word is used up by this.
	Open
	// Stale: the word is too old to act on, and is cleared.
	Stale
)

// At decides what a login in the given boot, at the given time, does with a word.
func At(word Word, found bool, boot string, now time.Time) Decision {
	switch {
	case !found:
		return Nothing
	case now.Sub(word.Set) > Expires || now.Before(word.Set.Add(-time.Minute)):
		return Stale
	case boot == "" || boot == word.Boot:
		return Wait
	}
	return Open
}

// Said is what the agent is told when it comes back, around whatever it left for itself.
func Said(word Word) string {
	text := "The computer has restarted and you are picking your hardware work up again. Read BRIEF.md and PROGRESS.md in this folder first."
	if word.Prompt != "" {
		text += " Before the restart you left yourself this: " + word.Prompt
	}
	return text + " Check with `uname -r` and `hw-test status` which kernel you are on now, and whether the change took."
}
