package guided

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// A USB device, not one of its interfaces (1-1:1.0) and not a root hub (usb1): 1-1, 3-1.2
var usbDevice = regexp.MustCompile(`^\d+-[\d.]+$`)

// plug is one device seen appearing on a port.
type plug struct {
	port  string // bus and port, like 3-1
	speed int64  // Mb/s
	// same names the device (vendor:product), only to tell here whether two plugs were the same
	// stick. It is never sent.
	same string
}

type usbMsg struct{ present map[string]bool }

// usb watches for devices being plugged in, so each port is tested by plugging something
// into it, and the speed it ran at is read off instead of asked about.
type usb struct {
	run     *report.Run
	sys     machine.System
	present map[string]bool
	plugs   []plug
	// dead counts times the owner plugged something in and nothing appeared.
	dead int
	over bool
}

func newUSB(run *report.Run, sys machine.System) *usb {
	return &usb{run: run, sys: sys}
}

func (u *usb) name() string { return "USB ports" }
func (u *usb) done() bool   { return u.over }
func (u *usb) keys() string {
	if len(u.plugs)+u.dead == 0 {
		return "Plug a USB stick or any USB device into a port · n if you plug one in and nothing shows here · s to skip"
	}
	return "Next port, or the same USB-C plug the other way round · n if nothing shows · Enter when that was every port"
}

func (u *usb) devices() map[string]bool {
	present := map[string]bool{}
	for _, device := range u.sys.Glob("/sys/bus/usb/devices/*") {
		if usbDevice.MatchString(filepath.Base(device)) {
			present[filepath.Base(device)] = true
		}
	}
	return present
}

func (u *usb) poll() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return usbMsg{u.devices()} })
}

func (u *usb) start() tea.Cmd {
	u.present = u.devices()
	return u.poll()
}

func (u *usb) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case usbMsg:
		if u.over {
			return nil
		}
		for device := range msg.present {
			// A hub brings its own devices along; the hub itself is the plug
			if u.present[device] || strings.Contains(device, ".") && msg.present[device[:strings.LastIndex(device, ".")]] && !u.present[device[:strings.LastIndex(device, ".")]] {
				continue
			}
			dir := "/sys/bus/usb/devices/" + device
			speed, _ := u.sys.Int(dir + "/speed")
			u.plugs = append(u.plugs, plug{port: device, speed: speed, same: u.sys.Read(dir+"/idVendor") + ":" + u.sys.Read(dir+"/idProduct")})
		}
		u.present = msg.present
		return u.poll()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "n":
			u.dead++
		case "s":
			u.over = true
		case "enter":
			u.finish()
		}
	}
	return nil
}

func speedName(speed int64) string {
	switch {
	case speed >= 1000:
		return fmt.Sprintf("%g Gb/s", float64(speed)/1000)
	case speed > 0:
		return fmt.Sprintf("%d Mb/s", speed)
	}
	return "an unknown speed"
}

// slowed says whether the same device ran at SuperSpeed in one plug and slower in another,
// which is what a USB-C port wired for one orientation only does.
func (u *usb) slowed() bool {
	fastest := map[string]int64{}
	for _, p := range u.plugs {
		fastest[p.same] = max(fastest[p.same], p.speed)
	}
	for _, p := range u.plugs {
		if fastest[p.same] >= 5000 && p.speed < 5000 {
			return true
		}
	}
	return false
}

func (u *usb) finish() {
	u.over = true
	if len(u.plugs)+u.dead == 0 {
		return
	}
	var seen []string
	for _, p := range u.plugs {
		seen = append(seen, p.port+" at "+speedName(p.speed))
	}
	sort.Strings(seen)
	evidence := fmt.Sprintf("%d plug-ins detected", len(u.plugs))
	if len(seen) > 0 {
		evidence += ": " + strings.Join(seen, ", ")
	}
	switch {
	case len(u.plugs) == 0:
		u.run.Set("usb", report.Broken, report.Owner, evidence, fmt.Sprintf("Plugged in %d times, and no device ever appeared.", u.dead))
	case u.dead > 0:
		u.run.Set("usb", report.Partial, report.Owner, evidence, fmt.Sprintf("%d plug-ins gave no device.", u.dead))
	case u.slowed():
		u.run.Set("usb", report.Partial, report.Measured, evidence, "The same device ran at full speed in one plug-in and only at USB 2 speed in another, as a USB-C port does when only one orientation is wired for SuperSpeed.")
	default:
		u.run.Set("usb", report.Works, report.Measured, evidence, "")
	}
}

func (u *usb) view(width, height int) string {
	lines := []string{wrap("Now the USB ports. Plug a USB stick, or any USB device, into each port in turn. It shows here when the computer sees it, with the speed it runs at. For a USB-C port, try the plug both ways round.", width), ""}
	for i, p := range u.plugs {
		lines = append(lines, good.Render(fmt.Sprintf("  ✓ Plug-in %d: port %s at %s", i+1, p.port, speedName(p.speed))))
	}
	for i := 0; i < u.dead; i++ {
		lines = append(lines, bad.Render("  ✗ A plug-in gave no device"))
	}
	if len(u.plugs)+u.dead == 0 {
		lines = append(lines, "  ○ Waiting for a device…")
	}
	if u.slowed() {
		lines = append(lines, "", close_.Render(wrap("The same device ran slower in one plug-in than in another.", width)))
	}
	return strings.Join(lines, "\n")
}
