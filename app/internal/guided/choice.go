package guided

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// choice is a question with a short list of answers, picked with the arrow keys and Enter or
// with an answer's number.
type choice struct {
	question string
	options  []string
	cursor   int
}

// update moves the cursor, and returns the picked answer's place in the list, or -1.
func (c *choice) update(msg tea.KeyPressMsg) int {
	switch key := msg.String(); key {
	case "up", "k", "shift+tab":
		c.cursor = (c.cursor + len(c.options) - 1) % len(c.options)
	case "down", "j", "tab":
		c.cursor = (c.cursor + 1) % len(c.options)
	case "enter", "space":
		return c.cursor
	default:
		if len(key) == 1 && key[0] >= '1' && int(key[0]-'1') < len(c.options) {
			c.cursor = int(key[0] - '1')
			return c.cursor
		}
	}
	return -1
}

func (c *choice) view(width int) string {
	var out strings.Builder
	out.WriteString(bold.Render(wrap(c.question, width)))
	out.WriteString("\n\n")
	for i, option := range c.options {
		marker, style := "  ", dim
		if i == c.cursor {
			marker, style = accent.Render("› "), bold
		}
		fmt.Fprintf(&out, "%s%s %s\n", marker, dim.Render(fmt.Sprint(i+1)), style.Render(option))
	}
	return strings.TrimRight(out.String(), "\n")
}

const choiceKeys = "↑ ↓ to move · Enter to choose · or press its number"

// line is one line of typed text, for saying what's wrong.
type line struct {
	prompt string
	text   []rune
}

// update takes a key, and says whether Enter ended the line.
func (l *line) update(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "enter":
		return true
	case "backspace":
		if len(l.text) > 0 {
			l.text = l.text[:len(l.text)-1]
		}
	default:
		if msg.Text != "" && len(l.text) < 300 {
			l.text = append(l.text, []rune(msg.Text)...)
		}
	}
	return false
}

func (l *line) view(width int) string {
	return bold.Render(wrap(l.prompt, width)) + "\n\n" + accent.Render("› ") + string(l.text) + "█"
}

func (l *line) value() string { return strings.TrimSpace(string(l.text)) }
