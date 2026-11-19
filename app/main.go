// hw-test checks what works on a Snapdragon PC under Omarchy and reports it to arewearmyet.com.
//
//	hw-test            the guided test: the quick check, then keyboard, sound, camera and sleep with you
//	hw-test quick      the quick check alone: read-only, a few seconds, no questions
//	hw-test sound      one part of the guided test alone, reporting only that part
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

	"github.com/birkskyum/arewearmyet/app/internal/guided"
	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
	"github.com/birkskyum/arewearmyet/app/internal/report"
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
	var only []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		word := args[0]
		args = args[1:]
		switch {
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
	case "quick", "check", "guided":
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
	if len(only) > 0 && !guided.Interactive() {
		fmt.Fprintln(os.Stderr, "hw-test: a step is done with you at the keyboard, so it needs a terminal")
		os.Exit(2)
	}

	run, facts, err := probe.Quick(machine.Real())
	if err != nil {
		fmt.Fprintln(os.Stderr, "hw-test:", err)
		os.Exit(1)
	}
	run.Model, run.Agent = *model, *agent

	if *jsonOnly {
		fmt.Println(string(run.JSON()))
		return
	}
	// The guided test needs someone at the keyboard; without a terminal it is the quick check
	if command == "guided" && guided.Interactive() {
		// The quick results are kept aside: if the guided test can't run here, they are still sent
		measured := append([]report.Result(nil), run.Results...)
		err := guided.Run(run, facts, guided.Options{API: *api, Send: !*noSend, Only: only})
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

func usage() {
	fmt.Fprint(os.Stderr, `hw-test checks what works on a Snapdragon PC and reports it to arewearmyet.com.

  hw-test            the guided test: the quick check, then keyboard, sound, camera and sleep with you
  hw-test quick      the quick check alone: read-only, a few seconds, no questions
  hw-test STEP...    one part of the guided test alone, reporting only that part:
                     `+strings.Join(guided.StepWords(), ", ")+`
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
