// Package probe is the quick check: what can be measured on a Snapdragon PC without asking
// anyone and without changing anything. It only reads files and runs read-only commands.
//
// A check gets a result here only when the machine itself shows it. What takes eyes or ears
// (do the speakers play, does the picture look right) is left to the guided test.
package probe

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// Facts are what the quick check learned about the hardware that the guided test builds on:
// which devices exist, whatever state they are in.
type Facts struct {
	Laptop      bool
	Keyboard    string // the built-in keyboard's device name
	Touchpad    string
	Touchscreen string
	// TouchscreenDevice is the touchscreen's input device under /sys, to find its interrupt by.
	TouchscreenDevice string
	Lid               string
	Backlight         string // its folder under /sys/class/backlight
	Wifi              string // the interface, like wlan0
	Bluetooth         string // the controller, like hci0
	SoundCard         string
	Camera            string
	Fingerprint       string // USB vendor:product of a known reader
	Encrypted         bool   // the root disk asks for a password at startup
}

var packages = []string{"linux-omarchy", "linux-firmware", "linux-firmware-qcom", "mesa", "systemd", "omarchy", "hyprland", "pipewire", "alsa-ucm-conf", "bluez", "libcamera"}

// Desktops, mini PCs and all-in-ones, by SMBIOS chassis type: no battery, lid or built-in keyboard to expect.
var desktops = map[string]bool{"3": true, "4": true, "5": true, "6": true, "7": true, "13": true, "15": true, "16": true, "35": true, "36": true}

var (
	chidLine = regexp.MustCompile(`^\s*(\S+)\s+\S+\s+([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\s*$`)
	sensor   = regexp.MustCompile(`^[a-z0-9]+ [0-9]+-[0-9a-f]{4}$`)
	selected = regexp.MustCompile(`\[([^\]]*)\]`)
)

// Quick measures the machine. It fails only when the machine isn't an ARM one.
func Quick(sys machine.System) (*report.Run, Facts, error) {
	arch := strings.TrimSpace(sys.Run("uname", "-m"))
	if arch != "aarch64" && arch != "arm64" {
		return nil, Facts{}, fmt.Errorf("this is a %s machine, and the check is for ARM PCs", arch)
	}

	// Without /sys there is nothing to read, and every device would look missing: that is a
	// container or another system, not a computer with broken hardware
	if !sys.IsDir("/sys/class/input") && !sys.IsDir("/sys/class/drm") {
		return nil, Facts{}, fmt.Errorf("this system's /sys can't be read, so there is nothing to check (is this a container?)")
	}

	run := &report.Run{Mode: "quick", Hardware: map[string]string{}, Packages: map[string]string{}, Chids: []string{}}
	dmi := "/sys/class/dmi/id/"
	for key, file := range map[string]string{"maker": "sys_vendor", "product": "product_name", "family": "product_family", "sku": "product_sku", "board": "board_name", "bios": "bios_version"} {
		if value := sys.Read(dmi + file); value != "" {
			run.Hardware[key] = value
		}
	}
	run.Compatible = sys.Read("/proc/device-tree/compatible")
	run.Kernel = strings.TrimSpace(sys.Run("uname", "-r"))
	run.OS = osName(sys)
	for _, name := range packages {
		if fields := strings.Fields(sys.Run("pacman", "-Q", name)); len(fields) == 2 {
			run.Packages[name] = fields[1]
		}
	}
	// The machine's hardware IDs, most specific first, which name its model exactly
	for _, line := range strings.Split(sys.Run("systemd-analyze", "chid"), "\n") {
		if m := chidLine.FindStringSubmatch(line); m != nil {
			run.Chids = append(run.Chids, m[1]+":"+m[2])
		}
	}

	facts := Facts{Laptop: !desktops[sys.Read(dmi+"chassis_type")]}
	facts.Encrypted = strings.Contains(sys.ReadAll("/proc/cmdline"), "cryptdevice=") || strings.Contains(sys.ReadAll("/proc/cmdline"), "rd.luks")
	measured := func(check, state, evidence string) { run.Set(check, state, report.Measured, evidence, "") }
	klog := sys.Run("journalctl", "-k", "-b", "--no-pager", "-q", "-o", "cat")

	// Each part on its own: these machines have half-working drivers and odd values in /sys,
	// and one part tripping over them mustn't cost the others their results
	for _, part := range []func(){
		func() { screen(sys, measured) },
		func() { gpu(sys, klog, measured) },
		func() { input(sys, &facts, measured) },
		func() { wireless(sys, &facts, measured) },
		func() { processors(sys, measured) },
		func() { sound(sys, &facts, measured) },
		func() { power(sys, &facts, measured) },
		func() { sleep(sys, measured) },
		func() { ports(sys, measured) },
		func() { media(sys, &facts, measured) },
		func() { sensors(sys, &facts, measured) },
	} {
		safely(part)
	}
	return run, facts, nil
}

// safely runs one part of the check and lets nothing it does stop the rest.
func safely(part func()) {
	defer func() { _ = recover() }()
	part()
}

type set func(check, state, evidence string)

func osName(sys machine.System) string {
	name := "Linux"
	for _, line := range strings.Split(sys.ReadAll("/etc/os-release"), "\n") {
		key, value, _ := strings.Cut(line, "=")
		value = strings.Trim(value, `"`)
		if key == "PRETTY_NAME" {
			return value
		}
		if key == "NAME" {
			name = value
		}
	}
	return name
}

// connector is a display connector's name without its card, like eDP-1.
func connector(path string) string {
	_, name, _ := strings.Cut(filepath.Base(path), "-")
	return name
}

func screen(sys machine.System, measured set) {
	panels := append(sys.Glob("/sys/class/drm/card*-eDP-*"), sys.Glob("/sys/class/drm/card*-DSI-*")...)
	for _, panel := range panels {
		status := sys.Read(panel + "/status")
		if status == "" {
			continue
		}
		if status == "connected" {
			mode := sys.Read(panel + "/modes")
			if mode == "" {
				mode = "no mode listed"
			}
			measured("display", report.Works, connector(panel)+" connected, "+mode)
		} else {
			measured("display", report.Broken, connector(panel)+" "+status)
		}
		return
	}
}

// The adreno driver bound to a GPU, or the msm display driver on its own.
func gpu(sys machine.System, klog string, measured set) {
	device := ""
	for _, entry := range sys.Glob("/sys/bus/platform/drivers/adreno/*") {
		if strings.Contains(filepath.Base(entry), "gpu") {
			device = filepath.Base(entry)
		}
	}
	renderer := ""
	for _, node := range sys.Glob("/sys/class/drm/renderD*") {
		if name := sys.LinkName(node + "/device/driver"); name != "" {
			renderer = name
		}
	}
	software := false
	for _, line := range strings.Split(sys.ReadAll("/etc/environment"), "\n") {
		software = software || strings.HasPrefix(line, "LIBGL_ALWAYS_SOFTWARE=1")
	}
	switch {
	case device != "":
		for _, line := range strings.Split(klog, "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "zap") && (strings.Contains(lower, "fail") || strings.Contains(lower, "error")) {
				measured("gpu", report.Broken, "GPU "+device+" bound, but: "+line)
				return
			}
		}
		if software {
			measured("gpu", report.Partial, "GPU "+device+" bound, but /etc/environment forces software rendering")
			return
		}
		if renderer == "" {
			renderer = "?"
		}
		measured("gpu", report.Works, "adreno bound to "+device+", render node from "+renderer)
	case renderer == "msm" || renderer == "msm_dpu" || strings.Contains(klog, "no GPU device was found"):
		evidence := "msm drives the display, but no GPU is bound (the device tree has it off or its firmware failed)"
		if software {
			evidence += "; software rendering is on"
		}
		measured("gpu", report.Broken, evidence)
	}
}

// Input devices, by udev's view of them. One that's missing on a laptop is broken; one
// that's there still needs a person to try it.
func input(sys machine.System, facts *Facts, measured set) {
	for _, backlight := range sys.Glob("/sys/class/backlight/*") {
		if sys.Exists(backlight + "/max_brightness") {
			facts.Backlight = backlight
		}
	}
	for _, event := range sys.Glob("/sys/class/input/event*") {
		name := sys.Read(event + "/device/name")
		if name == "" {
			continue
		}
		properties := sys.Run("udevadm", "info", "-q", "property", "-p", event)
		has := func(line string) bool { return strings.Contains("\n"+properties+"\n", "\n"+line+"\n") }
		if has("ID_BUS=usb") || has("ID_BUS=bluetooth") {
			continue
		}
		lower := strings.ToLower(name)
		button := false
		for _, word := range []string{"gpio", "power", "pmic", "video bus", "headset", "jack"} {
			button = button || strings.Contains(lower, word)
		}
		if !button && has("ID_INPUT_KEYBOARD=1") {
			facts.Keyboard = name
		}
		if has("ID_INPUT_TOUCHPAD=1") {
			facts.Touchpad = name
		}
		if has("ID_INPUT_TOUCHSCREEN=1") {
			facts.Touchscreen, facts.TouchscreenDevice = name, event
		}
		// Bit 0 of the switch capabilities is the lid
		words := strings.Fields(sys.Read(event + "/device/capabilities/sw"))
		if len(words) > 0 {
			if bits, err := strconv.ParseUint(words[len(words)-1], 16, 64); err == nil && bits&1 == 1 {
				facts.Lid = name
			}
		}
	}
	if !facts.Laptop {
		return
	}
	if facts.Backlight == "" {
		measured("backlight", report.Broken, "no backlight device")
	}
	if facts.Keyboard == "" {
		measured("keyboard", report.Broken, "no built-in keyboard device")
	}
	if facts.Touchpad == "" {
		measured("touchpad", report.Broken, "no built-in touchpad device")
	}
	if facts.Lid == "" {
		measured("lid", report.Broken, "no input device reports a lid switch")
	}
}

func wireless(sys machine.System, facts *Facts, measured set) {
	for _, net := range sys.Glob("/sys/class/net/*") {
		if sys.IsDir(net+"/wireless") || sys.Exists(net+"/phy80211") {
			facts.Wifi = filepath.Base(net)
		}
	}
	if facts.Wifi == "" {
		measured("wifi", report.Broken, "no Wi-Fi interface")
	} else {
		net := "/sys/class/net/" + facts.Wifi
		card := facts.Wifi + " (" + sys.LinkName(net+"/device/driver") + ")"
		if sys.Read(net+"/operstate") == "up" {
			measured("wifi", report.Works, card+" connected")
		}
		switch sys.Read(net + "/addr_assign_type") {
		case "0":
			measured("wifi-address", report.Works, card+": permanent address")
		case "1":
			measured("wifi-address", report.Broken, card+": random address made by the kernel, so it changes every boot")
		case "3":
			measured("wifi-address", report.Works, card+": address set by the system (not from the card)")
		}
	}

	for _, controller := range sys.Glob("/sys/class/bluetooth/hci*") {
		facts.Bluetooth = filepath.Base(controller)
	}
	if facts.Bluetooth == "" {
		measured("bluetooth", report.Broken, "no Bluetooth controller")
		return
	}
	for _, line := range strings.Split(sys.Run("bluetoothctl", "show"), "\n") {
		switch strings.TrimSpace(line) {
		case "Powered: yes":
			measured("bluetooth", report.Works, facts.Bluetooth+" powered")
			return
		case "Powered: no":
			// Switched off by the owner, as in airplane mode, says nothing about the hardware
			if !switchedOff(sys, "bluetooth") {
				measured("bluetooth", report.Partial, facts.Bluetooth+" present, but it doesn't power on")
			}
			return
		}
	}
}

// switchedOff says whether a radio is turned off in software, which the owner or airplane
// mode does.
func switchedOff(sys machine.System, kind string) bool {
	for _, radio := range sys.Glob("/sys/class/rfkill/rfkill*") {
		if sys.Read(radio+"/type") == kind && sys.Read(radio+"/soft") == "1" {
			return true
		}
	}
	return false
}

// The remote processors: the audio DSP, which sound, the battery and USB-C power need, and
// the compute DSP.
func processors(sys machine.System, measured set) {
	for _, processor := range sys.Glob("/sys/class/remoteproc/remoteproc*") {
		name := sys.Read(processor + "/name")
		check := ""
		switch {
		case strings.Contains(name, "adsp"):
			check = "adsp"
		case strings.Contains(name, "cdsp"):
			check = "cdsp"
		default:
			continue
		}
		firmware := sys.Read(processor + "/firmware")
		with := func(prefix string) string {
			if firmware == "" {
				return ""
			}
			return " (" + prefix + firmware + ")"
		}
		switch sys.Read(processor + "/state") {
		case "running":
			measured(check, report.Works, name+" running"+with(""))
		case "crashed":
			measured(check, report.Broken, name+" crashed"+with(""))
		case "offline":
			measured(check, report.Broken, name+" offline"+with("wants "))
		}
	}
}

// A sound card on the machine itself, not an HDMI or USB one.
func sound(sys machine.System, facts *Facts, measured set) {
	for _, line := range strings.Split(sys.ReadAll("/proc/asound/cards"), "\n") {
		if !strings.Contains(line, "]:") || strings.Contains(line, "USB-Audio") || strings.Contains(strings.ToLower(line), "hdmi") {
			continue
		}
		facts.SoundCard = strings.TrimSpace(line)
	}
	if facts.SoundCard == "" {
		measured("speakers", report.Broken, "no built-in sound card")
		measured("microphones", report.Broken, "no built-in sound card")
	}
}

func power(sys machine.System, facts *Facts, measured set) {
	battery, charging, chargingState := "", "", ""
	for _, supply := range sys.Glob("/sys/class/power_supply/*") {
		name := filepath.Base(supply)
		switch sys.Read(supply + "/type") {
		case "Battery":
			percent, ok := sys.Int(supply + "/capacity")
			// Qualcomm's battery manager gives energy or charge, not always a percentage
			for _, kind := range []string{"energy", "charge"} {
				now, hasNow := sys.Int(supply + "/" + kind + "_now")
				full, hasFull := sys.Int(supply + "/" + kind + "_full")
				if !ok && hasNow && hasFull && full > 0 {
					percent, ok = now*100/full, true
				}
			}
			if ok {
				battery = fmt.Sprintf("%s at %d%%, %s", name, percent, sys.Read(supply+"/status"))
			}
		case "USB", "Mains", "USB_PD", "USB_C":
			if sys.Read(supply+"/online") != "1" {
				continue
			}
			kind := ""
			if m := selected.FindStringSubmatch(sys.Read(supply + "/usb_type")); m != nil {
				kind = m[1]
			}
			volts, _ := sys.Int(supply + "/voltage_now")
			amps, _ := sys.Int(supply + "/current_now")
			description := name
			if kind != "" {
				description += " " + kind
			}
			if volts > 0 {
				description += fmt.Sprintf(", %d mV", volts/1000)
			}
			if amps > 0 {
				description += fmt.Sprintf(", %d mA", amps/1000)
			}
			switch {
			case strings.HasPrefix(kind, "PD") || volts > 6_000_000:
				charging, chargingState = description, report.Works
			case charging == "" && volts > 0:
				charging, chargingState = description+" (5 V only)", report.Partial
			}
		}
	}
	if battery != "" {
		measured("battery", report.Works, battery)
	} else if facts.Laptop {
		measured("battery", report.Broken, "no battery reports its charge")
	}
	if charging != "" {
		measured("charging", chargingState, charging)
	}
}

// A machine that slept and is running now woke up from it.
func sleep(sys machine.System, measured set) {
	modes := sys.Read("/sys/power/mem_sleep")
	ok, _ := sys.Int("/sys/power/suspend_stats/success")
	failed, _ := sys.Int("/sys/power/suspend_stats/fail")
	switch {
	case ok > 0 && failed > 0:
		measured("suspend", report.Partial, fmt.Sprintf("%d sleeps and %d failures since boot, modes %s", ok, failed, modes))
	case ok > 0:
		measured("suspend", report.Works, fmt.Sprintf("%d sleeps since boot, none failed, modes %s", ok, modes))
	case failed > 0:
		measured("suspend", report.Broken, fmt.Sprintf("%d failed sleeps since boot, modes %s", failed, modes))
	}
	if !strings.Contains(" "+sys.Read("/sys/power/state")+" ", " disk ") {
		measured("hibernate", report.Broken, "the kernel offers no hibernation")
	}
}

func ports(sys machine.System, measured set) {
	controllers := sys.Glob("/sys/bus/usb/devices/usb*")
	fast := 0
	for _, controller := range controllers {
		if speed, _ := sys.Int(controller + "/speed"); speed >= 5000 {
			fast++
		}
	}
	if len(controllers) > 0 && fast == 0 {
		measured("usb", report.Partial, "no USB controller offers SuperSpeed (5 Gb/s or more)")
	}
	external := append(sys.Glob("/sys/class/drm/card*-DP-*"), sys.Glob("/sys/class/drm/card*-HDMI-*")...)
	for _, screen := range external {
		if sys.Read(screen+"/status") == "connected" {
			measured("external-display", report.Works, connector(screen)+" "+sys.Read(screen+"/modes")+" connected")
		}
	}
	for _, device := range sys.Glob("/sys/bus/thunderbolt/devices/[0-9]-[0-9]*") {
		measured("usb4", report.Works, fmt.Sprintf("USB4 device %s at %s Gb/s x %s", sys.Read(device+"/device_name"), sys.Read(device+"/rx_speed"), sys.Read(device+"/rx_lanes")))
	}
}

// A built-in camera is a sensor on I2C behind Qualcomm's camera subsystem, named like
// "ov02c10 10-0036"; a USB one shows as a video device of its own.
func media(sys machine.System, facts *Facts, measured set) {
	decoder, subsystem := "", false
	for _, video := range sys.Glob("/sys/class/video4linux/video*") {
		name := sys.Read(video + "/name")
		lower := strings.ToLower(name)
		switch {
		case strings.Contains(lower, "iris") || strings.Contains(lower, "venus") || strings.Contains(lower, "decoder"):
			decoder = name
		case strings.HasPrefix(name, "msm_vfe") || strings.HasPrefix(name, "msm_csi") || strings.Contains(lower, "camss"):
			subsystem = true
		case strings.Contains(lower, "camera") || strings.Contains(lower, "webcam") || strings.Contains(name, "UVC"):
			facts.Camera = name
		}
	}
	for _, subdev := range sys.Glob("/sys/class/video4linux/v4l-subdev*") {
		if name := sys.Read(subdev + "/name"); sensor.MatchString(name) {
			facts.Camera = strings.Fields(name)[0] + " sensor"
		}
	}
	switch {
	case facts.Camera == "" && subsystem:
		measured("camera", report.Broken, "the camera subsystem is up, but no sensor was found")
	case facts.Camera == "" && facts.Laptop:
		measured("camera", report.Broken, "no camera device")
	}
	if decoder == "" {
		measured("video-decode", report.Broken, "no iris or venus video decoder")
	}
}

func sensors(sys machine.System, facts *Facts, measured set) {
	fan := ""
	for _, input := range sys.Glob("/sys/class/hwmon/hwmon*/fan*_input") {
		if rpm := sys.Read(input); rpm != "" {
			fan = sys.Read(filepath.Dir(input)+"/name") + " " + rpm + " rpm"
		}
	}
	if fan != "" {
		measured("fans", report.Works, fmt.Sprintf("%s; %d temperature sensors", fan, len(sys.Glob("/sys/class/thermal/thermal_zone*/temp"))))
	}
	// Makers of fingerprint readers, by USB vendor
	readers := map[string]bool{"27c6": true, "06cb": true, "04f3": true, "1c7a": true, "10a5": true, "2808": true, "298d": true}
	for _, vendor := range sys.Glob("/sys/bus/usb/devices/*/idVendor") {
		if id := sys.Read(vendor); readers[id] {
			facts.Fingerprint = id + ":" + sys.Read(filepath.Dir(vendor)+"/idProduct")
		}
	}
}

// The families Omarchy sorts ARM machines into, each named by the line a pull request for
// one of its machines opens with.
const (
	DragonMsft   = "arm-dragon-msft"   // Snapdragon machines that came with Windows
	DragonGoogle = "arm-dragon-google" // Snapdragon Chromebooks and Googlebooks
	Nvidia       = "arm-nvidia"
	Apple        = "arm-apple" // the Omarchy Mac team's
)

// Family names the family of the machine from what it says of itself: the makers in its
// device tree, and where it started without one, its maker's name and its processor. It is
// "" when nothing tells.
func Family(sys machine.System) string {
	// One name to a line, as the file is read
	tree := strings.Split(sys.ReadAll("/proc/device-tree/compatible"), "\n")
	has := func(maker string) bool {
		for _, name := range tree {
			if strings.HasPrefix(strings.TrimSpace(name), maker+",") {
				return true
			}
		}
		return false
	}
	maker := strings.ToLower(sys.Read("/sys/class/dmi/id/sys_vendor") + " " + sys.Read("/sys/class/dmi/id/product_name"))
	cpu := strings.ToLower(sys.ReadAll("/proc/cpuinfo"))
	switch {
	case has("apple") || strings.Contains(maker, "apple"):
		return Apple
	case has("nvidia") || strings.Contains(maker, "nvidia"):
		return Nvidia
	case has("qcom") && has("google"):
		return DragonGoogle
	// 0x51 is Qualcomm's mark on the processors it designs, for a machine started without a tree
	case has("qcom") || strings.Contains(cpu, "implementer\t: 0x51"):
		return DragonMsft
	}
	return ""
}
