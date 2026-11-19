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
