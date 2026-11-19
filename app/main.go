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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
		case command == "brief" || command == "fix" || command == "verify":
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
	case "quick", "check", "guided", "facts", "brief", "fix", "note", "verify":
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

	// Before and after a change: what a pull request changed on this machine, as measured
	if command == "verify" {
		os.Exit(verify(run, topics, *api, *pr, *title, !*noSend && !*jsonOnly))
	}

	// A note: what fits no check, written down where other owners' agents read it
	if command == "note" {
		os.Exit(writeNote(run, noteArgs{api: *api, kind: *kind, title: *title, check: *check, body: *body, bodyFile: *bodyFile, reply: *reply, solved: *solved,
			logs: logs, links: links, send: !*noSend && !*jsonOnly}))
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
		briefing := guided.Briefing{Version: version, Run: run, Kept: kept, Model: matched, Family: probe.Family(machine.Real()), Checks: known, Only: checks, When: time.Now()}
		if matched.ID != "" {
			briefing.Notes, _ = report.Notes(*api, matched.ID)
		}
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

// verify takes the snapshot a change is measured against, or measures the change against it.
func verify(run *report.Run, words []string, api, pr, what string, send bool) int {
	now := time.Now()
	step := ""
	if len(words) == 1 {
		step = words[0]
	}
	switch step {
	case "before":
		snapshot, err := report.TakeSnapshot(run, now)
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
		fmt.Fprintln(os.Stderr, "\nNow make the change. Then test the parts it touches again, like hw-test speakers or hw-test graphics,\nand measure the change against this snapshot:\n\n  hw-test verify after --pr <the pull request's address>")
		return 0
	case "after":
		snapshot, ok := report.LoadSnapshot()
		if !ok {
			fmt.Fprintln(os.Stderr, "hw-test: there is no snapshot to measure against. Take one before the change: hw-test verify before")
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
		fmt.Println(guided.VerifyBlock(run, model, snapshot, changes))
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
	host, _ := os.Hostname()
	user := os.Getenv("USER")
	var text string
	var err error
	logName := name + ".log"
	switch name {
	case "kernel":
		text, err = sys.RunFor(20*time.Second, "journalctl", "-k", "-b", "--no-hostname", "-o", "short-monotonic", "--no-pager")
	case "previous":
		text, err = sys.RunFor(20*time.Second, "journalctl", "-k", "-b", "-1", "--no-hostname", "-o", "short-monotonic", "--no-pager")
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
	clean := report.ScrubLog(text, host, user)
	if clean == "" {
		return report.Log{}, fmt.Errorf("%s is empty", name)
	}
	return report.Log{Name: logName, Text: clean}, nil
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
	host, _ := os.Hostname()
	note := report.Note{Body: report.ScrubLog(text, host, os.Getenv("USER")), Links: args.links, Kernel: run.Kernel, OS: run.OS, Packages: run.Packages, Agent: run.Agent}
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
  hw-test note       write down what fits no check, where other owners' agents read it and can
                     answer: a problem that takes a sequence of events, a way around one, a
                     pointer to work elsewhere. For example:
                       hw-test note --kind problem --check suspend --log previous \
                         --title "Restarts when woken with a dock plugged in" --body-file what-happens.md
                     --kind problem|workaround|upstream|note   --check ID   --link URL...
                     --log kernel|previous|FILE...  attaches a log, cleaned of addresses and names
                     --reply NOTE-ID answers a note, --solved NOTE-ID closes your own problem
                     --no-send shows what would be sent, logs included, and sends nothing
  hw-test verify before    take a snapshot of where this computer stands, before a change
  hw-test verify after --pr URL   after testing again: what the change did, check by check, as a
                     block for the pull request, and recorded with the pull request on the
                     model's page. --title says in a line what the change is
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
