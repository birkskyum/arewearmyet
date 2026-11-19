package report

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestALogIsCleanedOfWhatNamesTheOwnerLineByLine(t *testing.T) {
	log := strings.Join([]string{
		"[    0.000000] Linux version 7.3.0-59-omarchy (birkbuild@yogabox)",
		"[  812.4] wlan0: authenticate with aa:bb:cc:dd:ee:ff",
		"[  813.0] [UFW BLOCK] IN=wlan0 OUT= MAC=01:00:5e:00:00:fb:aa:bb:cc:dd:ee:ff SRC=192.168.1.20 DST=224.0.0.251 LEN=73",
		"[  813.1] usb 1-1: SerialNumber: 0123456789AB",
		"[  814.0] systemd[1]: Hostname set to <yogabox>.",
		"[  815.0] audit: exe=/home/margrethe/.local/bin/tool uid=1000 user margrethe",
		"[  816.0] tailscale0: 100.113.157.34 up, peer 10.0.0.7, gateway 172.20.1.1",
		"[  817.0] Chromium 153.0.8010.36, alsa-ucm-conf 1.2.16.1, firmware 172.4.0.2 kept",
	}, "\n")
	clean := ScrubLog(log, "yogabox", "margrethe")
	for _, personal := range []string{"aa:bb:cc", "192.168.1.20", "224.0.0.251", "0123456789AB", "yogabox", "margrethe", "100.113.157.34", "10.0.0.7", "172.20.1.1"} {
		if strings.Contains(clean, personal) {
			t.Errorf("%q is still in the log:\n%s", personal, clean)
		}
	}
	// What a reader needs stays: the lines, the versions, what happened
	for _, kept := range []string{"Linux version 7.3.0-59-omarchy", "wlan0: authenticate with xx:xx:xx:xx:xx:xx", "SRC=[removed] DST=[removed] LEN=73", "Hostname set to <this-computer>", "/home/user/.local/bin/tool", "Chromium 153.0.8010.36, alsa-ucm-conf 1.2.16.1, firmware 172.4.0.2 kept"} {
		if !strings.Contains(clean, kept) {
			t.Errorf("the log lost %q:\n%s", kept, clean)
		}
	}
	if strings.Count(clean, "\n") != 7 {
		t.Errorf("the lines ran together:\n%s", clean)
	}
	// A host called by an everyday word doesn't take that word out of every line
	if got := ScrubLog("omarchy kernel: usb 1-1 attached to user port", "omarchy", "user"); got != "omarchy kernel: usb 1-1 attached to user port" {
		t.Errorf("everyday names were replaced: %s", got)
	}
	// Of a log too long to send, the end is kept, from the start of a line
	long := strings.Repeat("an early line of the boot\n", 20000) + "the last line, where it broke"
	if got := ScrubLog(long, "", ""); len(got) > LogSize || !strings.HasSuffix(got, "the last line, where it broke") || !strings.HasPrefix(got, "an early line") {
		t.Errorf("a long log: %d bytes, starting %q", len(got), got[:30])
	}
}

func TestALogIsCleanedOfWhatNamesTheDiskTheInstallAndTheDevicesAround(t *testing.T) {
	log := strings.Join([]string{
		"Kernel command line: cryptdevice=PARTUUID=3069aaaa-1111-2222-3333-444455556666:root root=/dev/mapper/root rw",
		"BTRFS info (device dm-0): first mount of filesystem a504bbbb-1111-2222-3333-444455556666",
		"Kernel command line: root=PARTUUID=9f8e7d6c-02 rw",
		"systemd-journald[775]: File /var/log/journal/6703a82ab8ad4e0a975e10b2a27fa6e1/system.journal corrupted or uncleanly shut down",
		"r8152 2-1:1.0 enx00e04c680001: renamed from eth0",
		"systemd[1]: Expecting device /dev/disk/by-uuid/1AEE-4C7F...",
		"systemd[1]: Found device /dev/disk/by-id/nvme-SAMSUNG_MZVL21T0_S676NX0T123456-part2 then by-label/EFI",
		`systemd[1]: dev-disk-by\x2duuid-1AEE\x2d4C7F.device: Job timed out`,
		"input: Margrethes Tastatur as /devices/virtual/misc/uhid/0005:004C:0267.0008/input/input25",
		"input: Alice's AirPods (AVRCP) as /devices/virtual/input/input23",
		"apple 0005:05AC:0267.0008: input,hidraw5: BLUETOOTH HID v0.67 Keyboard [Kontorets tastatur] on xx",
		"usb 3-1: Product: Alice's iPhone",
		"wireguard: wg0: Handshake for peer 3 (203.0.113.5:51820) did not complete",
		"wireguard: wg0: Sending keepalive to peer 4 ([2001:db8::7]:51820)",
		"systemd[1]: Started Session 2 of User Dronning Ingrid.",
		"input: hid-over-i2c 048D:8987 Keyboard as /devices/platform/soc@0/bc0000.geniqup/b80000.i2c/i2c-1/1-003a/0018:048D:8987.0001/input/input2",
		"[mem 0x7a300000-0x7a3fffff]: assigned, firmware 172.4.0.2 kept, the queen will reign",
	}, "\n")
	clean := ScrubLog(log, "yogabox", "margrethe", "Dronning", "Ingrid", "Will")
	for _, personal := range []string{"3069aaaa", "a504bbbb", "9f8e7d6c", "6703a82a", "00e04c680001", "1AEE", "S676NX0T123456", "Margrethes", "Alice", "Kontorets", "203.0.113.5", "2001:db8", "Dronning", "Ingrid"} {
		if strings.Contains(clean, personal) {
			t.Errorf("%q is still in the log:\n%s", personal, clean)
		}
	}
	// The lines stay readable, and what tells one device or version from another is kept
	for _, kept := range []string{
		"cryptdevice=PARTUUID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx:root root=/dev/mapper/root rw", "root=PARTUUID=xxxxxxxx-xx rw", "enxxxxxxxxxxxxx: renamed from eth0", "Expecting device /dev/disk/by-uuid/[removed]", "Found device /dev/disk/by-id/[removed] then by-label/EFI", `dev-disk-by\x2duuid-[removed]`,
		"input: [name removed] as /devices/virtual/misc/uhid/", "input: [name removed] (AVRCP) as", "Keyboard [name removed] on xx", "Product: Someone's iPhone",
		"peer 3 (x.x.x.x:51820)", "peer 4 ([removed]:51820)", "Session 2 of User user user.",
		"input: hid-over-i2c 048D:8987 Keyboard as /devices/platform", "[mem 0x7a300000-0x7a3fffff]: assigned, firmware 172.4.0.2 kept, the queen will reign",
	} {
		if !strings.Contains(clean, kept) {
			t.Errorf("the log lost %q:\n%s", kept, clean)
		}
	}
	// A note's own words keep their possessives, and lose the same IDs
	if got := ScrubText("Lenovo's dock, disk a504bbbb-1111-2222-3333-444455556666.", "yogabox", "margrethe"); got != "Lenovo's dock, disk xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx." {
		t.Errorf("a note's text: %s", got)
	}
}

func TestANoteIsSentWithItsLogsAndAnAnswerGoesToItsNote(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var path, token string
	var got Note
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, token = r.URL.Path, r.Header.Get("x-machine-token")
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "refuse me") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "title must say in a line what the note is about."}`))
			return
		}
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"note": "NOTE-AAAAAAAAAA", "page": "https://arewearmyet.com/note?id=NOTE-AAAAAAAAAA"}`))
	}))
	defer server.Close()

	note := Note{Model: "lenovo-yoga-slim-7x-14q8x9", Check: "suspend", Kind: "problem", Title: "Freezes on wake with a dock", Body: "Close the lid, plug the dock in, open the lid.",
		Links: []string{"https://lore.kernel.org/x"}, Logs: []Log{{Name: "kernel.log", Text: "PM: suspend entry"}}, Kernel: "7.3.0-59-omarchy"}
	sent, err := SendNote(server.URL, note, "")
	if err != nil || sent.Note != "NOTE-AAAAAAAAAA" || path != "/api/notes" || len(token) < 32 {
		t.Fatalf("sent %+v, %v, to %s with a token of %d", sent, err, path, len(token))
	}
	if got.Kind != "problem" || got.Check != "suspend" || len(got.Logs) != 1 || got.Logs[0].Name != "kernel.log" || got.Links[0] != "https://lore.kernel.org/x" {
		t.Errorf("the site got %+v", got)
	}
	// An answer goes to the note it answers, from the same machine
	first := token
	if _, err = SendNote(server.URL, Note{Body: "Same here on another dock."}, "NOTE-AAAAAAAAAA"); err != nil || path != "/api/notes/NOTE-AAAAAAAAAA/replies" || token != first {
		t.Errorf("the answer went to %s: %v", path, err)
	}
	if _, err = Solve(server.URL, "NOTE-AAAAAAAAAA", true); err != nil || path != "/api/notes/NOTE-AAAAAAAAAA/status" {
		t.Errorf("solved went to %s: %v", path, err)
	}
	// What the site refuses is said in its own words
	if _, err = SendNote(server.URL, Note{Body: "refuse me"}, ""); err == nil || !strings.Contains(err.Error(), "title must say") {
		t.Errorf("a refusal: %v", err)
	}
}
