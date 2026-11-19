package guided

import (
	"strings"
	"testing"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

func TestTheBlockForAPullRequestShowsEachCheckBeforeAndAfter(t *testing.T) {
	run := &report.Run{Kernel: "7.3.0-72-omarchy", Hardware: map[string]string{"maker": "LENOVO", "family": "Yoga Slim 7 14Q8X9"},
		Packages: map[string]string{"linux-omarchy": "7.3-72", "linux-omarchy-dtbs": "7.3-10", "mesa": "26.2.5"}}
	snapshot := report.Snapshot{When: time.Date(2026, 11, 19, 6, 0, 0, 0, time.UTC),
		Versions: report.Versions{Kernel: "7.3.0-72-omarchy", Packages: map[string]string{"linux-omarchy": "7.3-72", "linux-omarchy-dtbs": "7.3-9", "mesa": "26.2.5"}}}
	side := func(state, how, evidence string) report.Result {
		return report.Result{State: state, How: how, Evidence: evidence}
	}
	changes := []report.Change{
		{Check: "speakers", From: side(report.Partial, report.Owner, "left heard at -31 dB, right not heard"), To: side(report.Works, report.Measured, "left heard at -31 dB, right heard at -32 dB")},
		{Check: "headphones", From: side(report.Works, report.Owner, ""), To: side(report.Broken, report.Owner, "no sound | after the change")},
		{Check: "microphones", From: report.Result{State: report.Untested}, To: side(report.Works, report.Measured, "1000 Hz at -35 dB")},
		{Check: "wifi", From: side(report.Works, report.Measured, "connected"), To: side(report.Works, report.Measured, "connected")},
	}
	block := VerifyBlock(run, report.Model{ID: "lenovo-yoga-slim-7x-14q8x9", Page: "https://arewearmyet.com/models/lenovo-yoga-slim-7x-14q8x9"}, snapshot, changes, false)
	for _, text := range []string{
		"### Measured with hw-test", "LENOVO Yoga Slim 7 14Q8X9, before and after this change",
		// Only what differs between the two sides is listed, which is what was changed
		"What differs between the two: linux-omarchy-dtbs 7.3-9 → 7.3-10.",
		"| ✅ Speakers | partly: left heard at -31 dB, right not heard (seen by the owner) | works: left heard at -31 dB, right heard at -32 dB (measured by the test) |",
		// What got worse is shown as plainly as what got better
		"| ❌ Headphone jack | works (seen by the owner) | broken: no sound / after the change (seen by the owner) |",
		// What was never tested before isn't a row: it says nothing of what the change did
		"Tested for the first time after the change, so with nothing to compare: built-in microphones (works).",
		"Tested again and still working: wi-fi.",
		"https://arewearmyet.com/models/lenovo-yoga-slim-7x-14q8x9",
	} {
		if !strings.Contains(block, text) {
			t.Errorf("the block lacks %q:\n%s", text, block)
		}
	}
	if strings.Contains(block, "mesa") || strings.Contains(block, "| Wi-Fi |") || strings.Contains(block, "| Built-in microphones |") {
		t.Errorf("the block lists what didn't change:\n%s", block)
	}
	// A check that works on both sides gets a row when it is asked for: a change can move a
	// number without moving the state
	if block = VerifyBlock(run, report.Model{}, snapshot, changes, false, "wifi"); !strings.Contains(block, "| Wi-Fi | works: connected (measured by the test) | works: connected (measured by the test) |") || strings.Contains(block, "still working") {
		t.Errorf("an asked-for row isn't shown:\n%s", block)
	}
	// A change tried outside the packages says so, since the versions alone would show nothing
	snapshot.Versions.Packages["linux-omarchy-dtbs"] = "7.3-10"
	if block = VerifyBlock(run, report.Model{}, snapshot, changes, false); !strings.Contains(block, "the same package versions on both sides, so the change was tried outside the packages") {
		t.Errorf("unchanged versions aren't said:\n%s", block)
	}
	// Started from the test image, it says so: that is the change no package version shows
	if block = VerifyBlock(run, report.Model{}, snapshot, changes, true); !strings.Contains(block, "started from a test boot image with the changed device tree, made by `hw-test tree boot`") {
		t.Errorf("the test image isn't said:\n%s", block)
	}
}
