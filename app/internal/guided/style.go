package guided

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// The terminal's own sixteen colours, so the test looks like the rest of the desktop's theme.
var (
	bold   = lipgloss.NewStyle().Bold(true)
	dim    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	good   = lipgloss.NewStyle().Foreground(lipgloss.Green)
	close_ = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	bad    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	accent = lipgloss.NewStyle().Foreground(lipgloss.Green).Bold(true)
	title  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightWhite)
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
	return dim
}

var stateLabel = map[string]string{report.Works: "works", report.Partial: "partly", report.Broken: "broken", report.Absent: "not on this model", report.Untested: "untested"}

// Names of the checks as the site shows them.
var checkTitle = map[string]string{
	"installer": "USB stick and installer", "disk-unlock": "Disk password prompt", "display": "Built-in screen", "gpu": "Graphics acceleration",
	"backlight": "Screen brightness", "keyboard": "Built-in keyboard", "touchpad": "Touchpad", "touchscreen": "Touchscreen and pen",
	"wifi": "Wi-Fi", "wifi-address": "Fixed Wi-Fi address", "bluetooth": "Bluetooth", "speakers": "Speakers", "microphones": "Built-in microphones",
	"headphones": "Headphone jack", "adsp": "Audio and battery processor", "battery": "Battery readings", "charging": "USB-C charging",
	"suspend": "Sleep and wake", "suspend-drain": "Battery use while asleep", "hibernate": "Hibernation", "lid": "Lid switch", "usb": "USB ports",
	"external-display": "External screen", "usb4": "USB4 and Thunderbolt", "camera": "Webcam", "video-decode": "Hardware video decoding",
	"cdsp": "Compute DSP (NPU)", "fans": "Fans and temperatures", "fingerprint": "Fingerprint reader", "modem": "Mobile broadband",
}

// Said wherever the microphones or the camera are used.
const staysHere = "It never leaves this computer: nothing recorded or photographed is sent anywhere, and it's deleted when the test ends."

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

// resultsTable lists results with their state, and the evidence where there is room.
func resultsTable(results []report.Result, width int) string {
	var out strings.Builder
	for i, result := range results {
		name := checkTitle[result.Check]
		if name == "" {
			name = result.Check
		}
		line := lipgloss.NewStyle().Width(30).Render(name) + stateStyle(result.State).Width(18).Render(stateLabel[result.State])
		detail := result.Note
		if detail == "" {
			detail = result.Evidence
		}
		if room := width - 50; room > 20 && detail != "" {
			if lipgloss.Width(detail) > room {
				detail = detail[:room-1] + "…"
			}
			line += dim.Render(detail)
		}
		out.WriteString(line)
		if i < len(results)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
