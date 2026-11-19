package deep

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	fans := one(t, Heat{Sensors: 59, Before: 52, After: 71}.Report(), "fans")
	if fans.State != report.Partial || fans.Note != FanUnread || !strings.Contains(fans.Evidence, "59 temperature sensors; the hottest went from 52 to 71 °C") {
		t.Errorf("no fan sensor: %+v", fans)
	}
	if fans = one(t, Heat{Sensors: 59, Before: 52, After: 52.5}.Report(), "fans"); fans.State != report.Partial || fans.Note == FanUnread || !strings.Contains(fans.Note, "barely moved") {
		t.Errorf("sensors that don't move: %+v", fans)
	}
	if fans = one(t, Heat{Sensors: 8, Before: 45, After: 68, Fan: "thinkpad", FanBefore: 0, FanAfter: 3200}.Report(), "fans"); fans.State != report.Works || !strings.Contains(fans.Evidence, "from 0 to 3200 rpm") {
		t.Errorf("a fan that speeds up: %+v", fans)
	}
	if fans = one(t, Heat{Sensors: 8, Before: 45, After: 68, Fan: "thinkpad", FanBefore: 2100, FanAfter: 2100}.Report(), "fans"); fans.State != report.Partial || !strings.Contains(fans.Note, "didn't rise") {
		t.Errorf("a fan that stays: %+v", fans)
	}
}

func TestWarmReadsTheSensorsAroundTheLoad(t *testing.T) {
	root := t.TempDir()
	set := func(path, text string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
	}
	set("sys/class/thermal/thermal_zone0/temp", "41000\n")
	set("sys/class/thermal/thermal_zone1/temp", "52500\n")
	// A sensor that can't be read, as on a tree that names one the hardware lacks
	set("sys/class/thermal/thermal_zone2/temp", "-274000\n")
	set("sys/class/hwmon/hwmon3/name", "pwmfan\n")
	set("sys/class/hwmon/hwmon3/fan1_input", "1800\n")
	heat := Warm(machine.Fixture(root), 20*time.Millisecond)
	if heat.Sensors != 2 || heat.Before != 52.5 || heat.After != 52.5 || heat.Fan != "pwmfan" || heat.FanBefore != 1800 {
		t.Errorf("read %+v", heat)
	}
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
