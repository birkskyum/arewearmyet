package guided

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// key is one key in the picture of the keyboard.
type key struct {
	label string
	code  rune
	width int
	// needed keys are the ones every keyboard has and the test waits for.
	needed bool
}

func letters(row string) []key {
	keys := make([]key, 0, len(row))
	for _, c := range row {
		keys = append(keys, key{label: strings.ToUpper(string(c)), code: c, width: 3, needed: unicode.IsLetter(c) || unicode.IsDigit(c)})
	}
	return keys
}

// The keyboard as a US laptop has it. Keys light up by their place, not by what a layout
// prints on them, so a Danish or Swiss keyboard lights the same picture.
var layout = [][]key{
	append(append([]key{{"Esc", tea.KeyEscape, 5, true}},
		key{"F1", tea.KeyF1, 3, false}, key{"F2", tea.KeyF2, 3, false}, key{"F3", tea.KeyF3, 3, false}, key{"F4", tea.KeyF4, 3, false},
		key{"F5", tea.KeyF5, 3, false}, key{"F6", tea.KeyF6, 3, false}, key{"F7", tea.KeyF7, 3, false}, key{"F8", tea.KeyF8, 3, false},
		key{"F9", tea.KeyF9, 3, false}, key{"F10", tea.KeyF10, 3, false}, key{"F11", tea.KeyF11, 3, false}, key{"F12", tea.KeyF12, 3, false}),
		key{"Del", tea.KeyDelete, 5, false}),
	append(letters("`1234567890-="), key{"Backspace", tea.KeyBackspace, 11, true}),
	append(append([]key{{"Tab", tea.KeyTab, 5, true}}, letters("qwertyuiop[]")...), key{`\`, '\\', 5, false}),
	append(append([]key{{"Caps", tea.KeyCapsLock, 6, false}}, letters("asdfghjkl;'")...), key{"Enter", tea.KeyEnter, 8, true}),
	append(append([]key{{"Shift", tea.KeyLeftShift, 8, false}}, letters("zxcvbnm,./")...), key{"Shift", tea.KeyRightShift, 10, false}),
	{{"Ctrl", tea.KeyLeftCtrl, 6, false}, {"Super", tea.KeyLeftSuper, 7, false}, {"Alt", tea.KeyLeftAlt, 5, false}, {"Space", tea.KeySpace, 21, true},
		{"Alt", tea.KeyRightAlt, 5, false}, {"←", tea.KeyLeft, 3, true}, {"↑", tea.KeyUp, 3, true}, {"↓", tea.KeyDown, 3, true}, {"→", tea.KeyRight, 3, true}},
}

// keyboard lights each key as it is pressed, and is over when every letter, number and arrow
// has been seen, or when the owner moves on without them.
type keyboard struct {
	run *report.Run
	lit map[rune]bool
	// other counts keys that aren't in the picture, like the extra key of an ISO keyboard.
	other map[rune]bool
	// recent holds the last presses, to notice a key pressed three times to move on.
	recent []rune
	// missing asks about the keys that never lit, once the owner moves on without them.
	missing *choice
	over    bool
}

func newKeyboard(run *report.Run) *keyboard {
	return &keyboard{run: run, lit: map[rune]bool{}, other: map[rune]bool{}}
}

func (k *keyboard) name() string       { return "Keyboard" }
func (k *keyboard) start() tea.Cmd     { return nil }
func (k *keyboard) done() bool         { return k.over }
func (k *keyboard) wantsAllKeys() bool { return k.missing == nil }

func (k *keyboard) keys() string {
	if k.missing != nil {
		return choiceKeys
	}
	if len(k.unlit()) == 0 {
		return "Enter to go on"
	}
	return "Press each key once · press Enter, Space or Esc three times in a row to go on without the rest"
}

// unlit lists the needed keys that haven't been pressed.
func (k *keyboard) unlit() []string {
	var names []string
	for _, row := range layout {
		for _, key := range row {
			if key.needed && !k.lit[key.code] {
				names = append(names, key.label)
			}
		}
	}
	return names
}

func (k *keyboard) count() int { return len(k.lit) + len(k.other) }

func (k *keyboard) update(msg tea.Msg) tea.Cmd {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if k.missing != nil {
		switch k.missing.update(press) {
		case 0:
			k.run.Set("keyboard", report.Works, report.Owner, fmt.Sprintf("%d keys registered; the owner didn't press %s", k.count(), strings.Join(k.unlit(), ", ")), "")
			k.over = true
		case 1:
			k.run.Set("keyboard", report.Partial, report.Owner, fmt.Sprintf("%d keys registered", k.count()), "No response from: "+strings.Join(k.unlit(), ", "))
			k.over = true
		case 2:
			k.missing = nil
			k.recent = nil
		}
		return nil
	}

	complete := len(k.unlit()) == 0
	// The key by its place on a US keyboard when the terminal says, else by what it types
	code := press.BaseCode
	if code == 0 {
		code = press.Code
	}
	code = unicode.ToLower(code)
	if complete && code == tea.KeyEnter {
		k.run.Set("keyboard", report.Works, report.Measured, fmt.Sprintf("%d keys registered, every letter, number and arrow among them", k.count()), "")
		k.over = true
		return nil
	}
	known := false
	for _, row := range layout {
		for _, key := range row {
			known = known || key.code == code
		}
	}
	if known {
		k.lit[code] = true
	} else {
		k.other[code] = true
	}

	k.recent = append(k.recent, code)
	if n := len(k.recent); n >= 3 && !complete {
		a, b, c := k.recent[n-3], k.recent[n-2], k.recent[n-1]
		if a == b && b == c && (c == tea.KeyEnter || c == tea.KeySpace || c == tea.KeyEscape) && len(k.unlit()) > 0 {
			k.missing = &choice{
				question: "These keys never lit up: " + strings.Join(k.unlit(), " ") + ". What's true?",
				options:  []string{"The keyboard is fine", "I pressed them and they don't respond", "Go back and press them"},
			}
		}
	}
	return nil
}

func (k *keyboard) view(width, height int) string {
	if k.missing != nil {
		return k.missing.view(width)
	}
	var rows []string
	for _, row := range layout {
		var cells []string
		for _, key := range row {
			style := lipgloss.NewStyle().Width(key.width).Align(lipgloss.Center)
			// Three looks that read on any theme: a pressed key is black on green, a key still
			// to press is the text colour reversed like a key cap, an optional key is plain text
			switch {
			case k.lit[key.code]:
				style = style.Background(lipgloss.Green).Foreground(lipgloss.Black).Bold(true)
			case key.needed:
				style = style.Reverse(true)
			}
			cells = append(cells, style.Render(key.label))
		}
		rows = append(rows, strings.Join(cells, " "))
	}
	left := len(k.unlit())
	status := fmt.Sprintf("%d keys seen, %d to go.", k.count(), left)
	if left == 0 {
		status = good.Render(fmt.Sprintf("Every letter, number and arrow works (%d keys seen).", k.count())) + " Press Enter to go on."
	}
	intro := "Press every letter, number and arrow key once, plus Space, Enter, Backspace, Tab and Esc. Each lights up green when the computer sees it. " +
		"The keys without a background are optional: some keyboards don't have them, and the desktop keeps some for itself."
	return wrap(intro, width) + "\n\n" + strings.Join(rows, "\n\n") + "\n\n" + status
}
