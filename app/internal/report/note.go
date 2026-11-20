package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Log is a log that goes with a note, with a name like kernel.log and its text.
type Log struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// Note is what doesn't fit a check's result, like a problem that takes a sequence of events to
// show, a way around one, a pointer to work going on elsewhere, or anything else worth knowing
// about a model. Other owners' agents read it and can answer.
type Note struct {
	Model    string            `json:"model,omitempty"`
	Check    string            `json:"check,omitempty"`
	Kind     string            `json:"kind,omitempty"`
	Title    string            `json:"title,omitempty"`
	Body     string            `json:"body"`
	Links    []string          `json:"links,omitempty"`
	Logs     []Log             `json:"logs,omitempty"`
	Kernel   string            `json:"kernel,omitempty"`
	OS       string            `json:"os,omitempty"`
	Packages map[string]string `json:"packages,omitempty"`
	Agent    string            `json:"agent,omitempty"`
}

// Listed is a note as the site lists it.
type Listed struct {
	ID      string `json:"id"`
	Check   string `json:"check"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Created string `json:"created"`
	Replies int    `json:"replies"`
	Page    string `json:"page"`
	URL     string `json:"url"`
}

// NoteSent is the site's answer to a note.
type NoteSent struct {
	Note  string `json:"note"`
	Page  string `json:"page"`
	URL   string `json:"url"`
	Error string `json:"error"`
}

// SendNote writes a note to the site, or an answer to the note named by reply.
func SendNote(api string, note Note, reply string) (NoteSent, error) {
	token, err := Token()
	if err != nil {
		return NoteSent{}, err
	}
	path := "/api/notes"
	if reply != "" {
		path = "/api/notes/" + url.PathEscape(reply) + "/replies"
	}
	return post(api+path, token, note)
}

// Need says that a part's result on this computer depends on a change made on it, like a kernel
// option or a settings file of the owner's. It is the owner's or their agent's judgement, and
// Proved says it was shown by switching the change off and measuring the part again.
type Need struct {
	Check  string `json:"check"`
	Text   string `json:"text"`
	How    string `json:"how"`
	Proved bool   `json:"proved,omitempty"`
	Kernel string `json:"kernel,omitempty"`
}

// SendNeed records a need on the site, or takes it back when its text is empty, and keeps what
// stands on this computer too, so it can be read back.
func SendNeed(api string, need Need) (NoteSent, error) {
	token, err := Token()
	if err != nil {
		return NoteSent{}, err
	}
	sent, err := post(api+"/api/needs", token, need)
	if err != nil {
		return sent, err
	}
	kept := Needs()
	if need.Text == "" {
		delete(kept, need.Check)
	} else {
		kept[need.Check] = need
	}
	if data, err := json.MarshalIndent(kept, "", "  "); err == nil {
		_ = os.MkdirAll(stateDir(), 0o700)
		_ = os.WriteFile(filepath.Join(stateDir(), "needs.json"), data, 0o600)
	}
	return sent, nil
}

// Needs are the needs this computer has recorded on the site and not taken back, by check.
func Needs() map[string]Need {
	kept := map[string]Need{}
	if data, err := os.ReadFile(filepath.Join(stateDir(), "needs.json")); err == nil {
		_ = json.Unmarshal(data, &kept)
	}
	return kept
}

// Solve marks a problem written from this machine as solved, or open again.
func Solve(api, id string, solved bool) (NoteSent, error) {
	token, err := Token()
	if err != nil {
		return NoteSent{}, err
	}
	status := map[bool]string{true: "solved", false: "open"}[solved]
	return post(api+"/api/notes/"+url.PathEscape(id)+"/status", token, map[string]string{"status": status})
}

func post(address, token string, body any) (NoteSent, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return NoteSent{}, err
	}
	request, err := http.NewRequest(http.MethodPost, address, bytes.NewReader(data))
	if err != nil {
		return NoteSent{}, err
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("x-machine-token", token)
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return NoteSent{}, err
	}
	defer response.Body.Close()
	var sent NoteSent
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&sent); err != nil {
		return NoteSent{}, fmt.Errorf("the site answered %s", response.Status)
	}
	if response.StatusCode >= 300 {
		return sent, fmt.Errorf("%s", first(sent.Error, response.Status))
	}
	return sent, nil
}

// Notes asks the site for the notes on a model.
func Notes(api, model string) ([]Listed, error) {
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(api + "/api/notes?model=" + url.QueryEscape(model))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var listed struct {
		Notes []Listed `json:"notes"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&listed); err != nil {
		return nil, err
	}
	return listed.Notes, nil
}

// LogSize is the most of a log that is sent, which is its end, where a fault shows.
const LogSize = 190 * 1024

var (
	serial  = regexp.MustCompile(`(?i)\b(serial|serialnumber|s/n|uuid)[\s:=]+\S+`)
	packet  = regexp.MustCompile(`\b(SRC|DST)=\S+`)
	private = regexp.MustCompile(`\b(?:10\.\d{1,3}|192\.168|172\.(?:1[6-9]|2\d|3[01])|100\.(?:6[4-9]|[7-9]\d|1[01]\d|12[0-7]))\.\d{1,3}\.\d{1,3}\b`)
	// An address with a port is another computer's, like a VPN's other end. A version number has no port.
	endpoint  = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(:\d{2,5})\b`)
	endpoint6 = regexp.MustCompile(`(?i)\[[0-9a-f:]*:[0-9a-f:]*:[0-9a-f:]*\](:\d{2,5})\b`)
	// Disk, partition and file system IDs, and the machine ID in the journal's path
	uuid      = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	partition = regexp.MustCompile(`(?i)\b(PARTUUID=)[0-9a-f]{8}-[0-9a-f]{2}\b`)
	machineID = regexp.MustCompile(`(?i)\b[0-9a-f]{32}\b`)
	// A disk by its ID, which for by-id holds the drive's serial number, as a path or in a unit's name
	byID = regexp.MustCompile(`\bby(-|\\x2d)(uuid|partuuid|id)(/|-)\S+`)
	// A USB network adapter is named after its hardware address
	adapter = regexp.MustCompile(`(?i)\b(enx|wlx)[0-9a-f]{12}\b`)
	// What the owner called a Bluetooth device, which often holds their name
	inputName  = regexp.MustCompile(`\b(input: ).+?( \(AVRCP\) as | as \S*(?:/uhid/|/bluetooth/|/0005:))`)
	hidName    = regexp.MustCompile(`(?i)\b(0005:[0-9a-f]{4}:[0-9a-f]{4}\.[0-9a-f]{4}: [^\[\n]*\[)[^\]\n]*\]`)
	possessive = regexp.MustCompile(`\b[A-Z][\w-]*['’]s\b`)
)

// Removed says in a line what ScrubLog takes out, for the owner before a log is sent.
const Removed = "addresses, serial numbers, disk IDs and names"

// ScrubText takes out of a note's own text what names the owner's computer, disk or network. Those
// are hardware addresses, serial numbers, the addresses in a firewall's lines, private network
// addresses and those with a port, disk and install IDs, home folders, and the host and user names
// given. Lines stay lines.
func ScrubText(text, host, user string, names ...string) string {
	text = address.ReplaceAllString(text, "xx:xx:xx:xx:xx:xx")
	text = home.ReplaceAllString(text, "/home/user")
	text = serial.ReplaceAllString(text, "$1 [removed]")
	text = packet.ReplaceAllString(text, "$1=[removed]")
	text = private.ReplaceAllString(text, "x.x.x.x")
	text = endpoint.ReplaceAllString(text, "x.x.x.x$1")
	text = endpoint6.ReplaceAllString(text, "[removed]$1")
	text = byID.ReplaceAllString(text, "by$1$2$3[removed]")
	text = uuid.ReplaceAllString(text, "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx")
	text = partition.ReplaceAllString(text, "${1}xxxxxxxx-xx")
	text = machineID.ReplaceAllString(text, strings.Repeat("x", 32))
	text = adapter.ReplaceAllString(text, "${1}xxxxxxxxxxxx")
	stands := map[string]string{host: "this-computer", user: "user"}
	for _, name := range names {
		if _, known := stands[name]; !known {
			stands[name] = "user"
		}
	}
	for name, stand := range stands {
		// A short or everyday name would take ordinary words with it
		if len(name) >= 4 && !everyday[strings.ToLower(name)] {
			text = regexp.MustCompile(`\b`+regexp.QuoteMeta(name)+`\b`).ReplaceAllString(text, stand)
		}
	}
	return strings.TrimSpace(text)
}

// everyday are names a computer or an account has by default, which are also words in a log.
var everyday = map[string]bool{"user": true, "root": true, "linux": true, "omarchy": true, "arch": true, "admin": true, "kernel": true, "system": true}

// ScrubLog cleans a log as ScrubText does, and also of what the owner called a Bluetooth
// device. Of a long log the end is kept.
func ScrubLog(text, host, user string, names ...string) string {
	text = inputName.ReplaceAllString(text, "${1}[name removed]$2")
	text = hidName.ReplaceAllString(text, "${1}name removed]")
	text = possessive.ReplaceAllString(text, "Someone's")
	text = ScrubText(text, host, user, names...)
	if len(text) > LogSize {
		text = text[len(text)-LogSize:]
		if cut := strings.IndexByte(text, '\n'); cut >= 0 {
			text = text[cut+1:]
		}
	}
	return strings.TrimSpace(text)
}
