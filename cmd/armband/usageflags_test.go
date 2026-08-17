package main

import (
	"flag"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// registeredGlobalFlags asks the flag package what run() registers, by building
// the same set on a throwaway FlagSet.
//
// An earlier version of this file scanned the source with go/ast for
// `flag.String(...)` calls. That was a second implementation of the flag
// registry, and it was blind to the whole XxxVar family — converting one flag to
// flag.IntVar would have removed it from the test's view without failing
// anything. VisitAll cannot drift from the thing it reports on.
func registeredGlobalFlags() []string {
	fs := flag.NewFlagSet("usage-test", flag.ContinueOnError)
	registerGlobalFlags(fs)
	var names []string
	fs.VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
	sort.Strings(names)
	return names
}

// documentedFlagLine matches a flag as the usage text's Flags: block writes one:
// indented, a leading dash, then a name that starts with a letter. Continuation
// lines of a description do not match, and neither does an em dash.
var documentedFlagLine = regexp.MustCompile(`(?m)^\s+-([A-Za-z][A-Za-z0-9-]*)`)

func documentedGlobalFlags(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, m := range documentedFlagLine.FindAllStringSubmatch(sectionOfUsage(t, "Flags:", "Examples:"), -1) {
		names = append(names, m[1])
	}
	sort.Strings(names)
	return names
}

// TestEveryGlobalFlagAppearsInTheUsageText is the guard for a defect that
// shipped. README.md carried a hand-written list of five flags when the CLI had
// eight — correct when it was written at 086eecf, stale from dbeee98, and
// rewritten unchecked by 4546732. -plain, -html and -weeks were absent from the
// README and from docs/ entirely, so the flag that writes the HTML page was
// discoverable only by running the binary with no arguments.
//
// The README's list is gone, which leaves the usage text as the only complete
// one. This asserts it stays complete BOTH WAYS: a registered flag that nobody
// documented, and a documented flag nobody registers, are both lies, and after
// the deletion this string is the only place left for either to hide.
func TestEveryGlobalFlagAppearsInTheUsageText(t *testing.T) {
	registered := registeredGlobalFlags()
	documented := documentedGlobalFlags(t)

	// A scan that found nothing would satisfy a one-directional check
	// vacuously. Set equality cannot be satisfied vacuously unless BOTH sides
	// are empty, so pin a floor on one of them and the rest follows.
	if len(registered) < 5 {
		t.Fatalf("only %d global flags registered (%v) — registerGlobalFlags has changed shape", len(registered), registered)
	}

	if strings.Join(registered, ",") != strings.Join(documented, ",") {
		t.Errorf("the usage text's Flags: block and the registered flags disagree.\n"+
			"registered:  %v\ndocumented:  %v\n"+
			"Every global flag must be reachable by reading `armband` with no arguments — "+
			"that string is the only complete list.", registered, documented)
	}
}

// TestTheUsageExamplesPutFlagsBeforeTheCommand pins the ordering the examples
// exist to teach, using the real parser rather than a model of it.
//
// It also checks the subcommands, which is where the first version of this test
// was worthless: rejectFlagsAfterCommand returns nil on sight of a command in
// commandsThatParseTheirOwnFlags, so the example
// `armband backfill 2023-24 -coverage` passed while being false — FlagSet.Parse
// stops at the first non-flag argument exactly as flag.Parse does, so -coverage
// was read as a second season name and the documented invocation would have
// crawled the Internet Archive it promised not to touch.
func TestTheUsageExamplesPutFlagsBeforeTheCommand(t *testing.T) {
	examples := sectionOfUsage(t, "Examples:", "The agent reads")

	if !strings.Contains(examples, "WRONG") {
		t.Error("the misordered example is no longer labelled WRONG")
	}

	var checked, wrongOnes int
	for _, line := range strings.Split(examples, "\n") {
		invocation := invocationOf(line)
		if invocation == "" {
			continue
		}
		fields := strings.Fields(invocation)[1:]

		// Let Go split flags from the command, so a new boolean flag cannot
		// silently change what this test thinks it is looking at.
		fs := flag.NewFlagSet("example", flag.ContinueOnError)
		fs.SetOutput(discard{})
		registerGlobalFlags(fs)
		if err := fs.Parse(fields); err != nil {
			t.Errorf("usage example %q does not parse: %v", invocation, err)
			continue
		}
		if fs.NArg() == 0 {
			t.Errorf("usage example %q names no command", invocation)
			continue
		}
		cmd, rest := fs.Arg(0), fs.Args()[1:]

		if strings.Contains(line, "WRONG") {
			wrongOnes++
			if err := rejectFlagsAfterCommand(cmd, rest); err == nil {
				t.Errorf("the example labelled WRONG is in fact accepted: %q", invocation)
			}
			continue
		}
		checked++

		if err := rejectFlagsAfterCommand(cmd, rest); err != nil {
			t.Errorf("usage example %q is the form the guard rejects: %v", invocation, err)
		}

		// The guard exempts the self-parsing subcommands, so it says nothing
		// about their arguments. Their own FlagSet has the same stop-at-the-
		// first-positional rule, so the examples must still respect it.
		if commandsThatParseTheirOwnFlags[cmd] {
			var seenPositional string
			for _, a := range rest {
				if len(a) > 1 && strings.HasPrefix(a, "-") {
					if seenPositional != "" {
						t.Errorf("usage example %q puts %s after the positional argument %q; "+
							"%s's FlagSet stops parsing there and would read it as another argument",
							invocation, a, seenPositional, cmd)
					}
					continue
				}
				seenPositional = a
			}
		}
	}

	if checked < 6 {
		t.Errorf("only %d examples were checked — the Examples block or its formatting has changed "+
			"in a way that hides invocations from this test", checked)
	}
	if wrongOnes != 1 {
		t.Errorf("expected exactly one WRONG example, checked %d", wrongOnes)
	}
}

// selfParsingSentence captures the command names the usage text claims parse
// their own flags — the run between the heading colon and the full stop.
var selfParsingSentence = regexp.MustCompile(`(?s)parse their own flags, which therefore go AFTER the command:\s*([a-z, ]+?)\.`)

// TestTheUsageTextNamesExactlyTheSelfParsingCommands binds that sentence to
// commandsThatParseTheirOwnFlags.
//
// Without this the change that deleted README.md's stale FLAG list would have
// installed an unguarded COMMAND list in its place — the same defect one noun
// over, against the same map whose hand-listed copy has already drifted once
// (the reviewkey omission, which made every documented invocation of a new
// command error while go build, go vet and go test stayed clean).
//
// The sentence and the map are now exactly equal. They were not while `ask` was
// a command: it registered no FlagSet and sat in the map only because its
// question was free text that could begin with a dash, so it had to be excluded
// here and asserted separately. Retiring `ask` removed that exception, and this
// test is stronger for it — every exemption must now be a command that really
// does parse its own flags.
func TestTheUsageTextNamesExactlyTheSelfParsingCommands(t *testing.T) {
	m := selfParsingSentence.FindStringSubmatch(sectionOfUsage(t, "Examples:", "The agent reads"))
	if m == nil {
		t.Fatal("the Examples block no longer names which commands parse their own flags")
	}

	var named []string
	for _, part := range strings.Split(strings.ReplaceAll(m[1], " and ", ","), ",") {
		if p := strings.TrimSpace(part); p != "" {
			named = append(named, p)
		}
	}
	sort.Strings(named)

	var want []string
	for cmd := range commandsThatParseTheirOwnFlags {
		want = append(want, cmd)
	}
	sort.Strings(want)

	if strings.Join(named, ",") != strings.Join(want, ",") {
		t.Errorf("the usage text and commandsThatParseTheirOwnFlags disagree.\n"+
			"usage names: %v\nthe map has: %v", named, want)
	}
}

// invocationOf returns the command part of an example line, or "" if the line is
// not one. Descriptions are aligned with a run of two or more spaces, so that is
// the separator — which keeps prose out of the argument list without this file
// having to guess where the arguments end.
func invocationOf(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "armband ") {
		return ""
	}
	return strings.TrimSpace(regexp.MustCompile(`\s{2,}`).Split(trimmed, 2)[0])
}

// sectionOfUsage returns the usage text between two headings, so an assertion
// about one block cannot be satisfied by text in another. The first version of
// TestEveryGlobalFlagAppearsInTheUsageText searched the whole string, and
// deleting -weeks from the Flags: block still passed because an example below
// happened to use it.
func sectionOfUsage(t *testing.T, from, to string) string {
	t.Helper()
	i := strings.Index(usage, from)
	if i < 0 {
		t.Fatalf("the usage text has no %q section", from)
	}
	rest := usage[i+len(from):]
	j := strings.Index(rest, to)
	if j < 0 {
		t.Fatalf("the usage text has no %q section after %q", to, from)
	}
	return rest[:j]
}

// discard silences a FlagSet's own error output; the test reports failures.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
