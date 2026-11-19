package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/birkskyum/arewearmyet/app/internal/guided"
)

// The reference on the site names every command, step and option that hw-test help does, so a
// command added here without a line there fails.
func TestReferenceCoversHelp(t *testing.T) {
	data, err := os.ReadFile("../src/lib/hw-test.md")
	if err != nil {
		t.Fatal(err)
	}
	reference := string(data)
	help := usageText()

	commands := regexp.MustCompile(`(?m)^  hw-test ([a-z]+)`).FindAllStringSubmatch(help, -1)
	options := regexp.MustCompile(`--[a-z][a-z-]+`).FindAllString(help, -1)
	if len(commands) < 15 || len(options) < 15 {
		t.Fatalf("the help text gave %d commands and %d options, too few to be all of them", len(commands), len(options))
	}
	for _, found := range commands {
		if !strings.Contains(reference, "hw-test "+found[1]) {
			t.Errorf("the reference has no hw-test %s", found[1])
		}
	}
	for _, word := range guided.StepWords() {
		if !strings.Contains(reference, "`"+word+"`") && !strings.Contains(reference, "hw-test "+word) {
			t.Errorf("the reference doesn't name the step %s", word)
		}
	}
	for _, option := range options {
		if !strings.Contains(reference, option) {
			t.Errorf("the reference doesn't name %s", option)
		}
	}
}
