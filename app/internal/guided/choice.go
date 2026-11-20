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
	// sides, when set, turn the list into a menu. Each answer has a mark in front of it and a few
	// words in a column after it, and no number to press. A menu is picked from with the arrow keys
	// alone.
	sides []side
	// heads are lines set above the answer at a place in the list, to name a group of answers.
	// An empty one is a blank line.
	heads  map[int]string
	cursor int
}

// side is what a menu shows around one answer, a mark in front and a few words after it, both in
// the given style.
type side struct {
	mark, text string
	style      lipgloss.Style
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
		if len(c.sides) == 0 && len(key) == 1 && key[0] >= '1' && int(key[0]-'1') < len(c.options) {
			c.cursor = int(key[0] - '1')
			return c.cursor
		}
	}
	return -1
}

// blocks draws each answer with what belongs to it, which is the group's line above it, and in a
// menu its mark and the words after it.
func (c *choice) blocks(width int) []string {
	if len(c.sides) > 0 {
		return c.menu(width)
	}
	var blocks []string
	for i, option := range c.options {
		marker, style := "  ", soft
		if i == c.cursor {
			marker, style = accent.Render("› "), bold
		}
		// A long answer wraps under its own first word, not under the number
		lines := strings.Split(wrap(option, width-4), "\n")
		block := fmt.Sprintf("%s%s %s", marker, soft.Render(fmt.Sprint(i+1)), style.Render(lines[0]))
		for _, more := range lines[1:] {
			block += "\n    " + style.Render(more)
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// menu draws the answers as a menu, with the cursor, the mark, the answer, and in one column after
// the longest answer the words that go with each, cut to what the window has room for.
func (c *choice) menu(width int) []string {
	column := 0
	for _, option := range c.options {
		column = max(column, lipgloss.Width(option))
	}
	// Cursor and mark take two cells each, and three more keep the words clear of the answers
	room := width - (4 + column + 3)
	var blocks []string
	for i, option := range c.options {
		block := ""
		if head, ok := c.heads[i]; ok {
			block = "\n"
			if head != "" {
				block += "  " + bold.Render(head) + "\n"
			}
		}
		marker, style := "  ", soft
		if i == c.cursor {
			marker, style = accent.Render("› "), bold
		}
		var around side
		if i < len(c.sides) {
			around = c.sides[i]
		}
		mark := "  "
		if around.mark != "" {
			mark = around.style.Render(around.mark) + " "
		}
		lines := strings.Split(wrap(option, width-4), "\n")
		block += marker + mark + style.Render(lines[0])
		if text := []rune(around.text); len(text) > 0 && room >= 12 && len(lines) == 1 {
			if len(text) > room {
				text = append(text[:room-1], '…')
			}
			block += strings.Repeat(" ", column+3-lipgloss.Width(lines[0])) + around.style.Render(string(text))
		}
		for _, more := range lines[1:] {
			block += "\n    " + style.Render(more)
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func (c *choice) view(width int) string { return c.viewIn(width, 0) }

// viewIn draws the question in at most the given rows. Answers that don't fit scroll, with the one
// the cursor is on always in view. No limit when rows is 0.
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

// menuKeys is the hint under a menu, which has no numbers.
const menuKeys = "↑ ↓ to move · Enter to choose"

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
