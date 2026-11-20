package guided

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

type screenMsg struct{ connected map[string]bool }

// screen waits for an external screen to be plugged in and reads what kind of connection it
// is and the picture it got, so the owner only says whether there is a picture.
type screen struct {
	run   *report.Run
	sys   machine.System
	phase string // waiting, picture, replug-out, replug-in
	// before is what was connected when the step began.
	before    map[string]bool
	connector string
	details   string
	replugged string
	since     time.Time
	// only is set when the screen under test is the only one the computer has: a mini PC's
	// monitor, which nobody is asked to unplug.
	only bool
	ask  choice
	over bool
}

func newScreen(run *report.Run, sys machine.System) *screen {
	return &screen{run: run, sys: sys, phase: "waiting"}
}

func (s *screen) name() string { return "External screen" }
func (s *screen) done() bool   { return s.over }

func (s *screen) keys() string {
	switch s.phase {
	case "waiting":
		return "Plug in an external screen · n if you plugged one in and nothing shows here · s if you have none at hand"
	case "picture":
		return choiceKeys
	case "replug-out":
		return "Unplug the screen now · s to skip this part"
	}
	return "Plug it back in · n if it doesn't come back · s to skip this part"
}

// external lists the connectors for screens other than the built-in one, and which have a screen.
func (s *screen) external() map[string]bool {
	connected := map[string]bool{}
	for _, pattern := range []string{"/sys/class/drm/card*-DP-*", "/sys/class/drm/card*-HDMI-*"} {
		for _, path := range s.sys.Glob(pattern) {
			connected[connector(path)] = s.sys.Read(path+"/status") == "connected"
		}
	}
	return connected
}

func (s *screen) poll() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return screenMsg{s.external()} })
}

// builtIn says whether the computer has a screen of its own that is on, as a laptop does.
func (s *screen) builtIn() bool {
	for _, pattern := range []string{"/sys/class/drm/card*-eDP-*", "/sys/class/drm/card*-DSI-*", "/sys/class/drm/card*-LVDS-*"} {
		for _, path := range s.sys.Glob(pattern) {
			if s.sys.Read(path+"/status") == "connected" {
				return true
			}
		}
	}
	return false
}

func (s *screen) start() tea.Cmd {
	s.before = s.external()
	plugged := 0
	for _, connected := range s.before {
		if connected {
			plugged++
		}
	}
	// On a computer without a screen of its own, the one monitor it has is what this is read on
	s.only = plugged == 1 && !s.builtIn()
	// A screen that is already plugged in is the one to test
	for name, connected := range s.before {
		if connected {
			s.found(name)
		}
	}
	return s.poll()
}

// describe reads how the screen is connected and the picture it got.
func (s *screen) describe(name string) string {
	how := "DisplayPort"
	switch {
	case strings.HasPrefix(name, "HDMI"):
		how = "HDMI"
	case len(s.sys.Glob("/sys/bus/thunderbolt/devices/[0-9]-[0-9]*")) > 0:
		how = "DisplayPort through USB4"
	case len(s.sys.Glob("/sys/class/typec/port*-partner")) > 0:
		how = "DisplayPort over USB-C"
	}
	mode := ""
	// Hyprland knows the mode in use; without it, the first mode the screen offers
	var monitors []struct {
		Name        string  `json:"name"`
		Width       int     `json:"width"`
		Height      int     `json:"height"`
		RefreshRate float64 `json:"refreshRate"`
	}
	if json.Unmarshal([]byte(s.sys.Run("hyprctl", "monitors", "-j")), &monitors) == nil {
		for _, monitor := range monitors {
			if monitor.Name == name {
				mode = fmt.Sprintf("%dx%d at %.0f Hz", monitor.Width, monitor.Height, monitor.RefreshRate)
			}
		}
	}
	if mode == "" {
		for _, path := range s.sys.Glob("/sys/class/drm/card*-" + name) {
			if first := s.sys.Read(path + "/modes"); first != "" {
				mode = first + " offered"
			}
		}
	}
	if mode == "" {
		return name + " (" + how + "), no mode read"
	}
	return name + " (" + how + "), " + mode
}

func (s *screen) found(name string) {
	s.connector, s.details, s.phase = name, s.describe(name), "picture"
	s.ask = choice{question: "The computer sees a screen on " + s.details + ". Does it show a picture?",
		options: []string{"Yes, a steady picture", "Yes, but it flickers or has the wrong size", "No, it stays black"}}
	if s.only {
		s.ask = choice{question: "This computer's screen is the one on " + s.details + ". Is the picture steady and the right size?",
			options: []string{"Yes, a steady picture", "No, it flickers or has the wrong size"}}
	}
}

func (s *screen) evidence() string {
	if s.replugged != "" {
		return s.details + ", and " + s.replugged
	}
	return s.details
}

func (s *screen) finish(state, how, note string) {
	s.run.Set("external-display", state, how, s.evidence(), note)
	s.over = true
}

func (s *screen) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case screenMsg:
		if s.over {
			return nil
		}
		switch s.phase {
		case "waiting":
			for name, connected := range msg.connected {
				if connected && !s.before[name] {
					// The mode settles a moment after the plug
					s.found(name)
				}
			}
		case "replug-out":
			if !msg.connected[s.connector] {
				s.phase, s.since = "replug-in", time.Now()
			}
		case "replug-in":
			if msg.connected[s.connector] {
				s.replugged = fmt.Sprintf("it came back %.0f s after plugging it in again", time.Since(s.since).Seconds())
				s.finish(report.Works, report.Owner, "")
			}
		}
		if s.over {
			return nil
		}
		return s.poll()
	case tea.KeyPressMsg:
		switch s.phase {
		case "waiting":
			switch msg.String() {
			case "n":
				s.details = "no screen detected on any connector"
				s.finish(report.Broken, report.Owner, "A screen was plugged in and the computer didn't see it.")
			case "s":
				s.over = true
			}
		case "picture":
			// Refresh what's known, now that the mode has settled
			s.details = s.describe(s.connector)
			switch s.ask.update(msg) {
			case 0:
				if s.only {
					// Unplugging it would leave nothing to read the test on
					s.finish(report.Works, report.Owner, "")
					return nil
				}
				s.phase = "replug-out"
			case 1:
				s.finish(report.Partial, report.Owner, "There is a picture, but it flickers or has the wrong size.")
			case 2:
				s.finish(report.Broken, report.Owner, "The computer sees the screen, but it stays black.")
			}
		case "replug-out":
			if msg.String() == "s" {
				s.finish(report.Works, report.Owner, "")
			}
		case "replug-in":
			switch msg.String() {
			case "n":
				s.replugged = "it didn't come back after plugging it in again"
				s.finish(report.Partial, report.Owner, "The screen works when plugged in at first, but not after unplugging and plugging it back in.")
			case "s":
				s.finish(report.Works, report.Owner, "")
			}
		}
	}
	return nil
}

func (s *screen) view(width, height int) string {
	switch s.phase {
	case "waiting":
		var names []string
		for name := range s.before {
			names = append(names, name)
		}
		text := "Now an external screen. Plug one in, through USB-C, a dock or HDMI. It shows here when the computer sees it, with the kind of connection and the picture it got."
		if len(names) == 0 {
			text += "\n\n" + close_.Render("This computer shows no connector for an external screen at all.")
		}
		return wrap(text, width) + "\n\n  ○ Waiting for a screen…"
	case "picture":
		return s.ask.view(width)
	case "replug-out":
		return good.Render("✓ "+s.details) + "\n\n" + wrap("For the last part, unplug the screen and plug it back in when this says so. It checks that the screen comes back.", width)
	}
	return good.Render("✓ Unplugged.") + " " + accent.Render("Now plug it back in.")
}

// connector is a display connector's name without its card, like DP-1 for card1-DP-1.
func connector(path string) string {
	_, name, _ := strings.Cut(filepath.Base(path), "-")
	return name
}
