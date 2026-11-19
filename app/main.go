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
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/deep"
	"github.com/birkskyum/arewearmyet/app/internal/guided"
	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

// version is set when a release is built.
var version = "dev"

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
		case command == "brief" || command == "fix":
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
	noSend := flags.Bool("no-send", false, "show the results without sending them")
	model := flags.String("model", "", "the model's id on arewearmyet.com, when the machine isn't recognised by itself")
	agent := flags.String("agent", "", "the agent running this, shown next to the run")
	api := flags.String("api", report.Site, "another server, for testing")
	flags.Usage = usage
	flags.Parse(args)

	switch command {
	case "version", "--version":
		fmt.Println("hw-test", version)
		return
	case "help", "--help", "-h":
		usage()
		return
	case "status":
		// The latest result of every check on this machine, without running anything
		fmt.Println(guided.Status(report.Local()))
		return
	case "quick", "check", "guided", "facts", "brief", "fix":
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
	if unattended && !guided.Automatic(only) {
		fmt.Fprintln(os.Stderr, "hw-test: that step is done with you at the keyboard, so it needs a terminal. Without one, these run: "+strings.Join(guided.AutomaticWords(), ", "))
		os.Exit(2)
	}

	run, facts, err := probe.Quick(machine.Real())
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		os.Exit(1)
	}
	run.Model, run.Agent = *model, *agent

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
		briefing := guided.Briefing{Version: version, Run: run, Kept: kept, Model: matched, Checks: known, Only: checks, When: time.Now()}
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
		guided.Unattended(run, deep.Tests(machine.Real(), facts.Wifi, facts.Bluetooth, dir), only, func(line string) { fmt.Fprintln(os.Stderr, line) })
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
		err := guided.Run(run, facts, guided.Options{API: *api, Send: !*noSend, Only: only, Version: version})
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

func usage() {
	fmt.Fprint(os.Stderr, `hw-test checks what works on a Snapdragon PC and reports it to arewearmyet.com.

  hw-test            the guided test: the quick check, then keyboard, sound, camera and sleep with you
  hw-test quick      the quick check alone: read-only, a few seconds, no questions
  hw-test STEP...    one part alone, reporting only that part:
                     `+wrapWords(guided.StepWords())+`
  hw-test manual     every test done with you, about ten minutes: keyboard, touchpad, touchscreen,
                     headphones, camera, USB ports, external screen, sleep and a few questions
  hw-test auto       every test that needs nobody, about four minutes: sound, graphics, WebGL and
                     WebGPU, Wi-Fi and Bluetooth, video decoding, temperatures. Without a
                     terminal it runs all but the sound, which plays tones out loud
  hw-test fix [WHAT...]    hand what doesn't work to your agent: writes a brief with what was
                     measured, where to look and where a fix goes, and opens Omarchy's
                     agent on it. WHAT is a check or a part, like speakers. Without it,
                     everything that fails
  hw-test brief [WHAT...]  print that brief, to give to any agent yourself
  hw-test status     where this computer stands: the latest result of every check, nothing is run
  hw-test facts      what the test found on this computer to work with (nothing is sent)
  hw-test version

Options:
  --no-send      show the results without sending them
  --json         print what would be sent, and send nothing
  --model ID     the model's id on arewearmyet.com, when the machine isn't recognised by itself
  --agent NAME   the agent running this, shown next to the run

Nothing personal is sent: no serial numbers, addresses, names or files. The microphone recording
and the camera picture of the guided test never leave this computer.
`)
}
