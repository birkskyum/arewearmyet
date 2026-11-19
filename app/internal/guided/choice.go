package guided

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// choice is a question with a short list of answers, picked with the arrow keys and Enter or
// with an answer's number.
type choice struct {
	question string
	options  []string
	// beside, when set, are a few words shown after each answer, already styled: a part's
	// status in the menu. notes are a line under it, for what takes a sentence.
	beside []string
	notes  []string
	// heads are lines set above the answer at a place in the list, to name a group of answers.
	// An empty one is a blank line.
	heads  map[int]string
	cursor int
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

// blocks draws each answer with what belongs to it: the group's line above it, the words
// beside it and the note under it.
func (c *choice) blocks(width int) []string {
	// The words beside the answers start in one column, where the window has room for that
	column, room := 0, 0
	for i, option := range c.options {
		column = max(column, lipgloss.Width(option))
		if i < len(c.beside) {
			room = max(room, lipgloss.Width(c.beside[i]))
		}
	}
	inline := room > 0 && column+2+room <= width-4
	var blocks []string
	for i, option := range c.options {
		var out strings.Builder
		if head, ok := c.heads[i]; ok {
			out.WriteString("\n")
			if head != "" {
				out.WriteString("  " + bold.Render(head) + "\n")
			}
		}
		marker, style := "  ", soft
		if i == c.cursor {
			marker, style = accent.Render("› "), bold
		}
		// Only the first nine answers have a number to press
		number := " "
		if i < 9 {
			number = fmt.Sprint(i + 1)
		}
		// A long answer wraps under its own first word, not under the number
		lines := strings.Split(wrap(option, width-4), "\n")
		status := ""
		if i < len(c.beside) {
			status = c.beside[i]
		}
		if inline && status != "" && len(lines) == 1 {
			fmt.Fprintf(&out, "%s%s %s%s%s\n", marker, soft.Render(number), style.Render(lines[0]), strings.Repeat(" ", column+2-lipgloss.Width(lines[0])), status)
			status = ""
		} else {
			fmt.Fprintf(&out, "%s%s %s\n", marker, soft.Render(number), style.Render(lines[0]))
		}
		for _, more := range lines[1:] {
			fmt.Fprintf(&out, "    %s\n", style.Render(more))
		}
		if status != "" {
			fmt.Fprintf(&out, "    %s\n", status)
		}
		if i < len(c.notes) && c.notes[i] != "" {
			fmt.Fprintf(&out, "    %s\n", c.notes[i])
		}
		blocks = append(blocks, strings.TrimRight(out.String(), "\n"))
	}
	return blocks
}

func (c *choice) view(width int) string { return c.viewIn(width, 0) }

// viewIn draws the question in at most the given rows: answers that don't fit scroll, with
// the one the cursor is on always in view. No limit when rows is 0.
func (c *choice) viewIn(width, rows int) string {
	question := bold.Render(wrap(c.question, width))
	blocks := c.blocks(width)
	height := func(from, to int) int {
		total := 0
		for _, block := range blocks[from:to] {
			total += strings.Count(block, "\n") + 1
		}
		return total
	}
	room := rows - strings.Count(question, "\n") - 2
	if rows <= 0 || height(0, len(blocks)) <= room {
		return question + "\n\n" + strings.TrimLeft(strings.Join(blocks, "\n"), "\n")
	}
	// A line each for "more above" and "more below"
	room = max(1, room-2)
	from := 0
	for from < c.cursor && height(from, c.cursor+1) > room {
		from++
	}
	to := c.cursor + 1
	for to < len(blocks) && height(from, to+1) <= room {
		to++
	}
	shown := []string{question, ""}
	if from > 0 {
		shown = append(shown, "  ↑ more")
	}
	shown = append(shown, strings.TrimLeft(strings.Join(blocks[from:to], "\n"), "\n"))
	if to < len(blocks) {
		shown = append(shown, "  ↓ more")
	}
	return strings.Join(shown, "\n")
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
