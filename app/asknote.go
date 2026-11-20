package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// Everyday words for a part, and the check each one means.
var partWords = map[string]string{
	"sleep": "suspend", "wake": "suspend", "resume": "suspend", "wi-fi": "wifi", "wlan": "wifi", "network": "wifi", "internet": "wifi",
	"sound": "speakers", "speaker": "speakers", "audio": "speakers", "mic": "microphones", "microphone": "microphones",
	"headphone": "headphones", "jack": "headphones", "screen": "display", "monitor": "external-display", "hdmi": "external-display",
	"fan": "fans", "heat": "fans", "temperature": "fans", "temperatures": "fans", "bt": "bluetooth", "webcam": "camera",
	"graphics": "gpu", "trackpad": "touchpad", "brightness": "backlight", "charger": "charging", "power": "charging",
	"hibernation": "hibernate", "password": "disk-unlock", "unlock": "disk-unlock", "install": "installer", "video": "video-decode",
}

// part finds the check a word means: its id, an everyday word for it, or a word of its title.
// Without the site's list, a known word still stands.
func part(word string, checks map[string]report.Check) (string, bool) {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return "", false
	}
	if id, known := partWords[word]; known {
		word = id
	}
	if _, known := checks[word]; known || len(checks) == 0 {
		return word, true
	}
	var found []string
	for id, check := range checks {
		for _, piece := range strings.FieldsFunc(strings.ToLower(check.Title), func(r rune) bool { return r == ' ' || r == '(' || r == ')' }) {
			if piece == word {
				found = append(found, id)
			}
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return "", false
}

// headline makes a note's one line from its text: all of it when it is short, and otherwise
// its start, up to the end of a sentence or of a word.
func headline(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 100 {
		text = text[:100]
		if end := strings.LastIndexAny(text, ".!?"); end >= 40 {
			text = text[:end]
		} else if cut := strings.LastIndexByte(text, ' '); cut > 60 {
			text = text[:cut]
		}
	}
	return strings.TrimRight(text, ".!? ")
}

// askNote asks at the terminal for what a note needs and wasn't given: which part it is
// about, what happened, and which log shows it. Nobody has to type the options out. It
// answers false when the owner gave nothing to write.
func askNote(in *bufio.Reader, out io.Writer, checks map[string]report.Check, args noteArgs, word string) (noteArgs, bool) {
	ask := func(question string) string {
		fmt.Fprint(out, question+"\n> ")
		answer, _ := in.ReadString('\n')
		return strings.TrimSpace(answer)
	}
	if args.check == "" && args.reply == "" {
		for tries := 0; tries < 3; tries++ {
			if word == "" {
				word = ask("Which part is it about? One word, like wifi, sleep, sound or camera. Just Enter if it fits none.")
			}
			if word == "" {
				break
			}
			if id, known := part(word, checks); known {
				args.check = id
				break
			}
			var ids []string
			for id := range checks {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			fmt.Fprintf(out, "No part is called %q here. The parts are %s.\n", word, strings.Join(ids, ", "))
			word = ""
		}
	}
	for tries := 0; args.body == "" && tries < 3; tries++ {
		text := ask("What happened? A sentence or two, then Enter.")
		switch {
		case text == "":
			return args, false
		case len(text) < 20:
			fmt.Fprintln(out, "A little more, please, on what you did and what the computer did.")
		default:
			args.body = text
		}
	}
	if args.body == "" {
		return args, false
	}
	if args.title == "" && args.reply == "" {
		args.title = headline(args.body)
	}
	if len(args.logs) == 0 {
		// The log that shows it is this start's, or the one before when the computer was restarted since
		answer := strings.ToLower(ask("Has the computer been restarted since it happened? [y/N]"))
		if answer == "y" || answer == "yes" {
			args.logs = []string{"previous"}
		} else {
			args.logs = []string{"kernel"}
		}
	}
	return args, true
}
