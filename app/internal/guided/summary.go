package guided

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

type sentMsg struct {
	sent report.Sent
	err  error
}

// slowMsg comes a few seconds into a send that hasn't finished, and retryMsg when it is time
// to try again after the network wasn't there.
type (
	slowMsg  struct{}
	retryMsg struct{}
)

// networkWait is how long sending keeps trying while the network is away. After the lid or
// sleep test, Wi-Fi takes some seconds to come back.
const networkWait = time.Minute

// offline says whether sending failed for want of a network, not because the site said no.
func offline(err error) bool {
	var network net.Error
	return errors.As(err, &network)
}

// summary shows every result, sends them and offers to open the model's page.
type summary struct {
	run     *report.Run
	options Options
	phase   string // review, sending, sent, failed
	ask     choice
	sent    report.Sent
	err     error
	skipped bool
	over    bool
	// again is set when the owner chose to go back to the start after this round.
	again bool
	// began is when this try at sending started, and waiting is set once it is taking long
	// or the network was found to be away.
	began   time.Time
	waiting bool
	// failed holds the steps that hit a fault of the test's own, by name.
	failed map[string]string
	// dir holds the test's recordings and pictures, which are deleted before anything is sent.
	dir string
	// recorded is set when the round had a step that uses the microphones or the camera.
	recorded bool
	// handed is how handing what failed to an agent went.
	handed string
	// asked is the last screen's question, put back after a part was picked for the agent.
	asked string
	// once ends the test here without a screen. The steps were opened for the owner by an agent, which
	// gets their result.
	once bool
}

// Report draws everything that would be sent in one frame, which is the computer's model and
// software, the results, and what else the run names. Nothing outside the frame leaves the computer.
func Report(run *report.Run, width int) string {
	framed := width >= 56
	inner := width
	if framed {
		inner = width - 4
	}
	// Plain text broken into lines that fit, with a word longer than a line cut short
	fit := func(text string) []string {
		lines := strings.Split(wrap(text, inner), "\n")
		for i, line := range lines {
			if letters := []rune(line); len(letters) > inner {
				lines[i] = string(letters[:inner-1]) + "…"
			}
		}
		return lines
	}
	var body []string
	name := strings.TrimSpace(run.Hardware["maker"] + " " + first(run.Hardware["family"], run.Hardware["product"]))
	if bios := run.Hardware["bios"]; bios != "" {
		name = strings.TrimPrefix(name+", BIOS "+bios, ", ")
	}
	if name != "" {
		body = append(body, fit(name)...)
	}
	software := "Kernel " + run.Kernel
	if count := len(run.Packages); count > 0 {
		software += fmt.Sprintf(", %d package versions", count)
	}
	if count := len(run.Chids); count > 0 {
		software += fmt.Sprintf(", %d hardware IDs", count)
	}
	body = append(body, fit(software)...)
	body = append(body, "")
	body = append(body, strings.Split(resultsTable(run.Results, inner), "\n")...)
	if line := SetupLine(run.Setup); line != "" {
		body = append(body, "")
		body = append(body, fit(line)...)
	}
	if !framed {
		// A narrow window has no room for a frame, so the report is set apart by its heading alone
		return title.Render("Report") + "\n" + strings.Join(body, "\n")
	}
	var out strings.Builder
	out.WriteString("╭─ " + title.Render("Report") + " " + strings.Repeat("─", inner-7) + "╮\n")
	for _, line := range body {
		out.WriteString("│ " + line + strings.Repeat(" ", max(0, inner-lipgloss.Width(line))) + " │\n")
	}
	out.WriteString("╰" + strings.Repeat("─", inner+2) + "╯")
	return out.String()
}

// SetupLine says in one line what goes with the results about the computer itself, or nothing
// on a computer that runs Omarchy's own kernel with Omarchy's own options.
func SetupLine(setup *report.Setup) string {
	lines := setup.Lines()
	if len(lines) == 0 {
		return ""
	}
	return "Also sent: " + strings.Join(lines, ", ") + "."
}

// failing lists the checks this round found partly working or broken, or working with something
// worth a second look, which is what an agent can be asked to look into.
func (s *summary) failing() []string {
	var failing []string
	for _, result := range s.run.Results {
		if severity[result.State] > 1 || worthALook(result) {
			failing = append(failing, result.Check)
		}
	}
	return failing
}

// agentOption is the last screen's offer to hand a part to the owner's agent. An agent takes one
// part at a time, so a part that can't be fixed doesn't hold up the others. With several to choose
// from, the owner picks one, and the offer is there again for the next.
func (s *summary) agentOption() string {
	switch failing := s.failing(); len(failing) {
	case 0:
		return ""
	case 1:
		return "Let an agent look into the " + strings.ToLower(first(checkTitle[failing[0]], failing[0]))
	}
	return "Let an agent look into one of them"
}

// hand gives one check to the agent, and leaves the last choice up for what comes after.
func (s *summary) hand(check string) tea.Cmd {
	s.handed = "Writing the brief and opening your agent…"
	return handOver(s.run, s.options, report.Model{ID: s.sent.Model, Page: s.sent.Page}, []string{check})
}

func (s *summary) name() string { return "Send" }
func (s *summary) done() bool   { return s.over }

func (s *summary) keys() string {
	if s.phase == "sending" {
		return "Sending…"
	}
	return choiceKeys
}

func (s *summary) start() tea.Cmd {
	s.phase = "review"
	// Every recording and picture is gone before the question of sending comes up
	if s.dir != "" {
		entries, _ := os.ReadDir(s.dir)
		for _, entry := range entries {
			os.RemoveAll(filepath.Join(s.dir, entry.Name()))
		}
	}
	if s.once {
		s.skipped, s.over = true, true
		return nil
	}
	s.ask = choice{question: "Send this report to arewearmyet.com?", options: []string{"Send report", "Don't send"}}
	if !s.options.Send {
		s.end("These are the results. Nothing is sent, as asked.")
	}
	return nil
}

// end puts up the last choice of a round, which is back to the start, where the cursor rests, the
// page when something was sent, or leaving.
func (s *summary) end(question string) {
	s.phase, s.asked = "end", question
	options := []string{"Back to the start"}
	if s.sent.Run != "" {
		open := "Open this computer's page in the browser"
		if s.sent.Page == "" {
			open = "Open the form in the browser"
		}
		options = append(options, open)
	}
	if offer := s.agentOption(); offer != "" {
		options = append(options, offer)
	}
	s.ask = choice{question: question, options: append(options, "Quit")}
}

func (s *summary) send() tea.Cmd {
	s.phase, s.began, s.waiting = "sending", time.Now(), false
	return tea.Batch(s.attempt(), tea.Tick(3*time.Second, func(time.Time) tea.Msg { return slowMsg{} }))
}

func (s *summary) attempt() tea.Cmd {
	run, api := s.run, s.options.API
	return func() tea.Msg {
		sent, err := report.Send(api, run)
		return sentMsg{sent, err}
	}
}

// page is where the run shows, which is the model's page, or for a machine the board doesn't know,
// the form that adds it.
func (s *summary) page() string {
	if s.sent.Page != "" {
		return s.sent.Page
	}
	return report.AddModelURL(s.run, s.sent.Run)
}

func (s *summary) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case handedMsg:
		s.handed = msg.handed.Said
	case slowMsg:
		if s.phase == "sending" {
			s.waiting = true
		}
	case retryMsg:
		if s.phase == "sending" {
			return s.attempt()
		}
	case sentMsg:
		// Without a network it keeps trying for a while and says so, as Wi-Fi comes back by itself
		if s.phase == "sending" && offline(msg.err) && time.Since(s.began) < networkWait {
			s.waiting = true
			return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return retryMsg{} })
		}
		s.sent, s.err = msg.sent, msg.err
		if s.err != nil {
			s.phase = "failed"
			s.ask = choice{question: "Sending failed (" + s.err.Error() + ").", options: []string{"Try again", "Give up"}}
			return nil
		}
		question := "Report sent as " + s.sent.Run + ". Thank you! It's on the model's page now."
		if s.sent.Page == "" {
			question = "Report sent as " + s.sent.Run + ". The board doesn't know this computer yet, so the run shows on no page. A form with its name and hardware IDs filled in adds it."
		}
		s.end(question)
	case tea.KeyPressMsg:
		picked := s.ask.update(msg)
		if picked < 0 {
			return nil
		}
		switch s.phase {
		case "review":
			if picked == 0 {
				return s.send()
			}
			s.skipped = true
			s.end("Not sent. The results are kept on this computer.")
		case "failed":
			if picked == 0 {
				return s.send()
			}
			s.end("Not sent. The results are kept on this computer.")
		case "pick":
			failing, asked := s.failing(), s.asked
			s.end(asked)
			if picked < len(failing) {
				return s.hand(failing[picked])
			}
		case "end":
			switch s.ask.options[picked] {
			case "Back to the start":
				s.again, s.over = true, true
			case "Quit":
				s.over = true
			case s.agentOption():
				// The choice stays up, as the agent works in a window of its own
				failing := s.failing()
				if len(failing) == 1 {
					return s.hand(failing[0])
				}
				s.phase = "pick"
				var lines []string
				for _, check := range failing {
					result, _ := s.run.Get(check)
					lines = append(lines, summed(check, result))
				}
				s.ask = choice{question: "Which one? An agent takes one part at a time, so a part that can't be fixed doesn't hold up the others. You can come back here for the next.", options: append(lines, "Back")}
			default:
				// The page opens in the browser, and the choice stays up for what comes after
				tools.Open(s.page())
			}
		}
	}
	return nil
}

func (s *summary) view(width, height int) string {
	if s.phase == "sending" {
		if s.waiting {
			return accent.Render("● Sending to arewearmyet.com") + "\n\n" +
				wrap("Waiting for the network. After sleep or a closed lid, Wi-Fi takes a few seconds to come back. It keeps trying for a minute.", width)
		}
		return accent.Render("● Sending to arewearmyet.com")
	}
	text := ""
	// The results stay in view until they have been sent
	if s.sent.Run == "" {
		// Where the question is whether to send, everything that would go is in one frame, the report
		if s.options.Send {
			text = Report(s.run, width) + "\n\n"
		} else {
			text = resultsTable(s.run.Results, width) + "\n\n"
		}
		// Said plainly, so a step that was passed over isn't taken for one that found nothing
		for name, fault := range s.failed {
			text += close_.Render(wrap("The "+name+" step ran into a problem of the test's own and was passed over ("+fault+"). Nothing is reported for it.", width)) + "\n\n"
		}
		if s.options.Send && s.phase == "review" {
			// What goes and what doesn't, said where the decision is made
			text += wrap("The report is everything that is sent.", width) + "\n"
			// Said only after a round that made a picture or a recording
			if s.recorded {
				text += good.Render("🔒 ") + wrap("No picture and no recording is sent. They were only for you to check, and are already deleted.", width-3) + "\n"
			}
			text += "\n"
		}
	}
	text += s.ask.view(width)
	if s.handed != "" {
		text += "\n\n" + accent.Render(wrap(s.handed, width))
	}
	return text
}

// farewell is what's left in the terminal after the test closes.
func (s *summary) farewell() string {
	switch {
	case s.once:
		return ""
	case s.err != nil:
		return "Sending failed (" + s.err.Error() + ").\nRun hw-test again to retry."
	case s.skipped || s.sent.Run == "":
		return "Not sent."
	}
	return fmt.Sprintf("Report sent as %s. Thank you!\n%s", s.sent.Run, s.page())
}
