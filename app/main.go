// hw-test checks what works on a Snapdragon PC under Omarchy and reports it to arewearmyet.com.
//
//	hw-test            the guided test: the quick check, then keyboard, sound, camera and sleep with you
//	hw-test quick      the quick check alone: read-only, a few seconds, no questions
//	hw-test sound      one part of the guided test alone, reporting only that part
//	hw-test manual     the tests done with you: keyboard, touchpad, camera, ports, sleep
//	hw-test fix        hand what doesn't work to the owner's agent, with a brief of what was measured
//	hw-test auto       the tests that need nobody: graphics, browser, wireless, video, temperatures, sound
//
// Both show the results and send them to the model's page. What's sent: the maker, model, SKU,
// board and BIOS version, the hardware IDs made from those, the kernel, OS and a few package
// versions, and the results. No serial numbers, addresses, user or host names, and no files:
// the guided test's microphone recording and camera picture stay on the computer and are deleted
// when it ends.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/deep"
	"github.com/birkskyum/arewearmyet/app/internal/guided"
	"github.com/birkskyum/arewearmyet/app/internal/kernel"
	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/resume"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
	"github.com/birkskyum/arewearmyet/app/internal/tree"
)

// version is set when a release is built.
var version = "dev"

// terminal is what the owner types, read a line at a time by whatever asks.
var terminal = bufio.NewReader(os.Stdin)

func main() {
	// Whatever goes wrong, the owner is told in a sentence, not shown a stack of the program's insides
	defer func() {
		if fault := recover(); fault != nil {
			fmt.Fprintln(os.Stderr, "hw-test ran into a problem of its own and stopped:", fault)
			os.Exit(1)
		}
	}()

	// The words before the options: a command, or the steps to run alone
	args := os.Args[1:]
	command := "guided"
	var only, topics []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		word := args[0]
		args = args[1:]
		switch {
		case command == "brief" || command == "fix" || command == "verify" || command == "resume" || command == "tree" || command == "kernel" || command == "note" || command == "ask":
			// What follows these two names what to hand over: checks, or parts of the test
			topics = append(topics, word)
		case guided.KnownStep(word):
			only = append(only, word)
		case len(only) == 0 && command == "guided":
			command = word
		default:
			command = "unknown " + word
		}
	}
	flags := flag.NewFlagSet("hw-test "+command, flag.ExitOnError)
	jsonOnly := flags.Bool("json", false, "print what would be sent, and send nothing")
	play := flags.Bool("play", false, "for an agent, without a terminal: the owner has said yes to the sound test's tones")
	once := flags.Bool("once", false, "end after the named steps, without the last screen")
	done := flags.String("done", "", "with --once: a file to write the steps' results to")
	noSend := flags.Bool("no-send", false, "show the results without sending them")
	model := flags.String("model", "", "the model's id on arewearmyet.com, when the machine isn't recognised by itself")
	agent := flags.String("agent", "", "the agent running this, shown next to the run")
	api := flags.String("api", report.Site, "another server, for testing")
	// For hw-test note
	var logs, links list
	kind := flags.String("kind", "problem", "what the note is: problem, workaround, upstream or note")
	title := flags.String("title", "", "the note in a line")
	check := flags.String("check", "", "the check the note is about, when it is about one")
	body := flags.String("body", "", "the note's text; without it, read from --body-file or from standard input")
	bodyFile := flags.String("body-file", "", "a file with the note's text")
	reply := flags.String("reply", "", "the id of the note this answers")
	solved := flags.String("solved", "", "the id of a problem written from this computer, to mark solved")
	flags.Var(&logs, "log", "a log to attach, cleaned first: kernel, previous (the kernel's log of the boot before) or a file; may be given more than once")
	flags.Var(&links, "link", "a link to where the work is; may be given more than once")
	pr := flags.String("pr", "", "for hw-test verify after: the pull request the change is in")
	// For hw-test resume
	cancel := flags.Bool("cancel", false, "for hw-test resume: take back what was left for after the restart")
	show := flags.Bool("show", false, "for hw-test resume: show what is left for after the restart")
	// For hw-test verify
	again := flags.Bool("new", false, "for hw-test verify before: replace the snapshot that is there, to start on another change")
	atLogin := flags.Bool("run", false, "for hw-test resume: what the login service calls")
	// For hw-test report
	state := flags.String("state", "", "for hw-test report: works, partial, broken or absent")
	how := flags.String("how", "owner", "for hw-test report: owner (they saw or heard it) or agent (your reading of logs)")
	noteText := flags.String("note", "", "for hw-test report: what was seen, in a sentence or two")
	evidence := flags.String("evidence", "", "for hw-test report: the line of output that shows it")
	flags.Usage = usage
	flags.Parse(args)

	// An agent that calls hw-test from the hardware folder says which conversation it is in,
	// so the next part and the word after a restart can go into the same one
	if held := tools.CurrentSessions(); len(held) > 0 {
		work := guided.Workspace()
		if here, err := os.Getwd(); err == nil && (here == work || strings.HasPrefix(here, work+string(os.PathSeparator))) {
			_ = report.NoteSession(work, held[0].Kind, held[0].ID, time.Now())
		}
	}

	switch command {
	case "version", "--version":
		fmt.Println("hw-test", version)
		return
	case "help", "--help", "-h":
		usage()
		return
	case "update":
		os.Exit(update())
	case "resume":
		os.Exit(comeBack(strings.Join(topics, " "), *cancel, *show, *atLogin))
	case "tree", "kernel":
		// The patch's text, which says why, comes as a note's does
		why := *body
		if why == "" && *bodyFile != "" {
			data, err := os.ReadFile(*bodyFile)
			if err != nil {
				fmt.Fprintln(os.Stderr, "hw-test:", err)
				os.Exit(1)
			}
			why = string(data)
		}
		if command == "kernel" {
			os.Exit(kernelChange(topics, why))
		}
		os.Exit(deviceTree(topics, why))
	case "status":
		// The latest result of every check on this machine, without running anything
		if *jsonOnly {
			// The same as data, for an agent: every check's latest result, with when and on which kernel
			data, _ := json.MarshalIndent(report.Local(), "", "  ")
			fmt.Println(string(data))
			return
		}
		announce()
		fmt.Println(guided.Status(report.Local()))
		return
	case "ask":
		os.Exit(ask(topics))
	case "quick", "check", "guided", "facts", "brief", "fix", "note", "verify", "report":
		if command == "check" {
			command = "quick"
		}
	default:
		fmt.Fprintf(os.Stderr, "hw-test: %q is neither a command nor a step\n\n", strings.TrimPrefix(command, "unknown "))
		usage()
		os.Exit(2)
	}
	if len(only) > 0 && command != "guided" {
		fmt.Fprintf(os.Stderr, "hw-test: steps can't be combined with %s\n", command)
		os.Exit(2)
	}
	// The automatic tests need nobody, so they also run without a terminal, as an agent has it
	unattended := len(only) > 0 && !guided.Interactive()
	if unattended && !guided.Automatic(only, *play) {
		fmt.Fprintln(os.Stderr, "hw-test: that step is done with you at the keyboard, so it needs a terminal. Without one, these run: "+strings.Join(guided.AutomaticWords(), ", ")+". The sound test (speakers, microphone) runs without one with --play, once the owner has said yes to its tones. A step done with the owner is opened for them by hw-test ask, as in hw-test ask camera, which waits and prints the result.")
		os.Exit(2)
	}

	// A newer release is said once at the start. The menu says it itself.
	if !(command == "guided" && len(only) == 0 && guided.Interactive()) {
		announce()
	}

	run, facts, err := probe.Quick(machine.Real())
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		os.Exit(1)
	}
	run.Model, run.Agent = *model, *agent
	// Which version measured it goes along, so a result can be read knowing what the test did then
	if version != "dev" {
		if run.Packages == nil {
			run.Packages = map[string]string{}
		}
		run.Packages["hw-test"] = version
	}

	// One result the owner told the agent, or the agent read from logs
	if command == "report" {
		os.Exit(tell(run, *api, *check, *state, *how, *noteText, *evidence, !*noSend && !*jsonOnly))
	}

	// Before and after a change: what a pull request changed on this machine, as measured
	if command == "verify" {
		os.Exit(verify(run, topics, *api, *pr, *title, *check, *again, !*noSend && !*jsonOnly))
	}

	// A note: what fits no check, written down where other owners' agents read it
	if command == "note" {
		args := noteArgs{api: *api, kind: *kind, title: *title, check: *check, body: *body, bodyFile: *bodyFile, reply: *reply, solved: *solved,
			logs: logs, links: links, send: !*noSend && !*jsonOnly}
		// At a terminal, what wasn't given is asked for: hw-test note, or hw-test note wifi, is the whole command
		if args.solved == "" && args.body == "" && args.bodyFile == "" && guided.Interactive() {
			checks, _ := report.Checks(*api)
			asked, written := askNote(terminal, os.Stderr, checks, args, strings.Join(topics, " "))
			if !written {
				fmt.Fprintln(os.Stderr, "Nothing written.")
				os.Exit(0)
			}
			args = asked
		} else if len(topics) > 0 && args.check == "" {
			if id, known := part(topics[0], nil); known {
				args.check = id
			}
		}
		os.Exit(writeNote(run, args))
	}

	// The brief for an agent: printed, or written down and handed to the owner's agent
	if command == "brief" || command == "fix" {
		checks, unknown := guided.ChecksFor(topics)
		if len(unknown) > 0 {
			fmt.Fprintf(os.Stderr, "hw-test: %s is neither a check nor a part of the test. hw-test status lists the checks.\n", strings.Join(unknown, ", "))
			os.Exit(2)
		}
		// Results from before stand; what was never tested here is as the quick check found it now
		kept := report.Local()
		for _, result := range run.Results {
			if _, had := kept[result.Check]; !had {
				kept[result.Check] = report.Kept{Result: result, Kernel: run.Kernel, When: time.Now()}
			}
		}
		known, _ := report.Checks(*api)
		matched, _ := report.Match(*api, run)
		// An agent takes one part at a time, so one that can't be fixed doesn't hold up the others.
		// Nothing is tested again for this: it goes by the results already on this computer
		if candidates := guided.Failing(kept); command == "fix" && len(checks) == 0 && len(candidates) > 1 && guided.Interactive() {
			picked, ok := pickOne(terminal, os.Stderr, guided.Summed(kept, candidates))
			if !ok {
				fmt.Fprintln(os.Stderr, "Nothing handed over.")
				return
			}
			checks = []string{candidates[picked]}
		}
		briefing := guided.Briefing{Version: version, Run: run, Kept: kept, Model: matched, Checks: known, Only: checks, When: time.Now()}
		briefing.Binary = guided.Command()
		if command == "fix" {
			briefing.GetGuide = guided.FetchGuide
			briefing.InThread = guided.InThread
			// Handing over at a terminal is the owner's own doing
			briefing.ByOwner = guided.Interactive()
		}
		briefing.Fill(*api)
		if command == "brief" {
			fmt.Print(guided.Brief(briefing))
			return
		}
		handed := guided.HandOver(briefing, guided.Workspace(), tools.Agent)
		fmt.Fprintln(os.Stderr, handed.Said)
		if !handed.Opened {
			os.Exit(1)
		}
		return
	}

	if unattended {
		dir, err := os.MkdirTemp("", "hw-test-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "hw-test:", err)
			os.Exit(1)
		}
		say := func(line string) { fmt.Fprintln(os.Stderr, line) }
		guided.Unattended(run, deep.Tests(machine.Real(), facts.Wifi, facts.Bluetooth, facts.Ethernet, dir), only, say)
		// The sound test plays tones out loud, so it runs here only on the owner's yes: their
		// handing the speakers or the microphones to an agent
		if guided.WantsSound(only) && (*play || guided.MaySound(time.Now())) && facts.SoundCard != "" && tools.Have("pw-play", "pw-record") {
			guided.Heard(run, dir, say)
		}
		os.RemoveAll(dir)
		fmt.Fprintln(os.Stderr)
	}

	if *jsonOnly {
		fmt.Println(string(run.JSON()))
		return
	}
	// What the test found to work with, for when a step doesn't behave as it should
	if command == "facts" {
		fmt.Printf("%+v\n", facts)
		for _, line := range guided.Describe(machine.Real(), facts.TouchscreenDevice) {
			fmt.Println(line)
		}
		for _, tool := range []string{"pw-play", "pw-record", "cam", "wpctl", "hyprctl", "systemd-analyze", "xdg-open", "vulkaninfo", "chromium", "ffmpeg", "iw", "bluetoothctl"} {
			fmt.Printf("%-16s %v\n", tool, tools.Have(tool))
		}
		return
	}
	// The guided test needs someone at the keyboard; without a terminal it is the quick check
	if command == "guided" && guided.Interactive() {
		// The quick results are kept aside: if the guided test can't run here, they are still sent
		measured := append([]report.Result(nil), run.Results...)
		err := guided.Run(run, facts, guided.Options{API: *api, Send: !*noSend, Only: only, Version: version, Once: *once})
		// The steps' results go back to whoever opened them for the owner
		if *done != "" {
			results := run.Results
			if err != nil || !*once {
				results = nil
			}
			data, _ := json.MarshalIndent(map[string]any{"steps": only, "results": results}, "", "  ")
			_ = os.WriteFile(*done, append(data, '\n'), 0o600)
		}
		if err == nil {
			return
		}
		fmt.Fprintln(os.Stderr, "hw-test: the guided test couldn't run here:", err)
		if len(only) > 0 {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Going on with what could be measured.")
		run.Mode, run.Results = "quick", measured
	}

	// Kept on this machine as its latest results, whether or not they are sent
	_ = report.Remember(run, time.Now())
	printResults(run)
	if *noSend {
		fmt.Fprintln(os.Stderr, "Not sent.")
		return
	}
	sent, err := report.Send(*api, run)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Sending failed:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Sent as %s.\n", sent.Run)
	if sent.Page != "" {
		fmt.Fprintln(os.Stderr, "It shows on", sent.Page)
	} else {
		fmt.Fprintln(os.Stderr, "The board doesn't know this machine yet, so the run shows on no page.")
		fmt.Fprintln(os.Stderr, "Add it with this form, which has its name and hardware IDs filled in:")
		fmt.Fprintln(os.Stderr, " ", report.AddModelURL(run, sent.Run))
	}
}

func printResults(run *report.Run) {
	fmt.Fprintf(os.Stderr, "%s %s", run.Hardware["maker"], run.Hardware["product"])
	if family := run.Hardware["family"]; family != "" {
		fmt.Fprintf(os.Stderr, " (%s)", family)
	}
	fmt.Fprintf(os.Stderr, "\nkernel %s on %s\n\n", run.Kernel, run.OS)
	for _, result := range run.Results {
		fmt.Fprintf(os.Stderr, "  %-17s %-8s %s\n", result.Check, result.State, result.Evidence)
	}
	fmt.Fprintln(os.Stderr)
}

// announce says when a newer release is out. It never holds a start up for long and never
// stops one.
func announce() {
	if newer := report.Newer(version, time.Now()); newer != "" {
		fmt.Fprintf(os.Stderr, "hw-test %s is out, and this is %s. Update with: hw-test update\n\n", newer, version)
	}
}

// tell sends one result that no test measured: what the owner saw, or what the agent read.
func tell(run *report.Run, api, check, state, how, note, evidence string, send bool) int {
	fail := func(text string) int {
		fmt.Fprintln(os.Stderr, "hw-test: "+text)
		return 2
	}
	if checks, unknown := guided.ChecksFor([]string{check}); check == "" || len(unknown) > 0 || len(checks) != 1 || checks[0] != check {
		return fail("report needs --check with a check's id, like suspend. hw-test status lists them.")
	}
	// absent is for a part the computer turns out not to have, like a headphone jack on a
	// laptop with only USB-C ports: it takes back a "broken" that was recorded for it
	if state != report.Works && state != report.Partial && state != report.Broken && state != report.Absent {
		return fail("report needs --state works, partial, broken or absent (the computer doesn't have the part).")
	}
	if how != report.Owner && how != "agent" {
		return fail("--how is owner (they saw or heard it and told you) or agent (your own reading of logs). What a test measured is sent by running the test.")
	}
	if strings.TrimSpace(note) == "" {
		return fail("report needs --note: what was seen, in a sentence or two.")
	}
	if run.Model == "" {
		if matched, err := report.Match(api, run); err == nil {
			run.Model = matched.ID
		}
	}
	// This result alone: the site keeps each check's newest result per machine, so the
	// others this machine has sent stand as they are
	run.Mode = "agent"
	run.Results = nil
	run.Quick = nil
	run.Set(check, state, how, evidence, note)
	_ = report.Remember(run, time.Now())
	if !send {
		fmt.Println(string(run.JSON()))
		fmt.Fprintln(os.Stderr, "Not sent.")
		return 0
	}
	sent, err := report.Send(api, run)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test: the site didn't take it:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Sent as %s. %s\n", sent.Run, first(sent.Page, "The board doesn't know this computer yet."))
	return 0
}

// asOwner runs a step that needs the owner's password: written out as a script to read, and
// run with sudo where there is a terminal to ask in. Without one, as with an agent, the same
// step is opened for the owner in a window of their own, and waited for.
func asOwner(dir, command, step, file, script, what string) int {
	fail := func(err error) int {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		return 1
	}
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return fail(err)
	}
	if !guided.Interactive() {
		// No terminal here to ask for the password in, as with an agent: the step is opened
		// for the owner in a window of their own, and waited for
		self, _ := os.Executable()
		quote := func(text string) string { return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'" }
		answered := filepath.Join(dir, "."+file+".done")
		os.Remove(answered)
		line := fmt.Sprintf("cd %s && %s %s %s; echo $? > %s; echo; read -n1 -rsp 'Press a key to close this window.'", quote(dir), quote(self), command, step, quote(answered))
		if err := tools.Terminal("bash", "-c", line); err == nil {
			fmt.Fprintf(os.Stderr, "This %s. It needs the owner's password, so it is open for them in a window of its own. Waiting until they are through, fifteen minutes at most.\n", what)
			for waited := time.Duration(0); waited < 15*time.Minute; waited += 500 * time.Millisecond {
				if data, err := os.ReadFile(answered); err == nil && len(strings.TrimSpace(string(data))) > 0 {
					os.Remove(answered)
					if code := strings.TrimSpace(string(data)); code != "0" {
						return fail(fmt.Errorf("it didn't go through in the owner's window (it ended with %s)", code))
					}
					fmt.Fprintln(os.Stderr, "Done in the owner's window.")
					return 0
				}
				time.Sleep(500 * time.Millisecond)
			}
			return fail(fmt.Errorf("the owner didn't get to it"))
		}
		fmt.Fprintf(os.Stderr, "This step needs the owner's password, and there is no terminal here to ask for it.\nAsk the owner to run this in their own terminal:\n\n  cd %s && hw-test %s %s\n\nIt %s. What it runs is in %s.\n", dir, command, step, what, path)
		return 3
	}
	fmt.Fprintf(os.Stderr, "This %s. It needs your password. What it runs is in %s.\n\n", what, path)
	sudo := exec.Command("sudo", "bash", path)
	sudo.Stdin, sudo.Stdout, sudo.Stderr = os.Stdin, os.Stderr, os.Stderr
	if err := sudo.Run(); err != nil {
		return fail(fmt.Errorf("it didn't go through: %v", err))
	}
	return 0
}

// deviceTree is the way to try a changed device tree: the sources, a build, a boot entry of
// its own, and taking it away again.
func deviceTree(words []string, body string) int {
	system := tree.Real()
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		return 1
	}
	step := ""
	if len(words) > 0 {
		step = words[0]
	}
	fail := func(err error) int {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		return 1
	}
	asRoot := func(file, script, what string) int { return asOwner(dir, "tree", step, file, script, what) }
	switch step {
	case "start":
		started, err := system.Start(dir)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "The device tree sources of linux-omarchy %s are in %s, under git, with %d of Omarchy's patches from %s laid over them, as its package builds them.\n", system.Kernel, started.Root, started.Applied, started.Recipe)
		if len(started.Skipped) > 0 {
			fmt.Fprintf(os.Stderr, "%d patches were passed over, since this kernel's sources have them already.\n", len(started.Skipped))
		}
		if started.Machine != "" {
			fmt.Fprintf(os.Stderr, "This computer's tree is arch/arm64/boot/dts/qcom/%s.dts, with the files it includes.\n", started.Machine)
		} else {
			fmt.Fprintln(os.Stderr, "This computer's tree wasn't found among them by itself. Name it when you build: hw-test tree build <name>, without .dts")
		}
		if len(started.Misfit) > 0 {
			named := started.Misfit
			if len(named) > 4 {
				named = append(append([]string{}, named[:4]...), fmt.Sprintf("and %d more", len(started.Misfit)-4))
			}
			fmt.Fprintf(os.Stderr, "%d of Omarchy's patches don't fit this kernel's sources and were left out (%s). That happens when the computer is behind Omarchy's packages or on another channel.\n", len(started.Misfit), strings.Join(named, ", "))
		}
		switch {
		case started.Stock == "same" && len(started.Misfit) > 0:
			fmt.Fprintln(os.Stderr, "They don't touch this computer's tree: compiled unchanged, it is the tree this computer starts with, byte for byte. So a test image differs by your change and nothing else. A patch for Omarchy still has to fit its current sources, which the pull request's build shows.")
		case started.Stock == "same":
			fmt.Fprintln(os.Stderr, "Compiled unchanged, it is the tree this computer starts with, byte for byte. So a test image differs by your change and nothing else.")
		case started.Stock == "differs":
			fmt.Fprintln(os.Stderr, "Compiled unchanged, it is NOT the tree this computer starts with: the installed linux-omarchy-dtbs and Omarchy's packages as they are now differ for this model. A test image would carry that difference besides your change. Have the owner run omarchy update and restart, then move this folder's tree away and start again.")
		case len(started.Misfit) > 0:
			fmt.Fprintln(os.Stderr, "Whether that touches this computer's tree couldn't be told. Have the owner run omarchy update and restart, then move this folder's tree away and start again.")
		}
		fmt.Fprintln(os.Stderr, "Change it, then: hw-test tree build")
		return 0
	case "build":
		machine := ""
		if len(words) > 1 {
			machine = strings.TrimSuffix(words[1], ".dts")
		} else if machine, err = system.MachineIn(dir); err != nil {
			return fail(err)
		}
		built, err := system.Build(dir, machine)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "Built %s\n", built.DTB)
		if len(built.Changed) == 0 {
			fmt.Fprintln(os.Stderr, "Nothing differs from Omarchy's sources yet, so this is the stock tree.")
		} else {
			fmt.Fprintf(os.Stderr, "Changed against Omarchy's sources: %s\nThe change as a diff: %s\n", strings.Join(built.Changed, ", "), built.Diff)
		}
		fmt.Fprintln(os.Stderr, "To try it from a boot entry of its own, the owner runs: hw-test tree boot")
		return 0
	case "patch":
		patched, err := system.Patch(dir, strings.Join(words[1:], " "), body)
		if err != nil {
			return fail(err)
		}
		name := filepath.Base(patched.File)
		fmt.Fprintf(os.Stderr, "The change is written as one patch: %s\nIt touches %s.\n\n", patched.File, strings.Join(patched.Changed, ", "))
		if !patched.Numbered {
			fmt.Fprintf(os.Stderr, "Omarchy's device tree package wasn't found at %s, so the number in its name is a guess. Give it the next free number among the package's patches.\n", patched.Recipe)
		}
		fmt.Fprintf(os.Stderr, "To put it into Omarchy's package, in %s:\n  1. Copy it there.\n  2. In PKGBUILD, add %s to source after the last patch, and its sum at the same place in b2sums: b2sum %s\n  3. Raise pkgrel by one.\n  4. Add a paragraph on it to README.md.\nRunning hw-test tree patch again writes the patch anew from the sources as they are then.\n", patched.Recipe, name, name)
		return 0
	case "boot":
		machine := ""
		if len(words) > 1 {
			machine = strings.TrimSuffix(words[1], ".dts")
		} else if machine, err = system.MachineIn(dir); err != nil {
			return fail(err)
		}
		dtb := filepath.Join(dir, machine+".dtb")
		if _, err := os.Stat(dtb); err != nil {
			return fail(fmt.Errorf("there is no %s yet. Build it first: hw-test tree build", dtb))
		}
		started, err := os.ReadFile("/proc/cmdline")
		if err != nil {
			return fail(err)
		}
		code := asRoot("tree-boot.sh", system.Script(dtb, string(started)), "puts the changed device tree into a boot image and a boot menu entry of its own, "+tree.Entry+". The usual entry is not touched")
		if code == 0 {
			// Noted so that a system update isn't started while the entry stands (hw-test ask update)
			_ = report.NoteTestEntry(system.Kernel, time.Now())
		}
		return code
	case "remove":
		code := asRoot("tree-remove.sh", tree.RemoveScript(), "takes the test entry "+tree.Entry+" and its boot image away")
		if code == 0 {
			report.ClearTestEntry()
		}
		return code
	case "status":
		if tree.Running("/proc/device-tree") {
			fmt.Println("This computer is running a device tree built by hw-test tree build: the test entry is what started.")
		} else {
			fmt.Println("This computer is running Omarchy's own device tree: the usual entry is what started.")
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, "hw-test: tree takes start, build, boot, status, patch or remove. Start with: hw-test tree start")
	return 2
}

// kernelChange is the way to try a change in one kernel driver: Omarchy's kernel source as
// the running kernel was built from it, one driver folder built against the running kernel,
// the result loaded until the next restart, and the change written as a patch.
func kernelChange(words []string, body string) int {
	system := kernel.Real()
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		return 1
	}
	step := ""
	if len(words) > 0 {
		step = words[0]
	}
	fail := func(err error) int {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		return 1
	}
	// The modules of the last build, and the ones picked to load, kept beside the source
	builtList, loadList := filepath.Join(dir, "kernel", "built"), filepath.Join(dir, "kernel", "load")
	lines := func(file string) []string {
		data, _ := os.ReadFile(file)
		return strings.Fields(string(data))
	}
	switch step {
	case "start":
		fmt.Fprintf(os.Stderr, "Fetching Omarchy's kernel source for linux-omarchy %s and applying its patches. This takes a few minutes and about 3 GB.\n", system.Release)
		started, err := system.Start(dir)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "The kernel source is in %s, under git, with Omarchy's %d patches applied, and nothing is built.\n", started.Source, started.Patches)
		if !started.Exact {
			fmt.Fprintf(os.Stderr, "Omarchy's recipe for exactly linux-omarchy %s wasn't found, so this is its newest. A driver built from it may not fit the running kernel: have the owner run omarchy update and restart first, then start again.\n", system.Release)
		}
		fmt.Fprintln(os.Stderr, "Change a driver, then build its folder: hw-test kernel build drivers/...")
		return 0
	case "build":
		built, err := system.Build(dir, words[1:])
		if err != nil {
			return fail(err)
		}
		_ = os.WriteFile(builtList, []byte(strings.Join(built.Modules, "\n")+"\n"), 0o644)
		os.Remove(loadList)
		fmt.Fprintf(os.Stderr, "Built %d modules for the running kernel %s:\n", len(built.Modules), system.Kernel)
		for _, module := range built.Modules {
			fmt.Fprintln(os.Stderr, "  "+module)
		}
		if len(built.Changed) == 0 {
			fmt.Fprintln(os.Stderr, "Nothing differs from Omarchy's source yet, so these are the kernel's own drivers built again.")
		} else {
			fmt.Fprintf(os.Stderr, "Changed against Omarchy's source: %s\nThe change as a diff: %s\n", strings.Join(built.Changed, ", "), built.Diff)
		}
		fmt.Fprintln(os.Stderr, "To try one until the next restart: hw-test kernel load <module>")
		return 0
	case "load":
		all := lines(builtList)
		if len(all) == 0 {
			return fail(fmt.Errorf("nothing is built yet. Build the driver's folder first: hw-test kernel build drivers/..."))
		}
		picked := lines(loadList)
		if len(words) > 1 {
			picked = nil
			for _, name := range words[1:] {
				want := strings.ReplaceAll(strings.TrimSuffix(filepath.Base(name), ".ko"), "-", "_")
				found := ""
				for _, module := range all {
					if strings.ReplaceAll(strings.TrimSuffix(filepath.Base(module), ".ko"), "-", "_") == want {
						found = module
					}
				}
				if found == "" {
					return fail(fmt.Errorf("%s isn't among the modules of the last build. hw-test kernel build lists them", name))
				}
				picked = append(picked, found)
			}
			_ = os.WriteFile(loadList, []byte(strings.Join(picked, "\n")+"\n"), 0o644)
		}
		if len(picked) == 0 {
			return fail(fmt.Errorf("name the module to load, as in: hw-test kernel load %s", strings.TrimSuffix(filepath.Base(all[0]), ".ko")))
		}
		return asOwner(dir, "kernel", step, "kernel-load.sh", system.LoadScript(picked), "loads the built modules in place of the kernel's own, until the next restart. Nothing is written to disk")
	case "restore":
		if len(system.Loaded()) == 0 {
			fmt.Fprintln(os.Stderr, "No test module is loaded: the kernel's own drivers are what runs.")
			return 0
		}
		return asOwner(dir, "kernel", step, "kernel-restore.sh", system.RestoreScript(), "takes the test modules out and loads the kernel's own again")
	case "status":
		if loaded := system.Loaded(); len(loaded) > 0 {
			fmt.Printf("Test modules loaded since this start, in place of the kernel's own: %s. A restart, or hw-test kernel restore, brings the kernel's own back.\n", strings.Join(loaded, ", "))
		} else {
			fmt.Println("No test module is loaded: the kernel's own drivers are what runs.")
		}
		return 0
	case "patch":
		patched, err := system.Patch(dir, strings.Join(words[1:], " "), body)
		if err != nil {
			return fail(err)
		}
		name := filepath.Base(patched.File)
		fmt.Fprintf(os.Stderr, "The change is written as one patch: %s\nIt touches %s.\n\n", patched.File, strings.Join(patched.Changed, ", "))
		fmt.Fprintf(os.Stderr, "To put it into Omarchy's kernel, in pkgbuilds/linux-omarchy-aarch64 of omarchy-pkgs:\n  1. Copy it there. Its number is the next after the recipe's last. The recipe's README says which numbers belong to what, so give it one from the right group.\n  2. In PKGBUILD, add %s to source after the last patch, and its checksum at the same place: sha256sum %s\n  3. Raise pkgrel by one, and say in the README what the patch does.\n", name, name)
		return 0
	}
	fmt.Fprintln(os.Stderr, "hw-test: kernel takes start, build, load, status, restore or patch. Start with: hw-test kernel start")
	return 2
}

// comeBack leaves word for after a restart, or at login acts on a word that was left.
func comeBack(prompt string, cancel, show, atLogin bool) int {
	places := resume.Home()
	word, found := places.Left()
	switch {
	case cancel:
		places.Clear()
		fmt.Fprintln(os.Stderr, "Nothing will open after the next restart.")
		return 0
	case show:
		if !found {
			fmt.Fprintln(os.Stderr, "Nothing is left for after a restart.")
			return 0
		}
		fmt.Fprintf(os.Stderr, "After the next restart and login, the agent opens in %s and is told:\n\n%s\n\nLeft at %s. hw-test resume --cancel takes it back.\n", word.Dir, resume.Said(word), word.Set.Format("2 Jan 15:04"))
		return 0
	case atLogin:
		// Called by the service at login. Whatever happens here, a login is never held up or failed
		defer func() { _ = recover() }()
		switch resume.At(word, found, report.BootID(), time.Now()) {
		case resume.Open:
			// Used up before the agent opens, so nothing can make it open twice
			places.Clear()
			// The launcher is waited for: this service ends here, and would take a launcher that is
			// still at it down with it. The owner is told how it went either way
			said := []string{"Your agent is back", "It goes on with the hardware work from before the restart. hw-test resume left this."}
			// In the conversation the word was left from, where its agent said which that is:
			// everything it had learnt is still there. Otherwise, or if that can't be opened, a
			// new one, which reads the notes
			err := tools.ErrNoSession
			for _, held := range word.Sessions {
				if err = tools.Continue(tools.Session{Kind: held.Kind, ID: held.ID}, word.Dir, resume.Said(word)); err == nil {
					break
				}
			}
			if err == nil {
				time.Sleep(3 * time.Second)
			} else {
				err = tools.AgentHandedOver(word.Dir, resume.Said(word))
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "hw-test: the agent couldn't be opened:", err)
				said = []string{"Your agent didn't open", "The hardware work from before the restart is in " + word.Dir + ". Open your agent there and tell it to read PROGRESS.md."}
			}
			if tools.Have("notify-send") {
				_ = exec.Command("notify-send", "-a", "hw-test", said[0], said[1]).Run()
			}
		case resume.Stale:
			places.Clear()
		}
		return 0
	}
	dir, err := os.Getwd()
	if err != nil {
		dir = guided.Workspace()
	}
	// The command the owner types, when it is installed, so the service outlives an update
	program, _ := os.Executable()
	if home, err := os.UserHomeDir(); err == nil {
		if installed := filepath.Join(home, ".local", "bin", "hw-test"); tools.Have(installed) {
			program = installed
		}
	}
	var sessions []resume.Held
	// The thread hw-test started for this folder comes first: its agent isn't told which thread it is in
	if kept, ok := report.ThreadFor(dir); ok {
		sessions = append(sessions, resume.Held{Kind: kept.Kind, ID: kept.ID})
	}
	for _, session := range tools.CurrentSessions() {
		sessions = append(sessions, resume.Held{Kind: session.Kind, ID: session.ID})
	}
	if err := places.Leave(resume.Word{Dir: dir, Prompt: prompt, Boot: report.BootID(), Set: time.Now(), Sessions: sessions}, program); err != nil {
		fmt.Fprintln(os.Stderr, "hw-test: it couldn't be left for after the restart:", err)
		return 1
	}
	how := "the agent opens again in " + dir
	if len(sessions) > 0 {
		how = "this same conversation goes on, in " + dir
	}
	fmt.Fprintf(os.Stderr, "Left for after the restart. At the owner's next login, %s, once, and is told to read BRIEF.md and PROGRESS.md there.\nWrite PROGRESS.md before you restart. hw-test resume --cancel takes this back.\n", how)
	return 0
}

// tool is how mise names the app.
const tool = "github:birkskyum/arewearmyet"

// update gets the newest release through mise, which is how the app is installed.
func update() int {
	if !tools.Have("mise") {
		fmt.Fprintln(os.Stderr, "hw-test is installed through mise, which isn't on this system. The releases are at https://github.com/birkskyum/arewearmyet/releases")
		return 1
	}
	// The version is asked for by its number: mise remembers what "latest" was for a while,
	// and would answer with the release this already is
	wanted := report.NewestRelease()
	switch {
	case wanted == "":
		fmt.Fprintln(os.Stderr, "hw-test: the newest release couldn't be looked up. Is the network up? omarchy update also brings hw-test up to date.")
		return 1
	case !report.After(wanted, version) && version != "dev":
		fmt.Fprintf(os.Stderr, "This is the newest release, %s.\n", version)
		report.ForgetRelease()
		return 0
	}
	// Without mise's waiting time for new releases
	env := append(os.Environ(), "MISE_MINIMUM_RELEASE_AGE=0")
	install := func(extra ...string) error {
		command := exec.Command("mise", "install", tool+"@"+wanted)
		command.Env, command.Stdout, command.Stderr = append(env, extra...), os.Stderr, os.Stderr
		return command.Run()
	}
	// mise's usual way first, which works from any network. Its list of versions can be a
	// few minutes behind a release, so a failure is tried once more by asking GitHub itself,
	// which a network address may only do sixty times an hour.
	if err := install(); err != nil {
		if err = install("MISE_USE_VERSIONS_HOST=0"); err != nil {
			fmt.Fprintln(os.Stderr, "hw-test: the update didn't go through:", err)
			return 1
		}
	}
	report.ForgetRelease()
	now := exec.Command("mise", "x", tool+"@"+wanted, "--", "hw-test", "version")
	now.Env = env
	out, err := now.Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Installed. Start hw-test again to use it.")
		return 0
	}
	fmt.Fprintf(os.Stderr, "Now on %s (this run was %s).\n", strings.TrimSpace(string(out)), version)
	return 0
}

// verify takes the snapshot a change is measured against, or measures the change against it.
func verify(run *report.Run, words []string, api, pr, what, rows string, again, send bool) int {
	now := time.Now()
	// verify before speakers: a snapshot for that part alone. Without a part it is the one
	// snapshot for whatever is being changed
	step, part := "", ""
	if len(words) == 1 || len(words) == 2 {
		step = words[0]
	}
	if len(words) == 2 {
		if part = words[1]; !report.PartNamed(part) {
			fmt.Fprintf(os.Stderr, "hw-test: %q can't name a part. Use a word like speakers, camera or webgpu.\n", part)
			return 2
		}
	}
	named := func(command string) string { return strings.TrimSpace(command + " " + part) }
	testImage := tree.Running("/proc/device-tree")
	switch step {
	case "before":
		// A snapshot that is still being measured against is not written over: after a
		// restart, a second one would hold the change already and show it as no change
		if standing, ok := report.LoadSnapshot(part); ok && !standing.Used && !again && len(standing.Results) > 0 {
			fmt.Fprintf(os.Stderr, "hw-test: a snapshot from %s is there already, and no change has been recorded against it yet.\nIf you are in the middle of that change (after a restart, say), leave it: %s measures against it.\nTo start on another change to the same part: %s --new\nTo work on another part, give its name, so each part has a snapshot of its own: hw-test verify before <part>\n", standing.When.Format("2 Jan 15:04"), named("hw-test verify after"), named("hw-test verify before"))
			return 2
		}
		snapshot, err := report.TakeSnapshot(run, now, part, testImage)
		if err != nil {
			fmt.Fprintln(os.Stderr, "hw-test: the snapshot couldn't be kept:", err)
			return 1
		}
		failing := guided.Failing(snapshot.Results)
		fmt.Fprintf(os.Stderr, "Snapshot taken: %d checks, %d of them failing", len(snapshot.Results), len(failing))
		if len(failing) > 0 {
			fmt.Fprintf(os.Stderr, " (%s)", strings.Join(failing, ", "))
		}
		fmt.Fprintln(os.Stderr, ".")
		if len(snapshot.Results) == 0 {
			fmt.Fprintln(os.Stderr, "Nothing has been tested on this computer yet, so there is nothing to measure a change against. Run the tests first, then take the snapshot again.")
		}
		fmt.Fprintln(os.Stderr, "\nNow make the change. Then test the parts it touches again, like hw-test speakers or hw-test graphics,\nand measure the change against this snapshot:\n\n  "+named("hw-test verify after")+" --pr <the pull request's address>")
		return 0
	case "after":
		snapshot, ok := report.LoadSnapshot(part)
		if !ok {
			standing := report.StandingParts()
			switch {
			case part == "" && len(standing) > 0:
				fmt.Fprintf(os.Stderr, "hw-test: there is a snapshot for %s, taken with the part's name. Name the part: hw-test verify after %s\n", strings.Join(standing, ", "), standing[0])
			case len(standing) > 0:
				fmt.Fprintf(os.Stderr, "hw-test: there is no snapshot for %s. There is one for: %s. Take one before the change: hw-test verify before %s\n", part, strings.Join(standing, ", "), part)
			default:
				fmt.Fprintf(os.Stderr, "hw-test: there is no snapshot to measure against. Take one before the change: %s\n", named("hw-test verify before"))
			}
			return 2
		}
		changes := report.Changes(snapshot, report.Latest())
		if len(changes) == 0 {
			fmt.Fprintf(os.Stderr, "hw-test: nothing has been tested since the snapshot of %s. Test the parts the change touches again, like hw-test speakers, then run this again.\n", snapshot.When.Format("2 Jan 15:04"))
			return 2
		}
		model := report.Model{ID: run.Model}
		if matched, err := report.Match(api, run); err == nil && matched.ID != "" {
			model = matched
		}
		var shown []string
		for _, row := range strings.Split(rows, ",") {
			if row = strings.TrimSpace(row); row != "" {
				shown = append(shown, row)
			}
		}
		fmt.Println(guided.VerifyBlock(run, model, snapshot, changes, testImage, shown...))
		if pr == "" {
			fmt.Fprintln(os.Stderr, "That is the block for the pull request's description. To record it on the model's page too, give the pull request: hw-test verify after --pr <its address>")
			return 0
		}
		if !send {
			fmt.Fprintln(os.Stderr, "Not sent.")
			return 0
		}
		if model.ID == "" {
			fmt.Fprintln(os.Stderr, "hw-test: the board doesn't know this computer, so there is no page to record it on. Name the model with --model.")
			return 2
		}
		sent, err := report.SendFix(api, report.Fix{Model: model.ID, URL: pr, Note: what, Changes: changes,
			Before: snapshot.Versions, After: report.Versions{Kernel: run.Kernel, Packages: run.Packages}})
		if err != nil {
			fmt.Fprintln(os.Stderr, "hw-test: the site didn't take it:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "Recorded as %s on %s\nWhat this pull request changed, over every machine that measured it: %s\n", sent.Fix, first(sent.Page, model.Page), sent.URL)
		// From here the snapshot has served, and the next change takes its own
		_ = report.MarkSnapshotUsed(part)
		return 0
	}
	fmt.Fprintln(os.Stderr, "hw-test: verify takes before or after. Take a snapshot with hw-test verify before, make the change, test again, then hw-test verify after --pr <address>.")
	return 2
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// list is an option that may be given more than once.
type list []string

func (l *list) String() string     { return strings.Join(*l, ", ") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

type noteArgs struct {
	api, kind, title, check, body, bodyFile, reply, solved string
	logs, links                                            []string
	send                                                   bool
}

// attach reads a log by name and cleans it of what names the owner: the kernel's log of this
// boot or the one before, or a file.
func attach(name string) (report.Log, error) {
	sys := machine.Real()
	host, account, names := owner()
	var text string
	var err error
	logName := name + ".log"
	// boot-3 is the kernel's log of the third boot before this one, for a fault that isn't the last one's
	back := regexp.MustCompile(`^boot-(\d{1,3})$`).FindStringSubmatch(name)
	switch {
	case name == "kernel":
		text, err = sys.RunFor(20*time.Second, "journalctl", "-k", "-b", "--no-hostname", "-o", "short-monotonic", "--no-pager")
	case name == "previous":
		text, err = sys.RunFor(20*time.Second, "journalctl", "-k", "-b", "-1", "--no-hostname", "-o", "short-monotonic", "--no-pager")
	case back != nil:
		text, err = sys.RunFor(20*time.Second, "journalctl", "-k", "-b", "-"+back[1], "--no-hostname", "-o", "short-monotonic", "--no-pager")
	default:
		var data []byte
		if data, err = os.ReadFile(name); err == nil && len(data) > 8<<20 {
			err = fmt.Errorf("it is over 8 MB: cut it down to the part that shows the problem")
		}
		text = string(data)
		logName = strings.ToLower(filepath.Base(name))
		logName = strings.Trim(regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(logName, "-"), "-.")
		if logName == "" {
			logName = "attached.log"
		}
	}
	if err != nil && strings.TrimSpace(text) == "" {
		return report.Log{}, fmt.Errorf("%s: %w", name, err)
	}
	clean := report.ScrubLog(text, host, account, names...)
	if clean == "" {
		return report.Log{}, fmt.Errorf("%s is empty", name)
	}
	return report.Log{Name: logName, Text: clean}, nil
}

// owner gives the names a log could hold of its owner: the computer's, the account's, and the
// words of their full name.
func owner() (host, account string, names []string) {
	host, _ = os.Hostname()
	account = os.Getenv("USER")
	if current, err := user.Current(); err == nil {
		if account == "" {
			account = current.Username
		}
		names = strings.Fields(strings.Split(current.Name, ",")[0])
	}
	return host, account, names
}

// attached says in a line or two which logs go with a note and what was taken out of them.
// Reading a log first is the owner's choice, with --json: nobody is shown one unasked.
func attached(logs []report.Log) string {
	var listed []string
	for _, log := range logs {
		lines := strings.Count(log.Text, "\n") + 1
		listed = append(listed, fmt.Sprintf("%s (%d line%s)", log.Name, lines, map[bool]string{true: "", false: "s"}[lines == 1]))
	}
	return "Attaching " + strings.Join(listed, ", ") + ", with " + report.Removed + " taken out.\n" +
		"It goes on the model's page, for others with the same computer. To read it first, run this again with --json.\n"
}

// ask opens steps that are done with the owner in a terminal window on their desktop, waits
// until they are through, and prints what the steps found. It is how an agent, which has no
// terminal the owner sits at, has the owner do a test without anything being retyped.
func ask(steps []string) int {
	// The system update is a step for the owner too, with its own care (update.go)
	if len(steps) == 1 && steps[0] == "update" {
		return askUpdate()
	}
	if len(steps) == 0 {
		fmt.Fprintln(os.Stderr, "hw-test: ask needs the steps to open for the owner, as in hw-test ask camera. They are: "+strings.Join(guided.StepWords(), ", "))
		return 2
	}
	for _, word := range steps {
		if !guided.KnownStep(word) {
			fmt.Fprintf(os.Stderr, "hw-test: %q is no step. They are: %s\n", word, strings.Join(guided.StepWords(), ", "))
			return 2
		}
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		return 1
	}
	file, err := os.CreateTemp("", "hw-test-asked-*.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		return 1
	}
	answered := file.Name()
	file.Close()
	os.Remove(answered)
	defer os.Remove(answered)
	if err := tools.Terminal(self, append(steps, "--no-send", "--once", "--done", answered)...); err != nil {
		fmt.Fprintln(os.Stderr, "hw-test: the step couldn't be opened for the owner:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Opened for the owner in a window of its own: %s. Waiting until they are through, half an hour at most.\n", strings.Join(steps, ", "))
	for waited := time.Duration(0); waited < 30*time.Minute; waited += 500 * time.Millisecond {
		if data, err := os.ReadFile(answered); err == nil && len(data) > 0 {
			fmt.Print(string(data))
			return 0
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "hw-test: the owner didn't get to it. Nothing was measured.")
	return 1
}

// pickOne asks which of several parts an agent should look into, by its number.
func pickOne(in *bufio.Reader, out io.Writer, lines []string) (int, bool) {
	fmt.Fprintln(out, "Which one should your agent look into? It takes one part at a time, so a part that can't be fixed doesn't hold up the others.")
	for at, line := range lines {
		fmt.Fprintf(out, "  %d  %s\n", at+1, line)
	}
	fmt.Fprint(out, "Its number, or Enter for none\n> ")
	answer, _ := in.ReadString('\n')
	number, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || number < 1 || number > len(lines) {
		return 0, false
	}
	return number - 1, true
}

// agreed asks whether to send. Enter or anything but a no is a yes, and no answer at all is a no.
func agreed(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "Send it? [Y/n] ")
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && answer == "" {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer != "n" && answer != "no"
}

// writeNote sends a note, an answer or a solved mark, and returns the exit code.
func writeNote(run *report.Run, args noteArgs) int {
	fail := func(format string, values ...any) int {
		fmt.Fprintf(os.Stderr, "hw-test: "+format+"\n", values...)
		return 2
	}
	if args.solved != "" {
		if _, err := report.Solve(args.api, args.solved, true); err != nil {
			return fail("%v", err)
		}
		fmt.Fprintln(os.Stderr, "Marked as solved:", report.Site+"/note?id="+args.solved)
		return 0
	}
	text := args.body
	switch {
	case text != "":
	case args.bodyFile != "":
		data, err := os.ReadFile(args.bodyFile)
		if err != nil {
			return fail("%v", err)
		}
		text = string(data)
	case !guided.Interactive():
		data, _ := io.ReadAll(io.LimitReader(os.Stdin, 64<<10))
		text = string(data)
	}
	if strings.TrimSpace(text) == "" {
		return fail("a note needs its text: --body, --body-file, or on standard input. Say what happens, how to make it happen, and what was tried.")
	}
	if args.reply == "" && args.title == "" {
		return fail("a note needs --title, the note in a line")
	}
	host, account, names := owner()
	note := report.Note{Body: report.ScrubText(text, host, account, names...), Links: args.links, Kernel: run.Kernel, OS: run.OS, Packages: run.Packages, Agent: run.Agent}
	if args.reply == "" {
		note.Kind, note.Title, note.Check, note.Model = args.kind, args.title, args.check, run.Model
		if note.Model == "" {
			if matched, err := report.Match(args.api, run); err == nil {
				note.Model = matched.ID
			}
		}
		if note.Model == "" {
			return fail("the board doesn't know this computer, so there is no page for the note. Name the model with --model, or send a test first: hw-test quick")
		}
	}
	for _, name := range args.logs {
		log, err := attach(name)
		if err != nil {
			return fail("the log couldn't be read: %v", err)
		}
		note.Logs = append(note.Logs, log)
	}
	if !args.send {
		// Everything that would go, logs included, for the owner to read first
		data, _ := json.MarshalIndent(note, "", "  ")
		fmt.Println(string(data))
		fmt.Fprintln(os.Stderr, "Not sent.")
		return 0
	}
	if len(note.Logs) > 0 {
		// A log holds more than its owner remembers, so they are told what goes and where before it does
		fmt.Fprint(os.Stderr, attached(note.Logs))
		if guided.Interactive() && !agreed(terminal, os.Stderr) {
			fmt.Fprintln(os.Stderr, "Not sent.")
			return 0
		}
	}
	sent, err := report.SendNote(args.api, note, args.reply)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test: the note wasn't taken:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Written as %s.\n%s\n", sent.Note, sent.Page)
	return 0
}

// wrapWords lists words on lines that fit under the usage text's second column.
func wrapWords(words []string) string {
	lines, line := []string{}, ""
	for _, word := range words {
		if line != "" && len(line)+len(word)+2 > 58 {
			lines, line = append(lines, line+","), ""
		}
		if line != "" {
			line += ", "
		}
		line += word
	}
	return strings.Join(append(lines, line), "\n                     ")
}

func usage() { fmt.Fprint(os.Stderr, usageText()) }

// usageText is what hw-test help prints. The reference at arewearmyet.com/docs/hw-test is held
// to it by a test.
func usageText() string {
	return `hw-test checks what works on a Snapdragon PC and reports it to arewearmyet.com.

  hw-test            the guided test: the quick check, then keyboard, sound, camera and sleep with you
  hw-test quick      the quick check alone: read-only, a few seconds, no questions
  hw-test STEP...    one part alone, reporting only that part:
                     ` + wrapWords(guided.StepWords()) + `
  hw-test manual     every test done with you: keyboard, touchpad, touchscreen,
                     headphones, camera, USB ports, external screen, sleep and a few questions
  hw-test auto       every test that needs nobody: sound, graphics, WebGL and
                     WebGPU, Wi-Fi and Bluetooth, the network cable, video decoding,
                     temperatures. Without a terminal it runs all but the sound, which
                     plays tones out loud
  hw-test fix [WHAT...]    hand what doesn't work to your agent: writes a brief with what was
                     measured, where to look and where a fix goes, and opens Omarchy's
                     agent on it. WHAT is a check or a part, like speakers. Without it,
                     it asks which one: an agent takes one part at a time, from the
                     results already here, and nothing is tested again
  hw-test brief [WHAT...]  print that brief, to give to any agent yourself
  hw-test ask STEP...      for an agent: open a step that takes the owner in a window on
                     their desktop, wait until they are through, and print the result.
                     The sound test needs no window: hw-test speakers --play, on the
                     owner's yes. hw-test status --json gives every latest result as data.
                     hw-test ask update opens Omarchy's update for the owner the same way and
                     prints how it ended. It refuses, with the reason, where an update would
                     do harm, as while a test boot entry stands
  hw-test note       write down what fits no check, where other owners' agents read it and can
                     answer: a problem that takes a sequence of events, a way around one, a
                     pointer to work elsewhere. In a terminal, hw-test note asks for the
                     part, what happened and which log shows it, and hw-test note wifi
                     names the part at once. Everything can also be given, as agents do:
                       hw-test note --kind problem --check suspend --log previous \
                         --title "Restarts when woken with a dock plugged in" --body-file what-happens.md
                     --kind problem|workaround|upstream|note   --check ID   --link URL...
                     --log kernel|previous|boot-N|FILE...  attaches a log, cleaned of addresses,
                     serial numbers, disk IDs and names; boot-3 is the third boot before this
                     one. --json prints what would go and sends nothing
                     --reply NOTE-ID answers a note, --solved NOTE-ID closes your own problem
                     --no-send shows what would be sent, logs included, and sends nothing
  hw-test verify before [PART]   take a snapshot of where this computer stands, before a change.
                     Name the part the change is about, as in hw-test verify before speakers,
                     and it gets a snapshot of its own, apart from another part's. One that
                     is still in use isn't replaced unless --new says so
  hw-test verify after [PART] --pr URL   after testing again: what the change did, check by check, as a
                     block for the pull request, and recorded with the pull request on the
                     model's page. --title says in a line what the change is. --check ID,ID
                     gives those checks a row even when they work on both sides
  hw-test status     where this computer stands: the latest result of every check, nothing is run
  hw-test tree start|build|boot|status|patch|remove   try a changed device tree from a boot entry of
                     its own: the sources Omarchy's package builds from, with its patches, a
                     build, and (run by the owner, with their password) a test entry in the
                     boot menu. hw-test tree patch "arm64: dts: qcom: BOARD: WHAT" --body-file WHY
                     writes the change as the one patch the package takes
  hw-test kernel start|build|load|status|restore|patch   try a change in one kernel driver
                     without building a kernel: Omarchy's kernel source as the running
                     kernel was built from it, a build of one driver's folder (hw-test kernel
                     build drivers/phy/qualcomm), and (run by the owner, with their password)
                     the built module loaded in place of the kernel's own until the next
                     restart. hw-test kernel patch "SUBSYSTEM: WHAT" --body-file WHY writes
                     the change as a patch for the kernel's recipe
  hw-test report --check ID --state works|partial|broken|absent --note TEXT   send one result
                     that no test measured: what the owner saw (--how owner, the default)
                     or what was read from logs (--how agent). absent is for a part the
                     computer doesn't have
  hw-test resume [WHAT]    for an agent, before it restarts the computer: at the owner's next
                     login the agent opens again in this folder, once, and is told WHAT.
                     --show says what is left, --cancel takes it back
  hw-test update     get the newest release. hw-test says at its start when there is one
  hw-test facts      what the test found on this computer to work with (nothing is sent)
  hw-test version

Options:
  --no-send      show the results without sending them
  --json         print what would be sent, and send nothing
  --model ID     the model's id on arewearmyet.com, when the machine isn't recognised by itself
  --agent NAME   the agent running this, shown next to the run

Nothing personal is sent: no serial numbers, addresses, names or files. The microphone recording
and the camera picture of the guided test never leave this computer.

Every command with its options: https://arewearmyet.com/docs/hw-test
`
}
