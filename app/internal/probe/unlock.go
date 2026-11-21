package probe

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// The disk password prompt runs from the early boot image, before most drivers load. Whether the
// built-in screen and keyboard were up by then is in the kernel's log of this boot, which keeps the
// kernel's own time for each line. A screen that came up before the encrypted disk opened showed
// the prompt, and one that came up after it left the prompt dark. The same goes for the keyboard.
var (
	// Any console on a screen, the display driver's or the firmware's
	screenUp = regexp.MustCompile(`\bfb\d+: .*frame buffer device`)
	// The first use of the opened disk, by the file systems an encrypted root is made with
	diskOpen = regexp.MustCompile(`^(BTRFS info \(device dm-\d+\): first mount of filesystem|EXT4-fs \(dm-\d+\): mounted filesystem|XFS \(dm-\d+\): Mounting|F2FS-fs \(dm-\d+\): Mounted)`)
)

// bootTimes reads the kernel's log of this boot as seconds since the kernel started and the line.
func bootTimes(sys machine.System, each func(at float64, line string)) {
	log := sys.Run("journalctl", "-k", "-b", "--no-pager", "-q", "-o", "json", "--output-fields=MESSAGE,_SOURCE_MONOTONIC_TIMESTAMP")
	for _, entry := range strings.Split(log, "\n") {
		var line struct {
			Message any    `json:"MESSAGE"`
			At      string `json:"_SOURCE_MONOTONIC_TIMESTAMP"`
		}
		if json.Unmarshal([]byte(entry), &line) != nil {
			continue
		}
		text, ok := line.Message.(string)
		micros, err := strconv.ParseFloat(line.At, 64)
		if !ok || err != nil {
			continue
		}
		each(micros/1e6, text)
	}
}

// unlock measures the disk password prompt of a laptop with an encrypted disk. It says nothing
// where the log doesn't show when the screen came up or when the disk opened.
func unlock(sys machine.System, facts *Facts, measured set) {
	if !facts.Encrypted || !facts.Laptop {
		return
	}
	const unseen = -1.0
	screen, keyboard, opened := unseen, unseen, unseen
	// The keyboard's line names it as the system does, at times with the kind of device after the name
	typing := "input: " + facts.Keyboard
	bootTimes(sys, func(at float64, line string) {
		switch {
		case screen == unseen && screenUp.MatchString(line):
			screen = at
		case keyboard == unseen && facts.Keyboard != "" && strings.HasPrefix(line, typing) && strings.Contains(line, " as /"):
			keyboard = at
		case opened == unseen && diskOpen.MatchString(line):
			opened = at
		}
	})
	if screen == unseen || opened == unseen {
		return
	}
	lit, typed := screen < opened, keyboard == unseen || keyboard < opened
	when := fmt.Sprintf("the screen came up at %.1f s", screen)
	if keyboard != unseen {
		when += fmt.Sprintf(" and the built-in keyboard at %.1f s", keyboard)
	}
	when += fmt.Sprintf(", and the encrypted disk opened at %.1f s", opened)
	switch {
	case lit && typed:
		measured("disk-unlock", report.Works, when)
	case lit:
		measured("disk-unlock", report.Partial, when+", so the prompt showed and the built-in keyboard wasn't ready for it")
	case typed:
		measured("disk-unlock", report.Partial, when+", so the prompt was dark and the password could be typed blind")
	default:
		measured("disk-unlock", report.Broken, when+", so the prompt was dark and the built-in keyboard wasn't ready for it")
	}
}
