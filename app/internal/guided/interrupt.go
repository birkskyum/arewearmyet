package guided

import (
	"encoding/binary"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
)

// interruptLine finds the interrupt of an input device that the device tree describes, as
// its folder under /sys/kernel/irq, or "" when it can't be found. A touchscreen or touchpad
// on I2C signals every touch on that line, and anyone may read how often it fired, where the
// touches themselves can only be read by the desktop.
func interruptLine(sys machine.System, inputDevice string) string {
	// Up from the input device to the hardware device the tree describes
	at := sys.Resolve(inputDevice + "/device")
	for at != "" && at != "/" && !sys.Exists(at+"/of_node") {
		at = filepath.Dir(at)
	}
	if at == "" || at == "/" {
		return ""
	}
	// Its pin on the interrupt controller: after the controller's handle in
	// interrupts-extended, or first in interrupts
	pin := int64(-1)
	if cells := sys.Bytes(at + "/of_node/interrupts-extended"); len(cells) >= 8 {
		pin = int64(binary.BigEndian.Uint32(cells[4:8]))
	} else if cells := sys.Bytes(at + "/of_node/interrupts"); len(cells) >= 4 {
		pin = int64(binary.BigEndian.Uint32(cells[0:4]))
	}
	if pin < 0 {
		return ""
	}
	// Several controllers can have a pin of that number; the one in use by an input driver is it
	fallback := ""
	for _, line := range sys.Glob("/sys/kernel/irq/*") {
		if hwirq, ok := sys.Int(line + "/hwirq"); !ok || hwirq != pin {
			continue
		}
		actions := sys.Read(line + "/actions")
		if strings.Contains(actions, "hid") || strings.Contains(actions, "touch") {
			return line
		}
		if actions != "" && fallback == "" {
			fallback = line
		}
	}
	return fallback
}

// interrupts is how often a line has fired, over all processors.
func interrupts(sys machine.System, line string) (int64, bool) {
	text := sys.Read(line + "/per_cpu_count")
	if text == "" {
		return 0, false
	}
	var total int64
	for _, part := range strings.Split(text, ",") {
		count, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return 0, false
		}
		total += count
	}
	return total, true
}

// Describe says how the guided test will check things on this machine: which hardware it
// found and what it can measure it by. It is what `hw-test facts` prints.
func Describe(sys machine.System, touchscreenDevice string) []string {
	var lines []string
	line := ""
	if touchscreenDevice != "" {
		line = interruptLine(sys, touchscreenDevice)
	}
	if line != "" {
		count, _ := interrupts(sys, line)
		lines = append(lines, "touchscreen interrupt "+line+", fired "+strconv.FormatInt(count, 10)+" times")
	} else if touchscreenDevice != "" {
		lines = append(lines, "touchscreen interrupt not found, taps are counted as clicks in the terminal")
	}
	return lines
}
