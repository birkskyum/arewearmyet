package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Log is a log that goes with a note: a name like kernel.log and its text.
type Log struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// Note is what doesn't fit a check's result: a problem that takes a sequence of events to
// show, a way around one, a pointer to work going on elsewhere, or anything else worth
// knowing about a model. Other owners' agents read it and can answer.
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

// LogSize is the most of a log that is sent: its end, where a fault shows.
const LogSize = 190 * 1024

var (
	serial  = regexp.MustCompile(`(?i)\b(serial|serialnumber|s/n|uuid)[\s:=]+\S+`)
	packet  = regexp.MustCompile(`\b(SRC|DST)=\S+`)
	private = regexp.MustCompile(`\b(?:10\.\d{1,3}|192\.168|172\.(?:1[6-9]|2\d|3[01])|100\.(?:6[4-9]|[7-9]\d|1[01]\d|12[0-7]))\.\d{1,3}\.\d{1,3}\b`)
)

// ScrubLog takes out of a log what names the owner, their computer or their network:
// hardware addresses, serial numbers, the addresses in a firewall's lines, private network
// addresses, home folders, and the host and user names given. Lines stay lines, and of a long
// log the end is kept.
func ScrubLog(text, host, user string) string {
	text = address.ReplaceAllString(text, "xx:xx:xx:xx:xx:xx")
	text = home.ReplaceAllString(text, "/home/user")
	text = serial.ReplaceAllString(text, "$1 [removed]")
	text = packet.ReplaceAllString(text, "$1=[removed]")
	text = private.ReplaceAllString(text, "x.x.x.x")
	for name, stand := range map[string]string{host: "this-computer", user: "user"} {
		// A short or everyday name would take ordinary words with it
		if len(name) >= 4 && name != "user" && name != "root" && name != "linux" && name != "omarchy" {
			text = regexp.MustCompile(`\b`+regexp.QuoteMeta(name)+`\b`).ReplaceAllString(text, stand)
		}
	}
	if len(text) > LogSize {
		text = text[len(text)-LogSize:]
		if cut := strings.IndexByte(text, '\n'); cut >= 0 {
			text = text[cut+1:]
		}
	}
	return strings.TrimSpace(text)
}
