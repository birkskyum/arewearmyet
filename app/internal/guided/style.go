package guided

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// The terminal's own colours, so the test looks like the rest of the desktop's theme. Only its
// normal text colour is used for anything that has to be read. A theme's "bright black" can be
// nearly its background, as it is on a blue theme, so secondary text is told apart by not being
// bold or coloured, never by being fainter.
var (
	bold   = lipgloss.NewStyle().Bold(true)
	soft   = lipgloss.NewStyle()
	good   = lipgloss.NewStyle().Foreground(lipgloss.Green)
	close_ = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	bad    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	accent = lipgloss.NewStyle().Foreground(lipgloss.Green).Bold(true)
	title  = lipgloss.NewStyle().Bold(true)
)

// stateStyle is the colour a check's state is shown in.
func stateStyle(state string) lipgloss.Style {
	switch state {
	case report.Works:
		return good
	case report.Partial:
		return close_
	case report.Broken:
		return bad
	}
	return soft
}

var stateLabel = map[string]string{report.Works: "works", report.Partial: "partly", report.Broken: "broken", report.Absent: "not on this model", report.Untested: "untested"}

// Names of the checks as the site shows them.
var checkTitle = map[string]string{
	"installer": "USB stick and installer", "disk-unlock": "Disk password prompt", "display": "Built-in screen", "gpu": "Graphics acceleration",
	"webgl": "WebGL in the browser", "webgpu": "WebGPU in the browser",
	"backlight": "Screen brightness", "keyboard": "Built-in keyboard", "touchpad": "Touchpad", "touchscreen": "Touchscreen and pen",
	"wifi": "Wi-Fi", "wifi-address": "Fixed Wi-Fi address", "bluetooth": "Bluetooth", "speakers": "Speakers", "microphones": "Built-in microphones",
	"headphones": "Headphone jack", "adsp": "Audio and battery processor", "battery": "Battery readings", "charging": "USB-C charging",
	"suspend": "Sleep and wake", "suspend-drain": "Battery use while asleep", "hibernate": "Hibernation", "lid": "Lid switch", "usb": "USB ports",
	"external-display": "External screen", "usb4": "USB4 and Thunderbolt", "ethernet": "Ethernet", "camera": "Webcam", "video-decode": "Hardware video decoding",
	"cdsp": "Compute DSP (NPU)", "fans": "Fans and temperatures", "fingerprint": "Fingerprint reader", "modem": "Mobile broadband",
}

// Said wherever the microphones or the camera are used.
const staysHere = "It never leaves this computer. Nothing recorded or photographed is sent anywhere, and it's deleted when the test ends."

// private is the line that says a recording or picture stays here, set apart so it's seen.
func private(width int) string {
	return good.Render("🔒 ") + wrap(staysHere, width-3)
}

// wrap breaks text into lines of at most width cells, at spaces.
func wrap(text string, width int) string {
	if width < 20 {
		width = 20
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			// A word longer than a line goes on in the next, since nothing shown is cut off
			for lipgloss.Width(word) > width {
				letters := []rune(word)
				room := width
				if line != "" {
					room = width - lipgloss.Width(line) - 1
				}
				if room < 8 {
					lines = append(lines, line)
					line = ""
					continue
				}
				room = min(room, len(letters))
				if line != "" {
					line += " "
				}
				lines = append(lines, line+string(letters[:room]))
				line, word = "", string(letters[room:])
			}
			if word == "" {
				continue
			}
			if line != "" && lipgloss.Width(line)+1+lipgloss.Width(word) > width {
				lines = append(lines, line)
				line = word
				continue
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// resultsTable lists results with their state and what was seen. The columns are as wide as
// what is in them, and text that doesn't fit its column goes on in the next line. Nothing is cut.
func resultsTable(results []report.Result, width int) string {
	width = max(width, 20)
	nameWidth, stateWidth := 0, 0
	for _, result := range results {
		nameWidth = max(nameWidth, lipgloss.Width(first(checkTitle[result.Check], result.Check))+2)
		stateWidth = max(stateWidth, lipgloss.Width(stateLabel[result.State])+2)
	}
	// In a narrow window the name's column gives way first
	nameWidth = max(10, min(nameWidth, width-stateWidth))
	// What was seen stands beside the state where there is room for a readable column, and under it otherwise
	detailWidth := width - nameWidth - stateWidth
	beside := detailWidth >= 24
	var out []string
	for _, result := range results {
		names := strings.Split(wrap(first(checkTitle[result.Check], result.Check), nameWidth-2), "\n")
		detail := first(result.Note, result.Evidence)
		var details []string
		if detail != "" && beside {
			details = strings.Split(wrap(detail, detailWidth), "\n")
		}
		for i := 0; i < max(len(names), len(details)); i++ {
			name, state, seen := "", "", ""
			if i < len(names) {
				name = names[i]
			}
			if i == 0 {
				state = stateLabel[result.State]
			}
			if i < len(details) {
				seen = details[i]
			}
			line := lipgloss.NewStyle().Width(nameWidth).Render(name) + stateStyle(result.State).Width(stateWidth).Render(state) + soft.Render(seen)
			out = append(out, strings.TrimRight(line, " "))
		}
		if detail != "" && !beside {
			for _, line := range strings.Split(wrap(detail, width-2), "\n") {
				out = append(out, "  "+soft.Render(line))
			}
		}
	}
	return strings.Join(out, "\n")
}
