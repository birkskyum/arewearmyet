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
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/probe"
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

// Engine is the browser the checks run in.
type Engine struct {
	Name    string
	Command string
	// Instead names the owner's default browser when this test can't drive it and another
	// browser stood in for it.
	Instead string
}

// The browsers this test can drive: the Chromium family, which all take the same switches.
// entry is the name of the desktop entry, without ".desktop".
var engines = []struct{ entry, name, command string }{
	{"google-chrome", "Chrome", "google-chrome-stable"},
	{"com.google.chrome", "Chrome", "google-chrome-stable"},
	{"chromium", "Chromium", "chromium"},
	{"brave-browser", "Brave", "brave"},
	{"vivaldi-stable", "Vivaldi", "vivaldi-stable"},
	{"microsoft-edge", "Edge", "microsoft-edge-stable"},
}

// Which picks the browser to test: the one the desktop opens for a link, since that is where
// the owner meets WebGL and WebGPU. Each browser reads its own switches (chrome-flags.conf,
// chromium-flags.conf), so one can have WebGPU while the other has not. When the default is not
// of the Chromium family, Chromium stands in where it is installed, and the result says so.
func Which(sys machine.System) (Engine, bool) {
	entry, err := sys.RunFor(3*time.Second, "omarchy-cmd-default-browser")
	if err != nil || strings.TrimSpace(entry) == "" {
		entry, _ = sys.RunFor(5*time.Second, "xdg-settings", "get", "default-web-browser")
	}
	entry = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(entry)), ".desktop")
	for _, known := range engines {
		if known.entry == entry && sys.Has(known.command) {
			return Engine{Name: known.name, Command: known.command}, true
		}
	}
	for _, known := range engines[2:] {
		if sys.Has(known.command) {
			return Engine{Name: known.name, Command: known.command, Instead: entry}, true
		}
	}
	if sys.Has(engines[0].command) {
		return Engine{Name: engines[0].name, Command: engines[0].command, Instead: entry}, true
	}
	return Engine{}, false
}

// The switches that turn WebGPU on in a Chromium browser where it isn't on by itself.
var webgpuFlags = []string{"--enable-unsafe-webgpu", "--enable-features=Vulkan,WebGPU"}

// answerWait is how long the browser gets to open the page and answer.
var answerWait = 40 * time.Second

// ask serves the test page on this computer alone, opens it in the browser without a window and
// waits for the page's answer, which doesn't depend on how long the GPU takes to come up. The
// browser runs in a profile of its own, so the owner's browser and its profile aren't touched.
func ask(sys machine.System, command, dir string, extra ...string) (Page, bool) {
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
		sys.RunUntil(stop, answerWait+10*time.Second, command, append(args, "http://"+listener.Addr().String()+"/")...)
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

// Browser opens the test page in the browser without a window and reports whether WebGL and
// WebGPU work, and on which renderer. WebGPU is tried as the browser comes, and again with
// the switches that turn it on, to tell "works" from "works only when switched on by hand".
// Every result names the browser it is about.
func Browser(sys machine.System, engine Engine, dir string) []report.Result {
	plain, ok := ask(sys, engine.Command, dir)
	if !ok {
		return nil
	}
	name := engine.Name
	// Said with each result when another browser stood in for the owner's default
	stood := ""
	if engine.Instead != "" {
		stood = "The default browser here (" + engine.Instead + ") is not one this test can drive, so this is " + name + "'s answer."
	}
	noted := func(note string) string { return strings.TrimSpace(note + " " + stood) }
	var results []report.Result
	switch {
	case plain.WebGL == "":
		results = append(results, result("webgl", report.Broken, name+" gave no WebGL context: "+plain.WebGLWhy, noted("")))
	case software(plain.WebGL):
		results = append(results, result("webgl", report.Partial, name+": "+plain.WebGL+", "+plain.WebGLVersion, noted("WebGL runs on the processor, not the GPU.")))
	default:
		results = append(results, result("webgl", report.Works, name+": "+plain.WebGL+", "+plain.WebGLVersion, noted("")))
	}

	// An adapter that runs on the processor isn't the GPU doing WebGPU, whatever it calls itself
	onProcessor := func(page Page) bool { return page.WebGPU != "" && (page.Fallback || software(page.WebGPU)) }
	usable := func(page Page) bool { return page.WebGPU != "" && page.Device && !onProcessor(page) }
	if usable(plain) {
		return append(results, result("webgpu", report.Works, name+" offers WebGPU on "+plain.WebGPU, noted("")))
	}
	switches := strings.Join(webgpuFlags, " ")
	switched, ok := ask(sys, engine.Command, dir, webgpuFlags...)
	if ok && usable(switched) {
		return append(results, result("webgpu", report.Partial, name+": WebGPU on "+switched.WebGPU+" with "+switches+"; without them: "+first(plain.WebGPUWhy, "only "+plain.WebGPU),
			noted(name+" doesn't offer WebGPU by itself here. It works when started with "+switches+".")))
	}
	why := first(switched.WebGPUWhy, plain.WebGPUWhy, "no answer")
	if onProcessor(plain) || onProcessor(switched) {
		why = "only an adapter that runs on the processor (" + first(switched.WebGPU, plain.WebGPU) + ")"
	}
	return append(results, result("webgpu", report.Broken, name+" offers no WebGPU, also with "+switches+": "+why, noted("")))
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

var frameCount = regexp.MustCompile(`frame=\s*(\d+)`)

// decode runs ffmpeg over the clip with the given way of decoding, and says how many frames came out.
func decode(sys machine.System, how ...string) (frames int, out string, err error, took time.Duration) {
	started := time.Now()
	args := append([]string{"-hide_banner", "-loglevel", "error", "-stats"}, how...)
	out, err = sys.RunFor(40*time.Second, "ffmpeg", append(args, "-f", "null", "-")...)
	took = time.Since(started)
	for _, match := range frameCount.FindAllStringSubmatch(out, -1) {
		frames, _ = strconv.Atoi(match[1])
	}
	return frames, out, err, took
}

// Video makes a two-second clip and decodes it on the hardware decoder. The quick check sees
// that a decoder exists; this sees whether it gives frames. A Snapdragon machine decodes on
// its video engine, an NVIDIA one on its GPU.
func Video(sys machine.System, dir string) []report.Result {
	clip := filepath.Join(dir, "clip.mp4")
	defer os.Remove(clip)
	if out, err := sys.RunFor(40*time.Second, "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=duration=2:size=1280x720:rate=30",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", clip); err != nil {
		// Nothing to decode with: no ffmpeg, or it can't make a clip
		_ = out
		return nil
	}
	switch probe.Platform(sys) {
	case "n1x", "gb10", "nvidia":
		return videoOnGPU(sys, clip)
	}
	decoded, out, err, took := decode(sys, "-c:v", "h264_v4l2m2m", "-i", clip)
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

// videoOnGPU decodes the clip on an NVIDIA GPU, through NVDEC and, failing that, through
// VA-API, which Omarchy sets up with libva-nvidia-driver. The frames are asked for as GPU
// frames and fetched back, so a decoder that quietly falls back to the processor gives an
// error and no frames, and doesn't pass for the GPU.
func videoOnGPU(sys machine.System, clip string) []report.Result {
	var whys []string
	for _, way := range []struct {
		name string
		how  []string
	}{
		{"NVDEC", []string{"-hwaccel", "nvdec", "-hwaccel_output_format", "cuda", "-i", clip, "-vf", "hwdownload,format=nv12"}},
		{"VA-API", []string{"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi", "-i", clip, "-vf", "hwdownload,format=nv12"}},
	} {
		decoded, out, err, took := decode(sys, way.how...)
		switch {
		case decoded >= 55:
			return []report.Result{result("video-decode", report.Works, fmt.Sprintf("%d frames of 720p H.264 decoded on the GPU through %s in %.1f s", decoded, way.name, took.Seconds()), "")}
		case decoded > 0:
			return []report.Result{result("video-decode", report.Partial, fmt.Sprintf("only %d of 60 frames came out of the GPU through %s", decoded, way.name), "")}
		}
		why := lastLine(out)
		if why == "" && err != nil {
			why = err.Error()
		}
		whys = append(whys, way.name+": "+why)
	}
	return []report.Result{result("video-decode", report.Broken, "the GPU gave no frames. "+strings.Join(whys, "; "), "")}
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(text, "\r", "\n")), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// How the temperature test runs: a warm machine cools first, then every core works until a
// fan is seen starting or the time is up, and the sensors are watched a little longer as it
// cools. The load ends early at a temperature well under where the hardware protects itself.
var (
	CoolFirst = 60 * time.Second
	MaxLoad   = 120 * time.Second
	CoolAfter = 15 * time.Second
)

const (
	// warm is the temperature above which the test waits for the machine to cool before it starts.
	warm = 50.0
	// margin is how close a sensor may come to the temperature its own shutdown is set at
	// before the load ends. A processor runs at 95 °C and more under full load and holds
	// itself there, so only the nearness of a shutdown ends the test.
	margin = 8.0
	// ceiling stands in for a sensor that names no shutdown temperature.
	ceiling = 105.0
	// answering is how far the hottest sensor has to move between rest and load to count as
	// showing the heat.
	answering = 5.0
	// fallen is how far the temperature has to fall under steady work to count as a fan starting.
	fallen = 3.0
)

// Sample is one reading during the temperature test.
type Sample struct {
	At time.Duration
	// Hot is the hottest sensor's reading in degrees.
	Hot float64
	// Rate is how much work the processor did in the second before, as a share of what it did
	// in the first seconds of the load. Under 1 it has been slowed down to stay cool.
	Rate float64
	// Fan is the fan's speed, where a sensor gives it.
	Fan int64
}

// Heat is what the temperature sensors and the fan showed across the test.
type Heat struct {
	Sensors int
	// Fan is the fan sensor's name, or "" when there is none.
	Fan string
	// Rest is the hottest sensor before the load, Peak its highest under load, and After its
	// reading once the machine had cooled for a moment.
	Rest, Peak, After float64
	// Loaded is how long the load ran.
	Loaded time.Duration
	// Samples are the readings under load, one a second.
	Samples []Sample
	// FanRest and FanPeak are the fan's speed before the load and its highest under it.
	FanRest, FanPeak int64
	// Kicked is set when the temperature fell while the processor went on working at the same
	// rate: more heat was being carried off, which is a fan starting.
	Kicked *Kick
	// Slowest is the lowest rate of work under load, as a share of the first seconds.
	Slowest float64
	// Heard is set when the owner said they heard the fan speed up, with the reading then.
	Heard *Sample
	// NoFan is set when the owner said the computer has no fan.
	NoFan bool
	// TooHot is set when the load was ended because a sensor came close to its shutdown temperature.
	TooHot bool
	// Graphics is set when the GPU was put to work as well as the processor.
	Graphics bool
	// Watts is the most the computer drew from its battery under load, or 0 on the charger.
	Watts float64
}

// Kick is a fan seen starting by the temperature it brought down.
type Kick struct {
	At       time.Duration
	From, To float64
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

// limits reads the temperature each sensor's shutdown is set at, by the sensor's folder.
func limits(sys machine.System) map[string]float64 {
	found := map[string]float64{}
	for _, kind := range sys.Glob("/sys/class/thermal/thermal_zone*/trip_point_*_type") {
		if sys.Read(kind) != "critical" {
			continue
		}
		if value, ok := sys.Int(strings.TrimSuffix(kind, "_type") + "_temp"); ok && value > 0 {
			zone := filepath.Dir(kind)
			if degrees := float64(value) / 1000; found[zone] == 0 || degrees < found[zone] {
				found[zone] = degrees
			}
		}
	}
	return found
}

// near says whether any sensor is close to the temperature its shutdown is set at.
func near(sys machine.System, limits map[string]float64) bool {
	for _, zone := range sys.Glob("/sys/class/thermal/thermal_zone*/temp") {
		value, ok := sys.Int(zone)
		if !ok || value <= 0 {
			continue
		}
		limit := limits[filepath.Dir(zone)]
		if limit == 0 {
			limit = ceiling
		}
		if float64(value)/1000 >= limit-margin {
			return true
		}
	}
	return false
}

// Warming is a temperature test under way. The app steps it once a second and draws what it
// reads; Warm runs it through by itself.
type Warming struct {
	sys   machine.System
	heat  Heat
	began time.Time
	stop  chan struct{}
	// work counts what the cores have done, and last what they had done at the reading before.
	work     atomic.Int64
	last     int64
	lastAt   time.Time
	baseline float64
	stopped  bool
	// limits are the temperatures the sensors' shutdowns are set at.
	limits map[string]float64
	// peakAt is the place in the samples of the highest reading.
	peakAt int
}

// Resting reads the sensors before any load. The app calls it while a warm machine cools.
func Resting(sys machine.System) (degrees float64, cool bool) {
	degrees, _ = hottest(sys)
	return degrees, degrees < warm
}

// StartWarming notes the readings at rest and sets every processor core to work, and the GPU
// too where there is a Chromium to do that with. dir is where the GPU's page is kept.
func StartWarming(sys machine.System, dir string) *Warming {
	w := &Warming{sys: sys, began: time.Now(), stop: make(chan struct{}), limits: limits(sys)}
	w.heat.Rest, w.heat.Sensors = hottest(sys)
	w.heat.Peak, w.heat.Slowest = w.heat.Rest, 1
	w.heat.Fan, w.heat.FanRest = fanSpeed(sys)
	w.heat.FanPeak = w.heat.FanRest
	w.lastAt = w.began
	for range runtime.NumCPU() {
		go w.labour()
	}
	w.graphics(dir)
	return w
}

// labour keeps one core as busy as it gets: eight streams of integer arithmetic and eight of
// multiply-and-add side by side, with a little memory traffic, so every part of the core has
// work in every cycle. On the Yoga Slim 7x this draws a quarter more power than hashing does.
func (w *Warming) labour() {
	var x [8]uint64
	var f [8]float64
	for i := range x {
		x[i] = uint64(i)*0x9E3779B97F4A7C15 + 1
		f[i] = 1 + float64(i)/10
	}
	table := make([]uint64, 1<<13)
	for {
		select {
		case <-w.stop:
			return
		default:
		}
		for n := 0; n < 200000; n++ {
			x[0] ^= x[0] << 13
			x[0] ^= x[0] >> 7
			x[0] ^= x[0] << 17
			x[1] ^= x[1] << 13
			x[1] ^= x[1] >> 7
			x[1] ^= x[1] << 17
			x[2] = x[2]*6364136223846793005 + 1442695040888963407
			x[3] = x[3]*6364136223846793005 + 1442695040888963407
			x[4] ^= x[4] << 13
			x[4] ^= x[4] >> 7
			x[4] ^= x[4] << 17
			x[5] = x[5]*0x2545F4914F6CDD1D + x[1]>>3
			x[6] += x[0] & x[2]
			x[7] ^= x[3] | x[4]
			f[0] = f[0]*1.0000001 + 0.25
			f[1] = f[1]*0.9999999 + 0.25
			f[2] = f[2]*1.0000001 + 0.25
			f[3] = f[3]*0.9999999 + 0.25
			f[4] = f[4]*1.0000001 + 0.25
			f[5] = f[5]*0.9999999 + 0.25
			f[6] = f[6]*1.0000001 + 0.25
			f[7] = f[7]*0.9999999 + 0.25
			table[x[0]&8191] += x[5]
			if x[1]&1 == 0 {
				table[x[2]&8191] ^= x[6]
			}
		}
		if f[0] > 1e300 {
			for i := range f {
				f[i] = 1
			}
		}
		w.work.Add(1)
	}
}

// The page that keeps the GPU busy: a shader with a long loop over a large canvas, drawn as
// often as the browser gets to it.
const gpuPage = `<!doctype html><meta charset=utf-8><title>hw-test load</title><canvas id=c width=2048 height=2048></canvas><script>
const gl=document.getElementById("c").getContext("webgl2")||document.getElementById("c").getContext("webgl");
const vs=gl.createShader(gl.VERTEX_SHADER);gl.shaderSource(vs,"attribute vec2 p;void main(){gl_Position=vec4(p,0.,1.);}");gl.compileShader(vs);
const fs=gl.createShader(gl.FRAGMENT_SHADER);gl.shaderSource(fs,"precision highp float;uniform float t;void main(){vec2 z=gl_FragCoord.xy/1024.-1.;vec2 c=vec2(sin(t)*.4-.4,cos(t*.7)*.4);float n=0.;for(int i=0;i<300;i++){z=vec2(z.x*z.x-z.y*z.y,2.*z.x*z.y)+c;n+=exp(-dot(z,z));}gl_FragColor=vec4(fract(n*.1),fract(n*.03),fract(n*.01),1.);}");gl.compileShader(fs);
const pr=gl.createProgram();gl.attachShader(pr,vs);gl.attachShader(pr,fs);gl.linkProgram(pr);gl.useProgram(pr);
const b=gl.createBuffer();gl.bindBuffer(gl.ARRAY_BUFFER,b);gl.bufferData(gl.ARRAY_BUFFER,new Float32Array([-1,-1,1,-1,-1,1,1,1]),gl.STATIC_DRAW);
const l=gl.getAttribLocation(pr,"p");gl.enableVertexAttribArray(l);gl.vertexAttribPointer(l,2,gl.FLOAT,false,0,0);
const u=gl.getUniformLocation(pr,"t");let t=0;const px=new Uint8Array(4);
function frame(){const end=performance.now()+12;do{t+=.01;gl.uniform1f(u,t);gl.drawArrays(gl.TRIANGLE_STRIP,0,4);gl.readPixels(0,0,1,1,gl.RGBA,gl.UNSIGNED_BYTE,px);}while(performance.now()<end);setTimeout(frame,0);}
frame();
</script>`

// graphics keeps the GPU busy as well, in a browser without a window and with a profile of
// its own, until the load ends. With it the Yoga Slim 7x draws 32 W, against 22 W for the
// processor alone. A machine without a browser this can drive is loaded by its processor only.
func (w *Warming) graphics(dir string) {
	engine, found := Which(w.sys)
	if dir == "" || !found {
		return
	}
	page, profile := filepath.Join(dir, "load.html"), filepath.Join(dir, "load-profile")
	if os.WriteFile(page, []byte(gpuPage), 0o600) != nil {
		return
	}
	w.heat.Graphics = true
	go func() {
		defer os.Remove(page)
		defer os.RemoveAll(profile)
		w.sys.RunUntil(w.stop, MaxLoad+30*time.Second, engine.Command, "--headless=new", "--no-first-run", "--disable-extensions", "--user-data-dir="+profile, "file://"+page)
	}()
}

// watts is what the computer draws from its battery right now, or 0 on the charger or where
// that can't be read.
func watts(sys machine.System) float64 {
	// With a charger in, the battery only makes up what the charger lacks, which says nothing
	// of what the computer draws
	for _, online := range sys.Glob("/sys/class/power_supply/*/online") {
		if sys.Read(online) == "1" {
			return 0
		}
	}
	for _, supply := range sys.Glob("/sys/class/power_supply/*/power_now") {
		if sys.Read(filepath.Dir(supply)+"/status") != "Discharging" {
			continue
		}
		if value, ok := sys.Int(supply); ok {
			return math.Abs(float64(value)) / 1e6
		}
	}
	return 0
}

// Step takes one reading under load and says whether the load is over: a fan was seen
// starting, the time is up, or the machine got hot enough to stop at.
func (w *Warming) Step() (Sample, bool) {
	now := time.Now()
	done := w.work.Load()
	rate := float64(done-w.last) / max(now.Sub(w.lastAt).Seconds(), 0.001)
	w.last, w.lastAt = done, now
	sample := Sample{At: now.Sub(w.began), Rate: 1}
	sample.Hot, _ = hottest(w.sys)
	_, sample.Fan = fanSpeed(w.sys)
	// The first seconds, before any heat, are what the processor does unhindered
	switch count := len(w.heat.Samples); {
	case count < 3:
		w.baseline = max(w.baseline, rate)
	case w.baseline > 0:
		sample.Rate = rate / w.baseline
	}
	w.heat.Samples = append(w.heat.Samples, sample)
	// One slow second is something else wanting the processor; five in a row is it being held back
	if count := len(w.heat.Samples); count >= 8 {
		w.heat.Slowest = min(w.heat.Slowest, mean(w.heat.Samples, count-5, count, func(s Sample) float64 { return s.Rate }))
	}
	w.heat.FanPeak = max(w.heat.FanPeak, sample.Fan)
	w.heat.Watts = max(w.heat.Watts, watts(w.sys))
	if sample.Hot > w.heat.Peak {
		w.heat.Peak, w.peakAt = sample.Hot, len(w.heat.Samples)-1
	}
	w.heat.Kicked = w.kicked()
	w.heat.TooHot = near(w.sys, w.limits)
	over := w.heat.Kicked != nil || sample.At >= MaxLoad || w.heat.TooHot || w.fanRose() && w.heat.Peak-w.heat.Rest >= answering
	return sample, over
}

// fanRose says whether the fan's speed, where it can be read, went clearly up under load.
func (w *Warming) fanRose() bool {
	return w.heat.Fan != "" && w.heat.FanPeak > w.heat.FanRest && float64(w.heat.FanPeak) >= 1.15*float64(w.heat.FanRest)
}

// mean is the average of the samples' readings from one place up to another.
func mean(samples []Sample, from, to int, of func(Sample) float64) float64 {
	from, to = max(from, 0), min(to, len(samples))
	if to <= from {
		return 0
	}
	total := 0.0
	for _, sample := range samples[from:to] {
		total += of(sample)
	}
	return total / float64(to-from)
}

// kicked looks for a fan starting: after the peak the temperature has stayed clearly lower
// for a few seconds, while the processor did as much work as it did around the peak. Heat
// was going in as before, so more of it was being carried off. A processor that was slowed
// down instead cools the same way, and isn't counted.
func (w *Warming) kicked() *Kick {
	samples := w.heat.Samples
	count := len(samples)
	if count-w.peakAt < 4 || w.heat.Peak-w.heat.Rest < answering {
		return nil
	}
	hot := func(s Sample) float64 { return s.Hot }
	rate := func(s Sample) float64 { return s.Rate }
	now := mean(samples, count-3, count, hot)
	if w.heat.Peak-now < fallen {
		return nil
	}
	if mean(samples, count-3, count, rate) < 0.93*mean(samples, w.peakAt-4, w.peakAt+1, rate) {
		return nil
	}
	return &Kick{At: samples[w.peakAt].At, From: w.heat.Peak, To: now}
}

// Heard notes that the owner heard the fan speed up at this moment.
func (w *Warming) Heard() {
	if count := len(w.heat.Samples); count > 0 && w.heat.Heard == nil {
		sample := w.heat.Samples[count-1]
		w.heat.Heard = &sample
	}
}

// NoFan notes that the owner says the computer has no fan.
func (w *Warming) NoFan() { w.heat.NoFan = true }

// Stop ends the load. It is safe to call more than once.
func (w *Warming) Stop() {
	if !w.stopped {
		w.stopped = true
		w.heat.Loaded = time.Since(w.began)
		close(w.stop)
	}
}

// Cooled takes the reading after the load, which shows the sensors following the heat down.
func (w *Warming) Cooled() float64 {
	w.heat.After, _ = hottest(w.sys)
	return w.heat.After
}

// Heat is what the test has found so far.
func (w *Warming) Heat() Heat { return w.heat }

// Warm runs the whole temperature test with nobody watching, telling what it reads as it goes.
func Warm(sys machine.System, dir string, say func(string)) Heat {
	// A machine that is warm from what it did before cools first, so the load has room to show
	waited := time.Now()
	for degrees, cool := Resting(sys); !cool && time.Since(waited) < CoolFirst; degrees, cool = Resting(sys) {
		say(fmt.Sprintf("  letting it cool first: %.0f °C", degrees))
		time.Sleep(pace * 5)
	}
	w := StartWarming(sys, dir)
	defer w.Stop()
	for {
		time.Sleep(pace)
		sample, over := w.Step()
		if count := len(w.heat.Samples); count%10 == 0 || over {
			say(fmt.Sprintf("  %3.0f s  %.0f °C  working at %.0f%%", sample.At.Seconds(), sample.Hot, sample.Rate*100))
		}
		if over {
			break
		}
	}
	w.Stop()
	time.Sleep(time.Duration(float64(CoolAfter) * float64(pace) / float64(time.Second)))
	w.Cooled()
	return w.Heat()
}

// pace is the time between readings, which tests shorten.
var pace = time.Second

// Report turns what the test showed into the fans check's result.
func (h Heat) Report() []report.Result {
	if h.Sensors == 0 {
		return []report.Result{result("fans", report.Broken, "no temperature sensor can be read", "")}
	}
	busy := "every core busy"
	if h.Graphics {
		busy = "every core and the GPU busy"
	}
	if h.Watts > 0 {
		busy += fmt.Sprintf(", drawing up to %.0f W from the battery", h.Watts)
	}
	evidence := fmt.Sprintf("%d temperature sensors; the hottest went from %.0f to %.0f °C in %.0f s with %s, and back to %.0f °C after", h.Sensors, h.Rest, h.Peak, h.Loaded.Seconds(), busy, h.After)
	if h.Slowest > 0 && h.Slowest < 0.75 {
		evidence += fmt.Sprintf("; the processor was slowed to %.0f%% of its speed as it got hot", h.Slowest*100)
	}
	if h.TooHot {
		evidence += "; the load was ended there, close to the temperature the computer shuts down at"
	}
	// The sensors show the heat when they rise with the load or fall once it ends
	responds := h.Peak-h.Rest >= answering || h.Peak-h.After >= answering
	rose := h.Fan != "" && h.FanPeak > h.FanRest && float64(h.FanPeak) >= 1.15*float64(h.FanRest)
	unread := ""
	switch {
	case h.Fan != "":
		evidence += fmt.Sprintf("; the fan (%s) went from %d to %d rpm", h.Fan, h.FanRest, h.FanPeak)
	default:
		evidence += "; no fan speed sensor"
		unread = " The fan's speed can't be read, for want of a sensor."
	}
	if h.Kicked != nil {
		evidence += fmt.Sprintf("; after %.0f s the temperature fell from %.0f to %.0f °C while the processor worked on at the same rate, as when a fan starts", h.Kicked.At.Seconds(), h.Kicked.From, h.Kicked.To)
	}
	if h.Heard != nil {
		evidence += fmt.Sprintf("; the owner heard the fan speed up at %.0f °C, %.0f s in", h.Heard.Hot, h.Heard.At.Seconds())
	}
	switch {
	case !responds:
		return []report.Result{result("fans", report.Partial, evidence, "The temperature sensors can be read, but barely moved between rest and full load.")}
	case rose || h.Kicked != nil:
		return []report.Result{result("fans", report.Works, evidence, strings.TrimSpace(unread))}
	case h.Heard != nil:
		return []report.Result{{Check: "fans", State: report.Works, How: report.Owner, Evidence: report.Scrub(evidence, 380), Note: strings.TrimSpace(unread)}}
	case h.NoFan:
		return []report.Result{{Check: "fans", State: report.Works, How: report.Owner, Evidence: report.Scrub(evidence, 380), Note: "This computer has no fan, by its owner's word. Its temperatures can be read and answer to load."}}
	case h.Fan != "":
		return []report.Result{result("fans", report.Partial, evidence, "The fan's speed can be read, but it didn't rise under load.")}
	}
	return []report.Result{result("fans", report.Partial, evidence, FanUnseen)}
}

// FanUnseen is the fans check's note when the temperatures answer to load and nothing showed
// a fan starting. Whether the computer has a fan at all is then for its owner to say.
const FanUnseen = "The temperatures can be read and answer to load. No fan was seen starting, and there is no sensor for its speed."

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
	// Run measures, telling what it reads as it goes where that takes a while.
	Run func(say func(string)) []report.Result
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
			Run:     func(func(string)) []report.Result { return Graphics(sys) },
			Nothing: "Vulkan gave no answer here, so there is nothing to report."})
	}
	if engine, found := Which(sys); found {
		tests = append(tests, Test{Name: "Browser", Doing: "Opening a test page in " + engine.Name + ", without a window", Checks: []string{"webgl", "webgpu"}, Takes: 5 * time.Second,
			About:   engine.Name + " opens a page that asks for WebGL and for WebGPU, which maps and 3D pages draw with. This reads whether it gets them, and whether on the GPU. No window opens, and your own browser profile is not touched.",
			Run:     func(func(string)) []report.Result { return Browser(sys, engine, dir) },
			Nothing: engine.Name + " didn't answer the test page, so there is nothing to report."})
	}
	if (wifi != "" && sys.Has("iw")) || (bluetooth != "" && sys.Has("bluetoothctl")) {
		tests = append(tests, Test{Name: "Wireless", Doing: "Reading the Wi-Fi link and listening for Bluetooth devices", Checks: []string{"wifi", "bluetooth"}, Takes: 10 * time.Second,
			About:   "This reads how strong the Wi-Fi link is and how fast it sends, then listens for eight seconds for Bluetooth devices nearby. Only how many it hears is kept, never which.",
			Run:     func(func(string)) []report.Result { return Wireless(sys, wifi, bluetooth) },
			Nothing: "Wi-Fi isn't connected and no Bluetooth device was heard nearby, which says nothing either way."})
	}
	if sys.Has("ffmpeg") {
		tests = append(tests, Test{Name: "Video decoding", Doing: "Decoding a two-second clip on the hardware decoder", Checks: []string{"video-decode"}, Takes: 5 * time.Second,
			About:   "This makes a short 720p clip and decodes it on the video engine, the way a player does to keep the processor idle.",
			Run:     func(func(string)) []report.Result { return Video(sys, dir) },
			Nothing: "No clip could be made to decode, so there is nothing to report."})
	}
	return append(tests, Test{Name: "Temperatures", Doing: "Keeping every processor core busy and watching the sensors", Checks: []string{"fans"}, Takes: MaxLoad + CoolAfter,
		About:   fmt.Sprintf("Every core and the GPU work for up to %.0f seconds, until a fan is seen starting. Sensors that work show the heat, and a fan shows by its speed or by the temperature it brings down. The computer may get loud.", MaxLoad.Seconds()),
		Run:     func(say func(string)) []report.Result { return Warm(sys, dir, say).Report() },
		Nothing: "No sensor answered."})
}
