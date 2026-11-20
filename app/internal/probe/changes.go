package probe

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// An owner gets a part working with a settings file as often as with a kernel option, and nobody
// remembers every file they wrote. Whether a part works because of one takes reading the file, so
// that is left to the owner's agent. Changes finds the files for it. A file counts as the owner's
// when no package holds it and none of Omarchy's own scripts name it.
//
// None of this is sent. A file's name and what is in it can say what the owner owns, and an
// ordinary setting for a keyboard or a speaker looks the same here as a fix for the laptop.

// Changes is what the owner changed on the computer for its hardware, as far as its files show.
type Changes struct {
	// Sent is what a run names, the only part of this that leaves the computer
	Sent *report.Setup `json:"sent,omitempty"`
	// Settings are the settings files that are in no package and that Omarchy's scripts don't name
	Settings []SettingsFile `json:"settings,omitempty"`
	// Placed are firmware, sound profile and camera files that no package holds and Omarchy's
	// firmware copy didn't place
	Placed []string `json:"placed,omitempty"`
	// Altered are files of hardware packages changed since the package was installed, or the
	// package's name where pacman doesn't say which files
	Altered []string `json:"altered,omitempty"`
	// Packages come from no repository and hold kernel modules for the running kernel, firmware
	// or sound profiles
	Packages []string `json:"packages,omitempty"`
}

// SettingsFile is one settings file of the owner's, with what it does as far as the test reads it.
type SettingsFile struct {
	Path string   `json:"path"`
	Does []string `json:"does,omitempty"`
}

// Empty says whether nothing was found.
func (c Changes) Empty() bool {
	return c.Sent.Empty() && len(c.Settings) == 0 && len(c.Placed) == 0 && len(c.Altered) == 0 && len(c.Packages) == 0
}

// ownFiles are settings files Omarchy wrote in earlier versions, which its scripts no longer
// name, as data/kernel-options.yaml lists them.
var ownFiles = []string{"hid_apple.conf"}

// The folders whose files change what the hardware does, by what a file in them holds
var settingsFolders = []struct{ dir, holds string }{
	{"/etc/modprobe.d", "options"},
	{"/etc/modules-load.d", "modules"},
	{"/etc/mkinitcpio.conf.d", "image"},
	{"/etc/udev/rules.d", "rules"},
}

// Folders of sound settings, which hold folders of their own
var soundFolders = []string{"/etc/wireplumber", "/etc/pipewire"}

// Folders of files the hardware needs, where a file that no package holds was put there by hand
var hardwareFolders = []string{"/usr/lib/firmware", "/usr/share/alsa/ucm2", "/usr/share/libcamera"}

// Packages whose files an owner changes in place to get sound or a camera working
var alterable = []string{"alsa-ucm-conf", "wireplumber", "pipewire", "libcamera", "linux-firmware-qcom"}

// What Omarchy's own scripts and its firmware copy are, on a computer
var omarchyFolders = []string{"/usr/share/omarchy/install", "/usr/share/omarchy/bin", "/usr/share/omarchy/migrations", "/usr/share/omarchy/default", "/usr/share/omarchy/config"}

const (
	firmwareCopy     = "/usr/bin/qcom-firmware-extract"
	firmwareManifest = "/var/lib/omarchy/qcom-firmware/manifest"
)

var (
	imageModules = regexp.MustCompile(`(?m)^\s*MODULES\+?=\(([^)]*)\)`)
	ruleToken    = regexp.MustCompile(`([A-Z_]+)(?:\{([^}]*)\})?\s*(==|!=|\+=|-=|:=|=)\s*"((?:[^"\\]|\\.)*)"`)
	deviceKind   = regexp.MustCompile(`^[a-z0-9_\-]{1,30}$`)
	attribute    = regexp.MustCompile(`^[A-Za-z0-9_/.\-]{1,60}$`)
	envName      = regexp.MustCompile(`^[A-Z0-9_]{1,40}$`)
	soundKey     = regexp.MustCompile(`"?([a-z][a-z0-9_\-]*(?:\.[a-z0-9_\-]+)+)"?\s*=\s*(\S)`)
	// A sound setting that picks a device out says which devices the owner has
	soundMatch = regexp.MustCompile(`(^|\.)(name|nick|description|class|bus|serial|id|path|address|vendor|product|profile)$|\.(name|id)\.`)
	altered    = regexp.MustCompile(`(?m)^(\S+): \d+ [^,\n]*, ([1-9]\d*) `)
	// "warning: alsa-ucm-conf: /usr/share/alsa/ucm2/x.conf (Modification time mismatch)"
	alteredFile = regexp.MustCompile(`(?m)^\S+ (\S+): (/\S+) \(`)
)

// FindChanges reads what the owner changed for the hardware. It finds only what a run names where
// it can't ask which package a file belongs to.
func FindChanges(sys machine.System, run *report.Run) Changes {
	found := Changes{Sent: run.Setup, Packages: hardwarePackages(sys, foreignPackages(sys), run.Kernel)}
	var folders []string
	for _, folder := range settingsFolders {
		folders = append(folders, folder.dir)
	}
	folders = append(append(folders, soundFolders...), hardwareFolders...)
	var there []string
	for _, folder := range folders {
		if sys.IsDir(folder) {
			there = append(there, folder)
		}
	}
	if len(there) == 0 {
		return found
	}
	// Every package with a file in one of the folders, and then every file of those packages
	answer, _ := sys.RunFor(10*time.Second, "pacman", append([]string{"-Qqo"}, there...)...)
	holders := names(answer, 80)
	if len(holders) == 0 {
		return found
	}
	held := map[string]bool{}
	answer, _ = sys.RunFor(20*time.Second, "pacman", append([]string{"-Qql"}, holders...)...)
	for _, line := range strings.Split(answer, "\n") {
		if strings.HasPrefix(line, "/") {
			held[line] = true
		}
	}
	if len(held) == 0 {
		return found
	}

	found.Placed = placedFiles(sys, held)
	answer, _ = sys.RunFor(30*time.Second, "pacman", append([]string{"-Qkk"}, alterable...)...)
	named := map[string]bool{}
	for _, match := range alteredFile.FindAllStringSubmatch(answer, -1) {
		named[match[1]] = true
		found.Altered = add(found.Altered, match[2], nil, 40)
	}
	for _, match := range altered.FindAllStringSubmatch(answer, -1) {
		if !named[match[1]] {
			found.Altered = add(found.Altered, match[1], simpleName, 40)
		}
	}

	// The settings files that are nobody's package's, by the folder they are in
	type file struct{ path, holds string }
	var loose []file
	for _, folder := range settingsFolders {
		// Only files with the ending their reader takes are in use, so a copy kept beside one isn't read
		ending := ".conf"
		if folder.holds == "rules" {
			ending = ".rules"
		}
		for _, path := range sys.Glob(folder.dir + "/*" + ending) {
			if !held[path] && !sys.IsDir(path) {
				loose = append(loose, file{path, folder.holds})
			}
		}
	}
	sound := soundFolders
	if home, err := os.UserHomeDir(); err == nil && sys.Root == "" {
		sound = append(append([]string{}, sound...), home+"/.config/wireplumber", home+"/.config/pipewire")
	}
	for _, folder := range sound {
		for _, path := range sys.Files(folder, 200) {
			if !held[path] && strings.HasSuffix(path, ".conf") {
				loose = append(loose, file{path, "sound"})
			}
		}
	}
	if len(loose) == 0 {
		return found
	}
	paths := make([]string, 0, len(loose))
	for _, f := range loose {
		paths = append(paths, f.path)
	}
	fromOmarchy, known := omarchys(sys, paths)
	if !known {
		// Without Omarchy's scripts to look in, its own files can't be told from the owner's
		return found
	}
	for _, f := range loose {
		if fromOmarchy[base(f.path)] || len(found.Settings) >= 60 {
			continue
		}
		text := sys.ReadAll(f.path)
		var does []string
		switch f.holds {
		case "options":
			does = moduleSettings(text)
		case "modules":
			for _, line := range lines(text) {
				does = add(does, "loads kernel module "+strings.Fields(line)[0]+" at start", nil, 20)
			}
		case "image":
			for _, match := range imageModules.FindAllStringSubmatch(text, -1) {
				for _, name := range strings.Fields(match[1]) {
					does = add(does, "puts kernel module "+strings.TrimSuffix(name, "?")+" into the boot image", nil, 20)
				}
			}
		case "rules":
			for _, rule := range Rules(text) {
				does = add(does, ruleWords(rule), nil, 20)
			}
		case "sound":
			for _, key := range SoundSettings(text) {
				does = add(does, "sets the sound setting "+key, nil, 20)
			}
		}
		found.Settings = append(found.Settings, SettingsFile{Path: f.path, Does: does})
	}
	return found
}

// Lines says what was found in the words the test shows, a file a line.
func (c Changes) Lines() []string {
	var out []string
	for _, line := range c.Sent.Lines() {
		out = append(out, line+" (in the report)")
	}
	for _, name := range c.Packages {
		out = append(out, "package "+name+", from no repository")
	}
	for _, file := range c.Settings {
		line := file.Path
		if len(file.Does) > 0 {
			line += ", which " + strings.Join(file.Does, ", ")
		}
		out = append(out, line)
	}
	for _, path := range c.Placed {
		out = append(out, path+", which no package holds")
	}
	for _, what := range c.Altered {
		out = append(out, what+", changed since its package was installed")
	}
	return out
}

// ruleWords puts one of Rules into words, like "has a device rule for usb that sets power/control=on".
func ruleWords(rule string) string {
	kind, does, _ := strings.Cut(rule, " ")
	text := "has a device rule for " + kind
	if kind == "any" {
		text = "has a device rule"
	}
	verb, what, _ := strings.Cut(does, " ")
	switch verb {
	case "sets", "env":
		return text + " that sets " + what
	case "runs":
		return text + " that runs a program"
	case "permissions":
		return text + " that changes who may use it"
	}
	return text
}

// placedFiles are the firmware, sound profile and camera files that no package holds and that
// Omarchy's copy of the firmware from Windows didn't place.
func placedFiles(sys machine.System, held map[string]bool) []string {
	copied := map[string]bool{}
	for _, line := range lines(sys.ReadAll(firmwareManifest)) {
		copied[strings.Fields(line)[0]] = true
	}
	var placed []string
	for _, folder := range hardwareFolders {
		for _, path := range sys.Files(folder, 30000) {
			if held[path] {
				continue
			}
			if under, ok := strings.CutPrefix(path, "/usr/lib/firmware/updates/"); ok && (copied[under] || copied[base(path)]) {
				continue
			}
			placed = add(placed, path, nil, 60)
		}
	}
	return placed
}

// omarchys says which of the files Omarchy wrote, by whether its scripts name them. The second
// answer is false on a computer without Omarchy's scripts.
func omarchys(sys machine.System, paths []string) (map[string]bool, bool) {
	var places []string
	for _, folder := range omarchyFolders {
		if sys.IsDir(folder) {
			places = append(places, folder)
		}
	}
	if len(places) == 0 {
		return nil, false
	}
	if sys.Exists(firmwareCopy) {
		places = append(places, firmwareCopy)
	}
	from := map[string]bool{}
	for _, name := range ownFiles {
		from[name] = true
	}
	args := []string{"-rhoF"}
	for _, path := range paths {
		if name := base(path); simpleName.MatchString(name) && !from[name] {
			args = append(args, "-e", name)
		}
	}
	if len(args) == 1 {
		return from, true
	}
	answer, _ := sys.RunFor(20*time.Second, "grep", append(args, places...)...)
	for _, name := range strings.Split(answer, "\n") {
		from[strings.TrimSpace(name)] = true
	}
	return from, true
}

// moduleSettings reads a file of module settings for the options it gives and the modules it
// keeps from loading.
func moduleSettings(text string) []string {
	var does []string
	for _, line := range lines(text) {
		fields := strings.Fields(line)
		if len(fields) < 2 || !simpleName.MatchString(fields[1]) {
			continue
		}
		switch {
		case fields[0] == "blacklist", fields[0] == "install" && len(fields) == 3 && (base(fields[2]) == "false" || base(fields[2]) == "true"):
			does = add(does, "keeps kernel module "+fields[1]+" from loading", nil, 20)
		case fields[0] == "options":
			for _, option := range fields[2:] {
				key, value, valued := strings.Cut(option, "=")
				if !optionKey.MatchString(key) {
					continue
				}
				if valued && (!optionValue.MatchString(value) || identity.MatchString(value)) {
					option = key + "=…"
				}
				does = add(does, "gives kernel module "+fields[1]+" the option "+option, nil, 20)
			}
		}
	}
	return does
}

// Rules says what a file of device rules does, one line for each thing a rule does to a kind of
// device, like "usb sets power/control=on". The devices a rule picks out by their IDs or names,
// and the program it runs, are left out.
func Rules(text string) []string {
	var rules []string
	for _, line := range lines(strings.ReplaceAll(text, "\\\n", " ")) {
		kind := "any"
		var does []string
		for _, token := range ruleToken.FindAllStringSubmatch(line, -1) {
			key, name, sign, value := token[1], token[2], token[3], token[4]
			matches := sign == "==" || sign == "!="
			switch {
			case matches && (key == "SUBSYSTEM" || key == "SUBSYSTEMS") && sign == "==" && kind == "any" && deviceKind.MatchString(value):
				kind = value
			case matches:
			case key == "ATTR" && attribute.MatchString(name):
				if optionValue.MatchString(value) && !identity.MatchString(value) {
					name += "=" + value
				}
				does = append(does, "sets "+name)
			case key == "RUN":
				does = append(does, "runs")
			case key == "ENV" && envName.MatchString(name):
				does = append(does, "env "+name)
			case key == "MODE" || key == "GROUP" || key == "OWNER" || (key == "TAG" && value == "uaccess"):
				does = append(does, "permissions")
			}
		}
		for _, did := range does {
			rules = add(rules, kind+" "+did, nil, 40)
		}
	}
	return rules
}

// SoundSettings are the names of the settings a sound settings file sets, without their values
// and without the ones that pick a device out.
func SoundSettings(text string) []string {
	var keys []string
	for _, line := range lines(text) {
		for _, match := range soundKey.FindAllStringSubmatch(line, -1) {
			if match[2] == "[" || match[2] == "{" || soundMatch.MatchString(match[1]) || len(match[1]) > 60 {
				continue
			}
			keys = add(keys, match[1], nil, 40)
		}
	}
	sort.Strings(keys)
	return keys
}

// lines are a settings file's lines that say something, without comments.
func lines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

// names are the lines of an answer that are a package's name and nothing else.
func names(answer string, limit int) []string {
	var out []string
	for _, line := range strings.Split(answer, "\n") {
		out = add(out, strings.TrimSpace(line), simpleName, limit)
	}
	return out
}

// add appends an item once, when it has the given shape and the list has room.
func add(list []string, item string, shape *regexp.Regexp, limit int) []string {
	if item == "" || len(list) >= limit || (shape != nil && !shape.MatchString(item)) {
		return list
	}
	for _, have := range list {
		if have == item {
			return list
		}
	}
	return append(list, item)
}

func base(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}
