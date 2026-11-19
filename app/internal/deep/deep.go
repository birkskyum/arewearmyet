// Package deep holds the tests that measure further than the quick check and need nobody at
// the keyboard: which renderer the graphics really use, whether the browser gets WebGL and
// WebGPU, what the radios hear, whether a video decodes on the hardware decoder, and whether
// the temperature sensors and fans answer to load. Each takes from a few seconds to half a
// minute, so each can be run alone.
package deep

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

func result(check, state, evidence, note string) report.Result {
	return report.Result{Check: check, State: state, How: report.Measured, Evidence: report.Scrub(evidence, 380), Note: report.Scrub(note, 580)}
}

// software names the renderers that draw on the processor instead of the GPU.
func software(renderer string) bool {
	lower := strings.ToLower(renderer)
	for _, name := range []string{"llvmpipe", "lavapipe", "swiftshader", "softpipe", "software"} {
		if strings.Contains(lower, name) {
			return true
		}
	}
	return false
}

var vulkanField = regexp.MustCompile(`(?m)^\s*(deviceName|deviceType|driverName)\s*=\s*(.+?)\s*$`)

// Graphics asks Vulkan which device it draws on. The quick check sees that the GPU's driver is
// bound; this sees whether programs actually get the GPU or a renderer on the processor.
func Graphics(sys machine.System) []report.Result {
	out, err := sys.RunFor(15*time.Second, "vulkaninfo", "--summary")
	if err != nil && out == "" {
		return nil
	}
	// The first device listed is the one programs get by default
	fields := map[string]string{}
	for _, match := range vulkanField.FindAllStringSubmatch(out, -1) {
		if _, seen := fields[match[1]]; !seen {
			fields[match[1]] = match[2]
		}
	}
	name := fields["deviceName"]
	switch {
	case name == "":
		return []report.Result{result("gpu", report.Broken, "Vulkan lists no device", "")}
	case software(name) || strings.Contains(fields["deviceType"], "CPU"):
		return []report.Result{result("gpu", report.Partial, "Vulkan draws on "+name+" ("+fields["driverName"]+")", "Programs get a renderer that runs on the processor, not the GPU.")}
	}
	return []report.Result{result("gpu", report.Works, "Vulkan draws on "+name+" ("+fields["driverName"]+")", "")}
}

// The page the browser is given: it makes a WebGL context and asks for a WebGPU adapter and
// device, and sends what it got back to the test.
const browserPage = `<!doctype html><meta charset=utf-8><title>hw-test</title><script>
(async()=>{const r={};
try{const c=document.createElement("canvas");const gl=c.getContext("webgl2")||c.getContext("webgl");
 if(gl){const e=gl.getExtension("WEBGL_debug_renderer_info");r.webgl=e?gl.getParameter(e.UNMASKED_RENDERER_WEBGL):gl.getParameter(gl.RENDERER);r.webglVersion=gl.getParameter(gl.VERSION)}else r.webgl=""}catch(x){r.webglWhy=String(x)}
try{if(!navigator.gpu){r.webgpuWhy="navigator.gpu is missing"}else{const a=await navigator.gpu.requestAdapter();
 if(!a){r.webgpuWhy="no adapter"}else{const i=a.info||{};r.webgpu=[i.vendor,i.architecture,i.device,i.description].filter(Boolean).join(" ")||"an adapter";r.fallback=!!a.isFallbackAdapter;
 try{await a.requestDevice();r.device=true}catch(x){r.webgpuWhy=String(x)}}}}catch(x){r.webgpuWhy=String(x)}
fetch("/result",{method:"POST",body:JSON.stringify(r)})})();
</script>`

// Page is what the browser said it got.
type Page struct {
	WebGL        string `json:"webgl"`
	WebGLVersion string `json:"webglVersion"`
	WebGLWhy     string `json:"webglWhy"`
	WebGPU       string `json:"webgpu"`
	Fallback     bool   `json:"fallback"`
	Device       bool   `json:"device"`
	WebGPUWhy    string `json:"webgpuWhy"`
}

// The switches that turn WebGPU on in Chromium where it isn't on by itself.
var webgpuFlags = []string{"--enable-unsafe-webgpu", "--enable-features=Vulkan,WebGPU"}

// answerWait is how long the browser gets to open the page and answer.
var answerWait = 40 * time.Second

// ask serves the test page on this computer alone, opens it in Chromium without a window and
// waits for the page's answer, which doesn't depend on how long the GPU takes to come up. The
// browser runs in a profile of its own, so the owner's Chromium and its profile aren't touched.
func ask(sys machine.System, dir string, extra ...string) (Page, bool) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Page{}, false
	}
	answer := make(chan Page, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(browserPage))
	})
	mux.HandleFunc("/result", func(w http.ResponseWriter, r *http.Request) {
		var page Page
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&page) == nil {
			select {
			case answer <- page:
			default:
			}
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	profile, err := os.MkdirTemp(dir, "browser-")
	if err != nil {
		return Page{}, false
	}
	defer os.RemoveAll(profile)
	args := append([]string{"--headless=new", "--no-first-run", "--disable-extensions", "--user-data-dir=" + profile}, extra...)
	stop, gone := make(chan struct{}), make(chan struct{})
	go func() {
		sys.RunUntil(stop, answerWait+10*time.Second, "chromium", append(args, "http://"+listener.Addr().String()+"/")...)
		close(gone)
	}()
	var page Page
	ok := false
	select {
	case page = <-answer:
		ok = true
	case <-gone:
		// The browser went away: it either answered just before, or never will
		select {
		case page = <-answer:
			ok = true
		default:
		}
	case <-time.After(answerWait):
	}
	close(stop)
	<-gone
	return page, ok
}

// Browser opens the test page in Chromium without a window and reports whether WebGL and
// WebGPU work, and on which renderer. WebGPU is tried as the browser comes, and again with
// the switches that turn it on, to tell "works" from "works only when switched on by hand".
func Browser(sys machine.System, dir string) []report.Result {
	plain, ok := ask(sys, dir)
	if !ok {
		return nil
	}
	var results []report.Result
	switch {
	case plain.WebGL == "":
		results = append(results, result("webgl", report.Broken, "Chromium gave no WebGL context: "+plain.WebGLWhy, ""))
	case software(plain.WebGL):
		results = append(results, result("webgl", report.Partial, plain.WebGL+", "+plain.WebGLVersion, "WebGL runs on the processor, not the GPU."))
	default:
		results = append(results, result("webgl", report.Works, plain.WebGL+", "+plain.WebGLVersion, ""))
	}

	// An adapter that runs on the processor isn't the GPU doing WebGPU, whatever it calls itself
	onProcessor := func(page Page) bool { return page.WebGPU != "" && (page.Fallback || software(page.WebGPU)) }
	usable := func(page Page) bool { return page.WebGPU != "" && page.Device && !onProcessor(page) }
	if usable(plain) {
		return append(results, result("webgpu", report.Works, "Chromium offers WebGPU on "+plain.WebGPU, ""))
	}
	switches := strings.Join(webgpuFlags, " ")
	switched, ok := ask(sys, dir, webgpuFlags...)
	if ok && usable(switched) {
		return append(results, result("webgpu", report.Partial, "WebGPU on "+switched.WebGPU+" with "+switches+"; without them: "+first(plain.WebGPUWhy, "only "+plain.WebGPU),
			"Chromium doesn't offer WebGPU by itself here. It works when started with "+switches+"."))
	}
	why := first(switched.WebGPUWhy, plain.WebGPUWhy, "no answer")
	if onProcessor(plain) || onProcessor(switched) {
		why = "only an adapter that runs on the processor (" + first(switched.WebGPU, plain.WebGPU) + ")"
	}
	return append(results, result("webgpu", report.Broken, "Chromium offers no WebGPU, also with "+switches+": "+why, ""))
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var (
	linkSignal = regexp.MustCompile(`signal:\s*(-?\d+) dBm`)
	linkRate   = regexp.MustCompile(`tx bitrate:\s*([\d.]+) MBit/s`)
	linkFreq   = regexp.MustCompile(`freq:\s*(\d+)`)
	btDevice   = regexp.MustCompile(`(?m)^.*\bDevice\s+([0-9A-Fa-f:]{17})\b`)
)

// Wireless reads how good the Wi-Fi link is and listens for Bluetooth devices for a few
// seconds. The quick check sees that Wi-Fi is connected and Bluetooth powered; this sees what
// the radios make of the air around them.
func Wireless(sys machine.System, wifi, bluetooth string) []report.Result {
	var results []report.Result
	if wifi != "" {
		link, _ := sys.RunFor(10*time.Second, "iw", "dev", wifi, "link")
		if signal := linkSignal.FindStringSubmatch(link); signal != nil {
			evidence := "signal " + signal[1] + " dBm"
			if freq := linkFreq.FindStringSubmatch(link); freq != nil {
				evidence = freq[1] + " MHz, " + evidence
			}
			if rate := linkRate.FindStringSubmatch(link); rate != nil {
				evidence += ", sending at " + rate[1] + " Mb/s"
			}
			state, note := report.Works, ""
			if dbm, _ := strconv.Atoi(signal[1]); dbm < -82 {
				note = "The signal is very weak here, which can be the distance to the router as much as the card."
			}
			results = append(results, result("wifi", state, "connected at "+evidence, note))
		}
	}
	if bluetooth != "" {
		// The scan's own output names every device it hears; only how many is kept
		heard, _ := sys.RunFor(15*time.Second, "bluetoothctl", "--timeout", "8", "scan", "on")
		devices := map[string]bool{}
		for _, match := range btDevice.FindAllStringSubmatch(heard, -1) {
			devices[strings.ToUpper(match[1])] = true
		}
		if len(devices) > 0 {
			results = append(results, result("bluetooth", report.Works, fmt.Sprintf("heard %d Bluetooth devices nearby in 8 s", len(devices)), ""))
		}
	}
	return results
}

var frames = regexp.MustCompile(`frame=\s*(\d+)`)

// Video makes a two-second clip and decodes it on the hardware decoder. The quick check sees
// that a decoder exists; this sees whether it gives frames.
func Video(sys machine.System, dir string) []report.Result {
	clip := filepath.Join(dir, "clip.mp4")
	defer os.Remove(clip)
	if out, err := sys.RunFor(40*time.Second, "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=duration=2:size=1280x720:rate=30",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", clip); err != nil {
		// Nothing to decode with: no ffmpeg, or it can't make a clip
		_ = out
		return nil
	}
	started := time.Now()
	out, err := sys.RunFor(40*time.Second, "ffmpeg", "-hide_banner", "-loglevel", "error", "-stats", "-c:v", "h264_v4l2m2m", "-i", clip, "-f", "null", "-")
	took := time.Since(started)
	decoded := 0
	for _, match := range frames.FindAllStringSubmatch(out, -1) {
		decoded, _ = strconv.Atoi(match[1])
	}
	switch {
	case decoded >= 55:
		return []report.Result{result("video-decode", report.Works, fmt.Sprintf("%d frames of 720p H.264 decoded on the hardware decoder in %.1f s", decoded, took.Seconds()), "")}
	case decoded > 0:
		return []report.Result{result("video-decode", report.Partial, fmt.Sprintf("only %d of 60 frames came out of the hardware decoder", decoded), "")}
	}
	why := lastLine(out)
	if why == "" && err != nil {
		why = err.Error()
	}
	return []report.Result{result("video-decode", report.Broken, "the hardware decoder gave no frames: "+why, "")}
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(text, "\r", "\n")), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Heat is what the temperature sensors and the fan showed across a spell of load.
type Heat struct {
	Sensors int
	// Before and After are the hottest sensor's reading in degrees, at rest and at the end of the load.
	Before, After float64
	// Fan is the fan sensor's name, or "" when there is none; FanBefore and FanAfter its speed.
	Fan                 string
	FanBefore, FanAfter int64
}

func hottest(sys machine.System) (degrees float64, sensors int) {
	for _, zone := range sys.Glob("/sys/class/thermal/thermal_zone*/temp") {
		if value, ok := sys.Int(zone); ok && value > 0 {
			sensors++
			if d := float64(value) / 1000; d > degrees {
				degrees = d
			}
		}
	}
	return degrees, sensors
}

func fanSpeed(sys machine.System) (name string, rpm int64) {
	for _, input := range sys.Glob("/sys/class/hwmon/hwmon*/fan*_input") {
		if value, ok := sys.Int(input); ok {
			return sys.Read(filepath.Dir(input) + "/name"), value
		}
	}
	return "", 0
}

// Warm keeps every processor core busy for the given time and reads the sensors before and
// after: sensors that work show the heat, and a fan that works speeds up.
func Warm(sys machine.System, load time.Duration) Heat {
	var heat Heat
	heat.Before, heat.Sensors = hottest(sys)
	heat.Fan, heat.FanBefore = fanSpeed(sys)
	stop := make(chan struct{})
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			x := 1.0001
			for {
				select {
				case <-stop:
					return
				default:
					for n := 0; n < 200000; n++ {
						x = x*1.0000001 + 0.0000001
					}
				}
			}
		}()
	}
	time.Sleep(load)
	heat.After, _ = hottest(sys)
	_, heat.FanAfter = fanSpeed(sys)
	close(stop)
	return heat
}

// Report turns what the load showed into the fans check's result.
func (h Heat) Report() []report.Result {
	if h.Sensors == 0 {
		return []report.Result{result("fans", report.Broken, "no temperature sensor can be read", "")}
	}
	evidence := fmt.Sprintf("%d temperature sensors; the hottest went from %.0f to %.0f °C under load", h.Sensors, h.Before, h.After)
	responds := h.After-h.Before >= 2
	switch {
	case h.Fan != "" && h.FanAfter > h.FanBefore && responds:
		return []report.Result{result("fans", report.Works, fmt.Sprintf("%s; the fan (%s) went from %d to %d rpm", evidence, h.Fan, h.FanBefore, h.FanAfter), "")}
	case h.Fan != "":
		return []report.Result{result("fans", report.Partial, fmt.Sprintf("%s; the fan (%s) stayed at %d rpm", evidence, h.Fan, h.FanAfter), "The fan's speed can be read, but it didn't rise under load.")}
	case !responds:
		return []report.Result{result("fans", report.Partial, evidence+"; no fan speed sensor", "The temperature sensors can be read but barely moved under load, and there is no fan speed to read.")}
	}
	return []report.Result{result("fans", report.Partial, evidence+"; no fan speed sensor", FanUnread)}
}

// FanUnread is the fans check's note when the temperatures answer to load and no sensor gives
// a fan's speed. Whether the computer has a fan at all is then for its owner to say.
const FanUnread = "The temperatures can be read and answer to load. A fan's speed can't be read: there is no sensor for it."

// Load is how long the temperature test keeps the processor busy.
const Load = 20 * time.Second

// Test is one of the tests, as the app offers it: its name, what it says while it runs and
// what it measures.
type Test struct {
	Name string
	// Doing is the line shown while it runs, and About what it does and why.
	Doing, About string
	// Checks are the checks it can give a result for.
	Checks []string
	// Takes is roughly how long it runs.
	Takes time.Duration
	Run   func() []report.Result
	// Nothing is what to say when it could measure nothing.
	Nothing string
}

// Tests lists the tests this machine can run, in the order to run them: each needs the
// program it measures with, and the wireless one a radio. The temperature test comes last,
// so the heat it makes doesn't slow the others.
func Tests(sys machine.System, wifi, bluetooth, dir string) []Test {
	var tests []Test
	if sys.Has("vulkaninfo") {
		tests = append(tests, Test{Name: "Graphics", Doing: "Asking Vulkan which device it draws on", Checks: []string{"gpu"}, Takes: time.Second,
			About:   "Programs draw through Vulkan. This reads which device they get, the GPU or a renderer that runs on the processor.",
			Run:     func() []report.Result { return Graphics(sys) },
			Nothing: "Vulkan gave no answer here, so there is nothing to report."})
	}
	if sys.Has("chromium") {
		tests = append(tests, Test{Name: "Browser", Doing: "Opening a test page in Chromium, without a window", Checks: []string{"webgl", "webgpu"}, Takes: 5 * time.Second,
			About:   "Chromium opens a page that asks for WebGL and for WebGPU, which maps and 3D pages draw with. This reads whether it gets them, and whether on the GPU. No window opens, and your own Chromium isn't touched.",
			Run:     func() []report.Result { return Browser(sys, dir) },
			Nothing: "Chromium didn't answer the test page, so there is nothing to report."})
	}
	if (wifi != "" && sys.Has("iw")) || (bluetooth != "" && sys.Has("bluetoothctl")) {
		tests = append(tests, Test{Name: "Wireless", Doing: "Reading the Wi-Fi link and listening for Bluetooth devices", Checks: []string{"wifi", "bluetooth"}, Takes: 10 * time.Second,
			About:   "This reads how strong the Wi-Fi link is and how fast it sends, then listens for eight seconds for Bluetooth devices nearby. Only how many it hears is kept, never which.",
			Run:     func() []report.Result { return Wireless(sys, wifi, bluetooth) },
			Nothing: "Wi-Fi isn't connected and no Bluetooth device was heard nearby, which says nothing either way."})
	}
	if sys.Has("ffmpeg") {
		tests = append(tests, Test{Name: "Video decoding", Doing: "Decoding a two-second clip on the hardware decoder", Checks: []string{"video-decode"}, Takes: 5 * time.Second,
			About:   "This makes a short 720p clip and decodes it on the video engine, the way a player does to keep the processor idle.",
			Run:     func() []report.Result { return Video(sys, dir) },
			Nothing: "No clip could be made to decode, so there is nothing to report."})
	}
	return append(tests, Test{Name: "Temperatures", Doing: "Keeping every processor core busy and watching the sensors", Checks: []string{"fans"}, Takes: Load + time.Second,
		About:   fmt.Sprintf("Every core works for %.0f seconds. Sensors that work show the heat, and a fan whose speed can be read is seen speeding up. The computer may get louder for a moment.", Load.Seconds()),
		Run:     func() []report.Result { return Warm(sys, Load).Report() },
		Nothing: "No sensor answered."})
}
