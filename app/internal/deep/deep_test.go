package deep

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// with is a machine whose programs answer as given: by name, or by name and an argument.
func with(t *testing.T, answers map[string]string) machine.System {
	t.Helper()
	sys := machine.Fixture(t.TempDir())
	sys.Command = func(name string, args ...string) (string, error) {
		if name == "which" {
			if _, ok := answers[args[0]]; ok {
				return "/usr/bin/" + args[0], nil
			}
			return "", os.ErrNotExist
		}
		for _, arg := range args {
			if out, ok := answers[name+" "+arg]; ok {
				return out, nil
			}
		}
		if out, ok := answers[name]; ok {
			return out, nil
		}
		return "", os.ErrNotExist
	}
	return sys
}

func one(t *testing.T, results []report.Result, check string) report.Result {
	t.Helper()
	for _, result := range results {
		if result.Check == check {
			return result
		}
	}
	t.Fatalf("no result for %s in %+v", check, results)
	return report.Result{}
}

// What vulkaninfo --summary prints on the Yoga Slim 7x, and with only the processor to draw on.
const adreno = `Devices:
========
GPU0:
	apiVersion         = 1.4.335
	driverVersion      = 26.2.3
	vendorID           = 0x5143
	deviceID           = 0x43050c01
	deviceType         = PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU
	deviceName         = Adreno X1-85
	driverID           = DRIVER_ID_MESA_TURNIP
	driverName         = turnip Mesa driver
GPU1:
	deviceType         = PHYSICAL_DEVICE_TYPE_CPU
	deviceName         = llvmpipe (LLVM 21.1.6, 128 bits)
	driverName         = llvmpipe
`

const onlyTheProcessor = `Devices:
GPU0:
	deviceType         = PHYSICAL_DEVICE_TYPE_CPU
	deviceName         = llvmpipe (LLVM 21.1.6, 128 bits)
	driverName         = llvmpipe
`

func TestGraphicsTellsTheGPUFromARendererOnTheProcessor(t *testing.T) {
	gpu := one(t, Graphics(with(t, map[string]string{"vulkaninfo": adreno})), "gpu")
	if gpu.State != report.Works || gpu.Evidence != "Vulkan draws on Adreno X1-85 (turnip Mesa driver)" || gpu.How != report.Measured {
		t.Errorf("on the GPU: %+v", gpu)
	}
	gpu = one(t, Graphics(with(t, map[string]string{"vulkaninfo": onlyTheProcessor})), "gpu")
	if gpu.State != report.Partial || !strings.Contains(gpu.Evidence, "llvmpipe") || !strings.Contains(gpu.Note, "not the GPU") {
		t.Errorf("on the processor: %+v", gpu)
	}
	gpu = one(t, Graphics(with(t, map[string]string{"vulkaninfo": "Devices:\n========\n"})), "gpu")
	if gpu.State != report.Broken {
		t.Errorf("no device: %+v", gpu)
	}
	// Without vulkaninfo there is nothing to say
	if results := Graphics(with(t, nil)); results != nil {
		t.Errorf("without the tool: %+v", results)
	}
}

// browser is a machine whose Chromium answers the test page one way as it comes, and another
// when started with the switches that turn WebGPU on. It fetches the page and sends its answer
// back the way the page's script does. An empty answer is a browser that never answers.
func browser(t *testing.T, plain, switched string) machine.System {
	sys := machine.Fixture(t.TempDir())
	sys.Command = func(name string, args ...string) (string, error) {
		if name != "chromium" {
			return "", os.ErrNotExist
		}
		answer, url, profile := plain, args[len(args)-1], ""
		for _, arg := range args {
			if arg == "--enable-unsafe-webgpu" {
				answer = switched
			}
			if dir, ok := strings.CutPrefix(arg, "--user-data-dir="); ok {
				profile = dir
			}
		}
		// A profile of its own, and never the owner's
		if info, err := os.Stat(profile); err != nil || !info.IsDir() {
			t.Errorf("the browser has no profile of its own: %q", profile)
		}
		got, err := http.Get(url)
		if err != nil {
			t.Errorf("the page isn't served: %v", err)
			return "", err
		}
		body, _ := io.ReadAll(got.Body)
		got.Body.Close()
		if !strings.Contains(string(body), "requestAdapter") {
			t.Errorf("the page doesn't ask for WebGPU: %s", body)
		}
		if answer != "" {
			sent, err := http.Post(url+"result", "text/plain", strings.NewReader(answer))
			if err != nil {
				t.Errorf("the answer can't be sent: %v", err)
				return "", err
			}
			sent.Body.Close()
		}
		return "", nil
	}
	return sys
}

func TestTheBrowserReportsWebGLAndWhetherWebGPUNeedsSwitchingOn(t *testing.T) {
	onGPU := `"webgl":"ANGLE (Qualcomm, Adreno X1-85, OpenGL ES 3.2)","webglVersion":"WebGL 2.0 (OpenGL ES 3.0 Chromium)"`
	adapter := `"webgpu":"qualcomm adreno-7xx","device":true`
	dir := t.TempDir()

	// As on the Yoga: WebGL on the GPU, WebGPU only with the switches
	results := Browser(browser(t, `{`+onGPU+`,"webgpuWhy":"no adapter"}`, `{`+onGPU+`,`+adapter+`}`), dir)
	webgl, webgpu := one(t, results, "webgl"), one(t, results, "webgpu")
	if webgl.State != report.Works || !strings.Contains(webgl.Evidence, "Adreno X1-85") {
		t.Errorf("webgl: %+v", webgl)
	}
	if webgpu.State != report.Partial || !strings.Contains(webgpu.Evidence, "--enable-unsafe-webgpu") || !strings.Contains(webgpu.Evidence, "no adapter") || !strings.Contains(webgpu.Note, "doesn't offer WebGPU by itself") {
		t.Errorf("webgpu with switches: %+v", webgpu)
	}

	// WebGPU as the browser comes
	if webgpu = one(t, Browser(browser(t, `{`+onGPU+`,`+adapter+`}`, `{}`), dir), "webgpu"); webgpu.State != report.Works || !strings.Contains(webgpu.Evidence, "qualcomm adreno-7xx") {
		t.Errorf("webgpu by itself: %+v", webgpu)
	}

	// No GPU behind either: WebGL on the processor, and an adapter on the processor isn't
	// WebGPU working, whether or not it calls itself a fallback
	for _, fallback := range []string{"true", "false"} {
		software := `{"webgl":"ANGLE (Google, Vulkan 1.3.0 (SwiftShader Device (LLVM 10.0.0)), SwiftShader driver)","webglVersion":"WebGL 2.0","webgpu":"google swiftshader","fallback":` + fallback + `,"device":true}`
		results = Browser(browser(t, software, software), dir)
		if webgl = one(t, results, "webgl"); webgl.State != report.Partial || !strings.Contains(webgl.Note, "processor") {
			t.Errorf("webgl on the processor: %+v", webgl)
		}
		if webgpu = one(t, results, "webgpu"); webgpu.State != report.Broken || !strings.Contains(webgpu.Evidence, "runs on the processor (google swiftshader)") {
			t.Errorf("webgpu on the processor, fallback %s: %+v", fallback, webgpu)
		}
	}

	// No WebGL at all
	none := `{"webgl":"","webgpuWhy":"navigator.gpu is missing"}`
	results = Browser(browser(t, none, none), dir)
	if webgl = one(t, results, "webgl"); webgl.State != report.Broken {
		t.Errorf("no webgl: %+v", webgl)
	}
	if webgpu = one(t, results, "webgpu"); webgpu.State != report.Broken || !strings.Contains(webgpu.Evidence, "navigator.gpu is missing") {
		t.Errorf("no webgpu: %+v", webgpu)
	}

	// A browser that goes away without answering reports nothing, at once, and nothing of the
	// test is left behind
	began := time.Now()
	if results := Browser(browser(t, "", ""), dir); results != nil {
		t.Errorf("no answer: %+v", results)
	}
	if waited := time.Since(began); waited > 5*time.Second {
		t.Errorf("it waited %s for a browser that was gone", waited)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("%d files left behind", len(left))
	}
}

func TestABrowserThatHangsIsGivenUpOn(t *testing.T) {
	before := answerWait
	answerWait = 150 * time.Millisecond
	defer func() { answerWait = before }()
	release := make(chan struct{})
	sys := machine.Fixture(t.TempDir())
	sys.Command = func(string, ...string) (string, error) {
		<-release
		return "", nil
	}
	done := make(chan []report.Result, 1)
	go func() { done <- Browser(sys, t.TempDir()) }()
	time.Sleep(400 * time.Millisecond)
	// The wait is over by now; the program itself is ended by the limit on a real machine
	close(release)
	select {
	case results := <-done:
		if results != nil {
			t.Errorf("a hung browser reported %+v", results)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the test never gave up")
	}
}

const link = `Connected to aa:bb:cc:dd:ee:ff (on wlan0)
	SSID: HomeNetwork
	freq: 5180.0
	RX: 120394 bytes (812 packets)
	signal: -48 dBm
	rx bitrate: 2401.9 MBit/s
	tx bitrate: 2401.9 MBit/s 160MHz EHT-MCS 11
`

const scan = `SetDiscoveryFilter success
Discovery started
[CHG] Controller 11:22:33:44:55:66 Discovering: yes
[NEW] Device AA:BB:CC:00:11:22 Living Room TV
[NEW] Device AA:BB:CC:00:11:33 AA-BB-CC-00-11-33
[CHG] Device AA:BB:CC:00:11:22 RSSI: 0xffffffc4 (-60)
[NEW] Device aa:bb:cc:00:11:44 Someone's Headphones
`

func TestWirelessKeepsTheSignalAndOnlyHowManyDevicesWereHeard(t *testing.T) {
	results := Wireless(with(t, map[string]string{"iw": link, "bluetoothctl": scan}), "wlan0", "hci0")
	wifi, bluetooth := one(t, results, "wifi"), one(t, results, "bluetooth")
	if wifi.State != report.Works || wifi.Evidence != "connected at 5180 MHz, signal -48 dBm, sending at 2401.9 Mb/s" {
		t.Errorf("wifi: %+v", wifi)
	}
	if bluetooth.State != report.Works || bluetooth.Evidence != "heard 3 Bluetooth devices nearby in 8 s" {
		t.Errorf("bluetooth: %+v", bluetooth)
	}
	// Nothing that names the network, the router or a neighbour's device leaves the machine
	for _, result := range results {
		text := strings.ToLower(result.Evidence + result.Note)
		for _, private := range []string{"homenetwork", "aa:bb", "living room", "headphones", "11:22"} {
			if strings.Contains(text, private) {
				t.Errorf("%s gives away %q: %s", result.Check, private, text)
			}
		}
	}

	// A weak signal still works, with a word on what it may be
	weak := one(t, Wireless(with(t, map[string]string{"iw": strings.Replace(link, "-48", "-88", 1)}), "wlan0", ""), "wifi")
	if weak.State != report.Works || !strings.Contains(weak.Note, "very weak") {
		t.Errorf("weak: %+v", weak)
	}
	// Not connected, and nothing heard: neither says the radio is broken, so nothing is reported
	if results := Wireless(with(t, map[string]string{"iw": "Not connected.\n", "bluetoothctl": "Discovery started\n"}), "wlan0", "hci0"); len(results) != 0 {
		t.Errorf("nothing to go on: %+v", results)
	}
	// A machine without the radio isn't asked about it
	if results := Wireless(with(t, map[string]string{"iw": link, "bluetoothctl": scan}), "", ""); len(results) != 0 {
		t.Errorf("no radios: %+v", results)
	}
}

func TestVideoCountsTheFramesTheHardwareDecoderGave(t *testing.T) {
	decoder := func(output string, err error) machine.System {
		sys := machine.Fixture(t.TempDir())
		sys.Command = func(name string, args ...string) (string, error) {
			for _, arg := range args {
				if arg == "h264_v4l2m2m" {
					return output, err
				}
			}
			return "", nil // the clip was made
		}
		return sys
	}
	dir := t.TempDir()
	// ffmpeg rewrites its progress line in place, so the last count is the one that stands
	video := one(t, Video(decoder("frame=   21 fps=0.0 q=-0.0 size=N/A\rframe=   60 fps=0.0 q=-0.0 Lsize=N/A time=00:00:02.00\n", nil), dir), "video-decode")
	if video.State != report.Works || !strings.Contains(video.Evidence, "60 frames of 720p H.264") {
		t.Errorf("all frames: %+v", video)
	}
	if video = one(t, Video(decoder("frame=   12 fps=0.0 q=-0.0 Lsize=N/A\n", nil), dir), "video-decode"); video.State != report.Partial || !strings.Contains(video.Evidence, "12 of 60") {
		t.Errorf("some frames: %+v", video)
	}
	video = one(t, Video(decoder("[h264_v4l2m2m @ 0xaaaa] Could not find a valid device\nError while opening decoder: Invalid argument\n", errors.New("exit status 234")), dir), "video-decode")
	if video.State != report.Broken || !strings.Contains(video.Evidence, "Error while opening decoder") {
		t.Errorf("no decoder: %+v", video)
	}
	// Without ffmpeg there is no clip to decode, and nothing to report
	if results := Video(with(t, nil), dir); results != nil {
		t.Errorf("without ffmpeg: %+v", results)
	}
}

func TestTheLoadShowsOnTheSensorsAndTheFan(t *testing.T) {
	if fans := one(t, Heat{}.Report(), "fans"); fans.State != report.Broken {
		t.Errorf("no sensors: %+v", fans)
	}
	hot := Heat{Sensors: 59, Rest: 38, Peak: 68, After: 47, Loaded: 95 * time.Second, Slowest: 1}

	// Nothing showed a fan: the sensors work, and whether there is a fan is still open
	fans := one(t, hot.Report(), "fans")
	if fans.State != report.Partial || fans.Note != FanUnseen || !strings.Contains(fans.Evidence, "59 temperature sensors; the hottest went from 38 to 68 °C in 95 s with every core busy, and back to 47 °C") || !strings.Contains(fans.Evidence, "back to 47 °C") {
		t.Errorf("no fan seen: %+v", fans)
	}
	// The temperature fell under steady work: a fan started, measured with no sensor for its speed
	kicked := hot
	kicked.Kicked = &Kick{At: 90 * time.Second, From: 68, To: 62}
	if fans = one(t, kicked.Report(), "fans"); fans.State != report.Works || fans.How != report.Measured || !strings.Contains(fans.Evidence, "fell from 68 to 62 °C while the processor worked on at the same rate") || !strings.Contains(fans.Note, "speed can't be read") {
		t.Errorf("a fan seen by its cooling: %+v", fans)
	}
	// The owner heard it
	heard := hot
	heard.Heard = &Sample{At: 88 * time.Second, Hot: 67}
	if fans = one(t, heard.Report(), "fans"); fans.State != report.Works || fans.How != report.Owner || !strings.Contains(fans.Evidence, "heard the fan speed up at 67 °C, 88 s in") {
		t.Errorf("a fan heard: %+v", fans)
	}
	// No fan to wait for
	fanless := hot
	fanless.NoFan = true
	if fans = one(t, fanless.Report(), "fans"); fans.State != report.Works || fans.How != report.Owner || !strings.Contains(fans.Note, "no fan") {
		t.Errorf("no fan: %+v", fans)
	}
	// Sensors that stay where they were, whatever the owner heard
	still := Heat{Sensors: 59, Rest: 52, Peak: 53, After: 52.5, Loaded: 2 * time.Minute, Heard: &Sample{Hot: 53}}
	if fans = one(t, still.Report(), "fans"); fans.State != report.Partial || !strings.Contains(fans.Note, "barely moved") {
		t.Errorf("sensors that don't move: %+v", fans)
	}
	// A machine that was already warm rises little, and shows the heat by cooling after
	warm := Heat{Sensors: 59, Rest: 66, Peak: 69, After: 52, Loaded: 40 * time.Second, Kicked: &Kick{From: 69, To: 65}}
	if fans = one(t, warm.Report(), "fans"); fans.State != report.Works {
		t.Errorf("already warm: %+v", fans)
	}
	// A fan whose speed can be read
	read := Heat{Sensors: 8, Rest: 45, Peak: 68, After: 50, Fan: "thinkpad", FanRest: 0, FanPeak: 3200}
	if fans = one(t, read.Report(), "fans"); fans.State != report.Works || fans.Note != "" || !strings.Contains(fans.Evidence, "from 0 to 3200 rpm") {
		t.Errorf("a fan that speeds up: %+v", fans)
	}
	read.FanRest, read.FanPeak = 2100, 2150
	if fans = one(t, read.Report(), "fans"); fans.State != report.Partial || !strings.Contains(fans.Note, "didn't rise") {
		t.Errorf("a fan that stays: %+v", fans)
	}
	// What did the heating is said, with what it drew where the battery tells
	loaded := hot
	loaded.Graphics, loaded.Watts = true, 31.8
	if fans = one(t, loaded.Report(), "fans"); !strings.Contains(fans.Evidence, "with every core and the GPU busy, drawing up to 32 W from the battery") {
		t.Errorf("the load: %+v", fans)
	}
	// A processor slowed down to stay cool is said, as it is what the owner feels
	slowed := hot
	slowed.Slowest = 0.6
	if fans = one(t, slowed.Report(), "fans"); !strings.Contains(fans.Evidence, "slowed to 60% of its speed") {
		t.Errorf("slowed: %+v", fans)
	}
}

// readings is a load whose samples are given, as if it had run.
func readings(rest float64, samples ...Sample) *Warming {
	w := &Warming{}
	w.heat.Rest, w.heat.Peak = rest, rest
	for at, sample := range samples {
		sample.At = time.Duration(at+1) * time.Second
		w.heat.Samples = append(w.heat.Samples, sample)
		if sample.Hot > w.heat.Peak {
			w.heat.Peak, w.peakAt = sample.Hot, at
		}
	}
	return w
}

func TestAFanIsSeenByTheTemperatureItBringsDownUnderSteadyWork(t *testing.T) {
	climb := []Sample{{Hot: 50, Rate: 1}, {Hot: 56, Rate: 1}, {Hot: 60, Rate: 1}, {Hot: 63, Rate: 1}, {Hot: 65, Rate: 1}, {Hot: 67, Rate: 1}, {Hot: 68, Rate: 1}}
	with := func(rate float64, hot ...float64) *Warming {
		samples := append([]Sample(nil), climb...)
		for _, degrees := range hot {
			samples = append(samples, Sample{Hot: degrees, Rate: rate})
		}
		return readings(38, samples...)
	}
	// Still climbing, or level: no fan yet
	if kick := readings(38, climb...).kicked(); kick != nil {
		t.Errorf("a fan seen while it climbs: %+v", kick)
	}
	if kick := with(1, 67.5, 67.8, 67.6, 67.9).kicked(); kick != nil {
		t.Errorf("a fan seen in a level temperature: %+v", kick)
	}
	// It falls and stays down while the work goes on as before: the fan
	kick := with(1, 66, 63, 62, 62.4).kicked()
	if kick == nil || kick.From != 68 || kick.To > 63 || kick.At != 7*time.Second {
		t.Fatalf("the fan wasn't seen: %+v", kick)
	}
	// It falls because the processor was slowed down: that is throttling, not a fan
	if kick := with(0.6, 66, 63, 62, 62.4).kicked(); kick != nil {
		t.Errorf("throttling taken for a fan: %+v", kick)
	}
	// One low reading isn't a fall
	if kick := with(1, 63).kicked(); kick != nil {
		t.Errorf("one reading taken for a fan: %+v", kick)
	}
	// A machine that never got warm has nothing to cool
	if kick := readings(38, Sample{Hot: 41, Rate: 1}, Sample{Hot: 42, Rate: 1}, Sample{Hot: 38, Rate: 1}, Sample{Hot: 38, Rate: 1}, Sample{Hot: 38, Rate: 1}, Sample{Hot: 38, Rate: 1}).kicked(); kick != nil {
		t.Errorf("a fan seen on a cool machine: %+v", kick)
	}
}

// thermal is a machine with two sensors, one of which can't be read, and a way to set the other.
func thermal(t *testing.T, degrees int) (machine.System, func(int)) {
	root := t.TempDir()
	set := func(path, text string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
	}
	set("sys/class/thermal/thermal_zone0/temp", "31000\n")
	// A sensor that can't be read, as on a tree that names one the hardware lacks
	set("sys/class/thermal/thermal_zone2/temp", "-274000\n")
	heat := func(degrees int) { set("sys/class/thermal/thermal_zone1/temp", strconv.Itoa(degrees*1000)+"\n") }
	heat(degrees)
	return machine.Fixture(root), heat
}

func TestTheTemperatureTestRunsThroughByItself(t *testing.T) {
	beforePace, beforeLoad, beforeCool := pace, MaxLoad, CoolFirst
	pace, MaxLoad, CoolFirst = 5*time.Millisecond, 60*time.Millisecond, 40*time.Millisecond
	defer func() { pace, MaxLoad, CoolFirst = beforePace, beforeLoad, beforeCool }()

	sys, heat := thermal(t, 41)
	var said []string
	found := Warm(sys, "", func(line string) { said = append(said, line) })
	if found.Sensors != 2 || found.Rest != 41 || found.Peak != 41 || found.Fan != "" || len(found.Samples) < 2 || found.Loaded < MaxLoad {
		t.Errorf("found %+v", found)
	}
	if len(said) == 0 || !strings.Contains(said[len(said)-1], "41 °C") {
		t.Errorf("said %q", said)
	}
	// A machine that is warm already is given time to cool, and the test starts all the same
	heat(72)
	said = nil
	began := time.Now()
	if found = Warm(sys, "", func(line string) { said = append(said, line) }); found.Rest != 72 || time.Since(began) < CoolFirst {
		t.Errorf("warm at the start: %+v after %s", found, time.Since(began))
	}
	if !strings.Contains(strings.Join(said, "\n"), "letting it cool first: 72 °C") {
		t.Errorf("said %q", said)
	}
	if degrees, cool := Resting(sys); degrees != 72 || cool {
		t.Errorf("resting %v, %v", degrees, cool)
	}

	// The owner's word is kept with the reading it came at, and the load ends when told to
	w := StartWarming(sys, "")
	w.Step()
	w.Heard()
	w.Heard()
	w.Stop()
	w.Stop()
	if found = w.Heat(); found.Heard == nil || found.Heard.Hot != 72 || found.Loaded <= 0 {
		t.Errorf("heard: %+v", found)
	}

	// A processor at 96 °C is at work, not in danger: the load goes on. It ends only when a
	// sensor comes close to the temperature its own shutdown is set at
	file := func(root, path, text string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
	}
	root := t.TempDir()
	file(root, "sys/class/thermal/thermal_zone0/temp", "96000\n")
	file(root, "sys/class/thermal/thermal_zone0/trip_point_0_type", "critical\n")
	file(root, "sys/class/thermal/thermal_zone0/trip_point_0_temp", "115000\n")
	// A cooler part of the machine with a lower limit of its own
	file(root, "sys/class/thermal/thermal_zone1/temp", "48000\n")
	file(root, "sys/class/thermal/thermal_zone1/trip_point_0_type", "passive\n")
	file(root, "sys/class/thermal/thermal_zone1/trip_point_0_temp", "50000\n")
	file(root, "sys/class/thermal/thermal_zone1/trip_point_1_type", "critical\n")
	file(root, "sys/class/thermal/thermal_zone1/trip_point_1_temp", "70000\n")
	hotMachine := machine.Fixture(root)
	w = StartWarming(hotMachine, "")
	if _, over := w.Step(); over || w.Heat().TooHot {
		t.Errorf("the load ended at 96 °C with the shutdown at 115: %+v", w.Heat())
	}
	file(root, "sys/class/thermal/thermal_zone0/temp", "108000\n")
	if _, over := w.Step(); !over || !w.Heat().TooHot {
		t.Errorf("the load went on at 108 °C with the shutdown at 115")
	}
	w.Stop()
	file(root, "sys/class/thermal/thermal_zone0/temp", "96000\n")
	file(root, "sys/class/thermal/thermal_zone1/temp", "63000\n")
	w = StartWarming(hotMachine, "")
	if _, over := w.Step(); !over {
		t.Errorf("the load went on with a cooler part at 63 °C and its shutdown at 70")
	}
	w.Stop()
	if fans := one(t, w.Heat().Report(), "fans"); !strings.Contains(fans.Evidence, "close to the temperature the computer shuts down at") {
		t.Errorf("why it ended isn't said: %+v", fans)
	}

	// What the battery gives is what the computer draws only with no charger in
	power := func(status, online string) float64 {
		root := t.TempDir()
		for path, text := range map[string]string{"sys/class/power_supply/bat/power_now": "-31793000\n", "sys/class/power_supply/bat/status": status + "\n", "sys/class/power_supply/usb/online": online + "\n"} {
			os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
			os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
		}
		return watts(machine.Fixture(root))
	}
	if got := power("Discharging", "0"); got < 31.7 || got > 31.9 {
		t.Errorf("on battery: %v W", got)
	}
	if got := power("Discharging", "1"); got != 0 {
		t.Errorf("with a weak charger in: %v W", got)
	}
	if got := power("Charging", "1"); got != 0 {
		t.Errorf("charging: %v W", got)
	}

	// Where there is a Chromium, the GPU works too, in a profile of its own, and nothing of
	// it is left behind when the load ends
	dir := t.TempDir()
	started := make(chan []string, 1)
	sys.Command = func(name string, args ...string) (string, error) {
		if name == "which" && args[0] == "chromium" {
			return "/usr/bin/chromium", nil
		}
		if name == "chromium" {
			// The page is there for as long as the browser runs
			page, _ := os.ReadFile(strings.TrimPrefix(args[len(args)-1], "file://"))
			started <- append(args, string(page))
		}
		return "", os.ErrNotExist
	}
	w = StartWarming(sys, dir)
	select {
	case args := <-started:
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "drawArrays") || !strings.Contains(joined, "--user-data-dir="+dir) || !strings.Contains(joined, "--headless=new") {
			t.Errorf("the GPU's load: %s", joined)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the GPU wasn't put to work")
	}
	w.Stop()
	if !w.Heat().Graphics {
		t.Error("the GPU's part isn't noted")
	}
	for range 100 {
		if left, _ := os.ReadDir(dir); len(left) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("the GPU's page was left behind")
}

func TestOnlyTheTestsThisMachineCanRunAreOffered(t *testing.T) {
	names := func(tests []Test) string {
		var out []string
		for _, test := range tests {
			out = append(out, test.Name)
			if test.Doing == "" || test.About == "" || test.Nothing == "" || len(test.Checks) == 0 || test.Run == nil {
				t.Errorf("%s is missing a part: %+v", test.Name, test)
			}
		}
		return strings.Join(out, ", ")
	}
	everything := with(t, map[string]string{"vulkaninfo": "", "chromium": "", "iw": "", "bluetoothctl": "", "ffmpeg": ""})
	if got := names(Tests(everything, "wlan0", "hci0", t.TempDir())); got != "Graphics, Browser, Wireless, Video decoding, Temperatures" {
		t.Errorf("with every tool: %s", got)
	}
	// A bare system still has its sensors to read
	if got := names(Tests(with(t, nil), "wlan0", "hci0", t.TempDir())); got != "Temperatures" {
		t.Errorf("with no tools: %s", got)
	}
	// The tools without a radio to use them on
	if got := names(Tests(with(t, map[string]string{"iw": "", "bluetoothctl": ""}), "", "", t.TempDir())); got != "Temperatures" {
		t.Errorf("with no radios: %s", got)
	}
}
