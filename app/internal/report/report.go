// Package report holds what a run finds and sends it to arewearmyet.com.
package report

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
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

// Site is where runs go and model pages live.
const Site = "https://arewearmyet.com"

// A check's state, as the site's /api/checks lists them.
const (
	Works    = "works"
	Partial  = "partial"
	Broken   = "broken"
	Untested = "untested"
	Absent   = "absent"
)

// How a result was found.
const (
	Measured = "measured" // the app saw it
	Owner    = "owner"    // the owner saw or heard it
)

// Result is one check's outcome.
type Result struct {
	Check    string `json:"check"`
	State    string `json:"state"`
	How      string `json:"how"`
	Evidence string `json:"evidence,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Run is what's sent: the machine, its software and the results. It holds no serial
// numbers, addresses or names.
type Run struct {
	Mode       string            `json:"mode"`
	Model      string            `json:"model,omitempty"`
	Agent      string            `json:"agent,omitempty"`
	Compatible string            `json:"compatible"`
	Kernel     string            `json:"kernel"`
	OS         string            `json:"os"`
	Hardware   map[string]string `json:"hardware"`
	Chids      []string          `json:"chids"`
	Packages   map[string]string `json:"packages"`
	Results    []Result          `json:"results"`
	// Quick is what the quick check found, by check, kept to tell its results from those of
	// the tests that look closer. It isn't sent.
	Quick map[string]Result `json:"-"`
}

// MarkQuick notes the results the run has now as the quick check's.
func (r *Run) MarkQuick() {
	r.Quick = map[string]Result{}
	for _, result := range r.Results {
		r.Quick[result.Check] = result
	}
}

// Shallow says whether a result is the quick check's own, which only looks at whether the
// hardware is there and its driver came up, and not that of a test that used it.
func (r *Run) Shallow(result Result) bool {
	quick, ok := r.Quick[result.Check]
	return ok && quick == result
}

var (
	address = regexp.MustCompile(`(?i)\b([0-9a-f]{2}[:-]){5}[0-9a-f]{2}\b`)
	home    = regexp.MustCompile(`/home/[^/\s]+`)
)

// Scrub takes addresses and home folders out of text and keeps it short.
func Scrub(text string, max int) string {
	text = address.ReplaceAllString(text, "xx:xx:xx:xx:xx:xx")
	text = home.ReplaceAllString(text, "/home/user")
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > max {
		text = text[:max]
	}
	return text
}

// Set stores a check's result, replacing an earlier one for the same check.
func (r *Run) Set(check, state, how, evidence, note string) {
	result := Result{Check: check, State: state, How: how, Evidence: Scrub(evidence, 380), Note: Scrub(note, 580)}
	for i := range r.Results {
		if r.Results[i].Check == check {
			r.Results[i] = result
			return
		}
	}
	r.Results = append(r.Results, result)
}

// Get returns a check's result, if the run has one.
func (r *Run) Get(check string) (Result, bool) {
	for _, result := range r.Results {
		if result.Check == check {
			return result, true
		}
	}
	return Result{}, false
}

// State is a check's state, or "" when the run has no result for it.
func (r *Run) State(check string) string {
	result, _ := r.Get(check)
	return result.State
}

// JSON is the run as it is sent.
func (r *Run) JSON() []byte {
	if r.Hardware == nil {
		r.Hardware = map[string]string{}
	}
	if r.Packages == nil {
		r.Packages = map[string]string{}
	}
	if r.Chids == nil {
		r.Chids = []string{}
	}
	if r.Results == nil {
		r.Results = []Result{}
	}
	data, _ := json.Marshal(r)
	return data
}

// Sent is the site's answer to a run.
type Sent struct {
	Run   string `json:"run"`
	Model string `json:"model"`
	Page  string `json:"page"`
	URL   string `json:"url"`
	Error string `json:"error"`
}

func stateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "arewearmyet")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "arewearmyet")
}

// Token is this machine's random token, made on first use. The site keeps only its hash, so
// the machine's later runs replace its earlier ones without any account.
func Token() (string, error) {
	file := filepath.Join(stateDir(), "token")
	if data, err := os.ReadFile(file); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return strings.TrimSpace(string(data)), nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return "", err
	}
	return token, os.WriteFile(file, []byte(token), 0o600)
}

// Send posts a run and remembers its id for later.
func Send(api string, run *Run) (Sent, error) {
	token, err := Token()
	if err != nil {
		return Sent{}, err
	}
	request, err := http.NewRequest(http.MethodPost, api+"/api/runs", bytes.NewReader(run.JSON()))
	if err != nil {
		return Sent{}, err
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("x-machine-token", token)
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return Sent{}, err
	}
	defer response.Body.Close()
	var sent Sent
	if err := json.NewDecoder(response.Body).Decode(&sent); err != nil {
		return Sent{}, fmt.Errorf("the site answered %s", response.Status)
	}
	if response.StatusCode >= 300 {
		return sent, fmt.Errorf("%s", sent.Error)
	}
	_ = os.WriteFile(filepath.Join(stateDir(), "last-run"), []byte(sent.Run+"\n"), 0o600)
	return sent, nil
}

// AddModelURL is the form that adds a machine the board doesn't know, with what identifies
// it filled in.
func AddModelURL(run *Run, id string) string {
	name := strings.TrimSpace(run.Hardware["maker"] + " " + first(run.Hardware["family"], run.Hardware["product"]))
	query := url.Values{}
	query.Set("template", "model.yml")
	query.Set("model", name)
	query.Set("model-number", run.Hardware["product"])
	query.Set("probe", id)
	if len(run.Chids) > 0 {
		query.Set("hwids", strings.ReplaceAll(strings.Join(run.Chids, "\n"), ":", " "))
	}
	return "https://github.com/birkskyum/arewearmyet/issues/new?" + query.Encode()
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// Model is what the board knows a machine as.
type Model struct {
	ID   string `json:"model"`
	Name string `json:"name"`
	Page string `json:"page"`
	// Family is the line a pull request for the machine opens with, like arm-dragon-msft.
	Family string `json:"family"`
	// Kernels is the linux-omarchy version on each of Omarchy's channels, as the site last saw them.
	Kernels map[string]string `json:"kernels"`
}

// Finding is what is written up on the site about one check on a model, by hand, from
// discussions and reports.
type Finding struct {
	State   string `json:"state"`
	Note    string `json:"note"`
	Version string `json:"version"`
	Checked string `json:"checked"`
	Source  string `json:"source"`
	// Basis is what the finding rests on: omarchy (or empty, the same), other-distro,
	// tree-author or maker-spec. Where says where it was seen, and Quote is the passage of the
	// source it rests on.
	Basis string `json:"basis"`
	Where string `json:"where"`
	Quote string `json:"quote"`
}

// OnOmarchy says whether a finding is about how the model does under Omarchy. One from
// another distro or from the device tree's author only shows that the part can work.
func (f Finding) OnOmarchy() bool {
	return f.Basis == "" || f.Basis == "omarchy" || f.Basis == "maker-spec"
}

// Findings asks the site what is written up about a model, by check. Only what is the
// model's own or holds across its chip family is there.
func Findings(api, model string) (map[string]Finding, error) {
	client := http.Client{Timeout: 6 * time.Second}
	response, err := client.Get(api + "/api/models/" + url.PathEscape(model))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var answer struct {
		Checks []struct {
			ID      string   `json:"id"`
			Finding *Finding `json:"finding"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&answer); err != nil {
		return nil, err
	}
	found := map[string]Finding{}
	for _, check := range answer.Checks {
		if check.Finding != nil {
			found[check.ID] = *check.Finding
		}
	}
	return found, nil
}

// Match asks the site which model a machine is, by its hardware IDs, its device tree's name
// and its product strings. It gives up after a few seconds: the answer is only for showing
// the owner their model's page, and nothing depends on it.
func Match(api string, run *Run) (Model, error) {
	query := url.Values{}
	for _, chid := range run.Chids {
		query.Add("chid", chid)
	}
	if run.Compatible != "" {
		query.Set("compatible", run.Compatible)
	}
	for field, key := range map[string]string{"product": "product", "sku": "sku", "family": "family", "vendor": "maker"} {
		if value := run.Hardware[key]; value != "" {
			query.Set(field, value)
		}
	}
	client := http.Client{Timeout: 4 * time.Second}
	response, err := client.Get(api + "/api/match?" + query.Encode())
	if err != nil {
		return Model{}, err
	}
	defer response.Body.Close()
	var model Model
	if err := json.NewDecoder(response.Body).Decode(&model); err != nil {
		return Model{}, err
	}
	return model, nil
}

// Check is what the site says about one check: what working means, and where to look when it doesn't.
type Check struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Works  string `json:"works"`
	Probe  string `json:"probe"`
	Person string `json:"person"`
	Debug  string `json:"debug"`
}

// Checks asks the site for its list of checks, by id.
func Checks(api string) (map[string]Check, error) {
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(api + "/api/checks")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var listed struct {
		Checks []Check `json:"checks"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&listed); err != nil {
		return nil, err
	}
	checks := map[string]Check{}
	for _, check := range listed.Checks {
		checks[check.ID] = check
	}
	return checks, nil
}
