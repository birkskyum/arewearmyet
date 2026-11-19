package guided

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/report"
	"github.com/birkskyum/arewearmyet/app/internal/tools"
)

type sentMsg struct {
	sent report.Sent
	err  error
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
	// failed holds the steps that hit a fault of the test's own, by name.
	failed map[string]string
	// dir holds the test's recordings and pictures, which are deleted before anything is sent.
	dir string
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
	s.ask = choice{question: "Send the lines above to arewearmyet.com?", options: []string{"Send", "Don't send"}}
	if !s.options.Send {
		s.ask = choice{question: "These are the results. Nothing is sent, as asked.", options: []string{"Done"}}
	}
	return nil
}

func (s *summary) send() tea.Cmd {
	s.phase = "sending"
	run, api := s.run, s.options.API
	return func() tea.Msg {
		sent, err := report.Send(api, run)
		return sentMsg{sent, err}
	}
}

// page is where the run shows: the model's page, or for a machine the board doesn't know,
// the form that adds it.
func (s *summary) page() string {
	if s.sent.Page != "" {
		return s.sent.Page
	}
	return report.AddModelURL(s.run, s.sent.Run)
}

func (s *summary) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case sentMsg:
		s.sent, s.err = msg.sent, msg.err
		if s.err != nil {
			s.phase = "failed"
			s.ask = choice{question: "Sending failed: " + s.err.Error(), options: []string{"Try again", "Give up"}}
			return nil
		}
		s.phase = "sent"
		question := "Sent as " + s.sent.Run + ". Thank you! It's on the model's page now."
		open := "Open the page in the browser"
		if s.sent.Page == "" {
			question = "Sent as " + s.sent.Run + ". The board doesn't know this computer yet, so the run shows on no page. A form with its name and hardware IDs filled in adds it."
			open = "Open the form in the browser"
		}
		s.ask = choice{question: question, options: []string{open, "Done"}}
	case tea.KeyPressMsg:
		picked := s.ask.update(msg)
		if picked < 0 {
			return nil
		}
		switch s.phase {
		case "review":
			if picked == 0 && s.options.Send {
				return s.send()
			}
			s.skipped, s.over = true, true
		case "failed":
			if picked == 0 {
				return s.send()
			}
			s.over = true
		case "sent":
			if picked == 0 {
				tools.Open(s.page())
			}
			s.over = true
		}
	}
	return nil
}

func (s *summary) view(width, height int) string {
	if s.phase == "sending" {
		return accent.Render("● Sending to arewearmyet.com")
	}
	text := ""
	if s.phase == "review" {
		text = resultsTable(s.run.Results, width) + "\n\n"
		// Said plainly, so a step that was passed over isn't taken for one that found nothing
		for name, fault := range s.failed {
			text += close_.Render(wrap("The "+name+" step ran into a problem of the test's own and was passed over ("+fault+"). Nothing is reported for it.", width)) + "\n\n"
		}
		if s.options.Send {
			// What goes and what doesn't, said where the decision is made
			text += wrap("Those lines are everything that is sent, with this computer's model and its software versions.", width) + "\n" +
				good.Render("🔒 ") + wrap("No picture and no recording is sent. They were only for you to check, and are already deleted.", width-3) + "\n\n"
		}
	}
	return text + s.ask.view(width)
}

// farewell is what's left in the terminal after the test closes.
func (s *summary) farewell() string {
	switch {
	case s.err != nil:
		return "Sending failed: " + s.err.Error() + "\nRun hw-test again to retry."
	case s.skipped || s.sent.Run == "":
		return "Not sent."
	}
	return fmt.Sprintf("Sent as %s. Thank you!\n%s", s.sent.Run, s.page())
}
