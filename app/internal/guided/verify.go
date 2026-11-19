package guided

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/birkskyum/arewearmyet/app/internal/report"
)

// measuredAs says a result in a few words for a table: its state, and what showed it.
func measuredAs(result report.Result) string {
	if result.State == report.Untested {
		return "not tested"
	}
	text := stateLabel[result.State]
	if detail := first(result.Evidence, result.Note); detail != "" {
		text += ": " + strings.ReplaceAll(detail, "|", "/")
	}
	if how := howSaid[result.How]; how != "" {
		text += " (" + how + ")"
	}
	return text
}

// versionsChanged lists the packages whose version differs between the two sides, and the
// kernel when it does.
func versionsChanged(before, after report.Versions) []string {
	var lines []string
	if before.Kernel != after.Kernel {
		lines = append(lines, fmt.Sprintf("kernel %s → %s", first(before.Kernel, "unknown"), first(after.Kernel, "unknown")))
	}
	var names []string
	for name := range after.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if was := before.Packages[name]; was != after.Packages[name] {
			lines = append(lines, fmt.Sprintf("%s %s → %s", name, first(was, "not installed"), after.Packages[name]))
		}
	}
	return lines
}

// VerifyBlock is what goes into a pull request to show what it changed: the machine, the
// versions on both sides, and each check tested before and after, in Markdown. A check that
// works on both sides is only named, unless it is among rows: those get a row with what was
// measured, for a change that moves a number and not the state.
func VerifyBlock(run *report.Run, model report.Model, snapshot report.Snapshot, changes []report.Change, testImage bool, rows ...string) string {
	row := map[string]bool{}
	for _, check := range rows {
		row[check] = true
	}
	var out strings.Builder
	say := func(format string, args ...any) { fmt.Fprintf(&out, format+"\n", args...) }
	machine := strings.TrimSpace(run.Hardware["maker"] + " " + first(run.Hardware["family"], run.Hardware["product"]))
	say("### Measured with hw-test")
	say("")
	say("%s, before and after this change. Before: %s. After: %s.", machine, snapshot.When.Format("2 Jan 2006 15:04"), time.Now().Format("2 Jan 2006 15:04"))
	after := report.Versions{Kernel: run.Kernel, Packages: run.Packages}
	moved := versionsChanged(snapshot.Versions, after)
	switch {
	// The test image was there before the change too, so the device tree is not what differs
	case testImage && snapshot.TestImage && len(moved) == 0:
		say("Kernel %s and the same package versions on both sides, so the change was tried outside the packages (a setting or a file of its own). Both sides ran the same test boot image, made by `hw-test tree boot` for another change.", first(run.Kernel, "unknown"))
	case testImage && snapshot.TestImage:
		say("What differs between the two: %s. Both sides ran the same test boot image, made by `hw-test tree boot` for another change.", strings.Join(moved, ", "))
	case testImage && len(moved) == 0:
		say("Kernel %s and the same package versions on both sides. After, the computer was started from a test boot image with the changed device tree, made by `hw-test tree boot`.", first(run.Kernel, "unknown"))
	case testImage:
		say("What differs between the two: %s. After, the computer was started from a test boot image with the changed device tree.", strings.Join(moved, ", "))
	case len(moved) > 0:
		say("What differs between the two: %s.", strings.Join(moved, ", "))
	default:
		say("Kernel %s and the same package versions on both sides, so the change was tried outside the packages (a boot entry or a file of its own).", first(run.Kernel, "unknown"))
	}
	say("")
	say("| Check | Before | After |")
	say("| --- | --- | --- |")
	var same, fresh []string
	for _, change := range changes {
		// With nothing from before, a result says nothing of what the change did
		if change.From.State == report.Untested {
			fresh = append(fresh, strings.ToLower(first(checkTitle[change.Check], change.Check))+" ("+stateLabel[change.To.State]+")")
			continue
		}
		from, known := report.Rank[change.From.State]
		to, measured := report.Rank[change.To.State]
		if known && measured && from == to && change.From.State == report.Works && !row[change.Check] {
			same = append(same, strings.ToLower(first(checkTitle[change.Check], change.Check)))
			continue
		}
		mark := ""
		switch {
		case !known || !measured:
		case to < from:
			mark = "✅ "
		case to > from:
			mark = "❌ "
		}
		say("| %s%s | %s | %s |", mark, first(checkTitle[change.Check], change.Check), measuredAs(change.From), measuredAs(change.To))
	}
	if len(same) > 0 {
		say("")
		say("Tested again and still working: %s.", strings.Join(same, ", "))
	}
	if len(fresh) > 0 {
		say("")
		say("Tested for the first time after the change, so with nothing to compare: %s.", strings.Join(fresh, ", "))
	}
	if model.ID != "" {
		say("")
		say("The model's page, with every machine's results: %s", first(model.Page, report.Site+"/models/"+model.ID))
	}
	return out.String()
}
