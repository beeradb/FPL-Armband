package fpl

import (
	"errors"
	"testing"
)

func TestParseEntryIDAcceptsWhatAVisitorPlausiblyPastes(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		want     int
	}{
		{"plain digits", "1234567", 1234567},
		{"surrounding whitespace", "  1234567  ", 1234567},
		{"pasted full URL", "https://fantasy.premierleague.com/entry/1234567/event/5", 1234567},
		{"pasted URL with trailing /event/N", "https://fantasy.premierleague.com/entry/42/event/12", 42},
		{"comma-separated", "1,234,567", 1234567},
		{"comma and space together", "1, 234, 567", 1234567},
		{"leading zeros", "0001234", 1234},
		{"minimum", "1", 1},
		{"maximum", "99999999", 99999999},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEntryID(tc.in)
			if err != nil {
				t.Fatalf("ParseEntryID(%q) = _, %v, want %d, nil", tc.in, err, tc.want)
			}
			if got != tc.want {
				t.Errorf("ParseEntryID(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseEntryIDRefusesWhatIsNotAnID(t *testing.T) {
	for _, tc := range []struct {
		name, in string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"zero", "0"},
		{"negative", "-5"},
		{"thirteen digits", "1234567890123"},
		{"a directory traversal attempt", "1/../../bootstrap-static"},
		{"traversal reachable via /entry/", "https://x/entry/../../../bootstrap-static"},
		// Arabic-indic digits for "123" — must not be transliterated into 123.
		{"unicode digits", "١٢٣"},
		{"way over range", "999999999999"},
		{"not a number at all", "not-an-id"},
		{"a decimal", "12.5"},
		{"plus sign", "+5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := ParseEntryID(tc.in); err == nil {
				t.Errorf("ParseEntryID(%q) = %d, nil, want ErrNotAnEntryID", tc.in, got)
			} else if !errors.Is(err, ErrNotAnEntryID) {
				t.Errorf("ParseEntryID(%q) returned %v, want ErrNotAnEntryID so a caller "+
					"can distinguish this from any other failure without matching on "+
					"message text", tc.in, err)
			}
		})
	}
}

// TestParseEntryIDNeverPathTraversesEvenOnRefusal pins that a string carrying a
// traversal attempt never reaches anywhere as a STRING. Two shapes:
//
//   - No "/entry/" marker at all: the comma/space-strip branch runs, the residue still
//     has "/", "." and letters in it, and the digit check refuses the whole thing.
//   - An "/entry/" marker followed by a genuine digit run and THEN traversal garbage:
//     this is the ordinary "pasted the whole URL" case (the digit run after "/entry/"
//     is exactly what the URL case is designed to extract), so it succeeds and returns
//     the digit run as a plain int — which is the safe outcome, since a parsed
//     ParseEntryID result is only ever used as an int from here on, never spliced back
//     into a request path as a string. Nothing downstream sees "../../../etc/passwd" at
//     all; it is discarded the moment the digit run ends.
func TestParseEntryIDNeverPathTraversesEvenOnRefusal(t *testing.T) {
	if _, err := ParseEntryID("1/../../bootstrap-static"); !errors.Is(err, ErrNotAnEntryID) {
		t.Errorf("a traversal string with no /entry/ marker was not refused: %v", err)
	}
	if _, err := ParseEntryID("/entry/../secrets"); !errors.Is(err, ErrNotAnEntryID) {
		t.Errorf("/entry/ followed by no digit at all was not refused: %v", err)
	}
	// The digit run is extracted and everything after it is discarded, not traversed.
	got, err := ParseEntryID("https://fantasy.premierleague.com/entry/1/../../../etc/passwd")
	if err != nil {
		t.Fatalf("digit run after /entry/ should parse regardless of what follows it: %v", err)
	}
	if got != 1 {
		t.Errorf("got id %d, want 1 — the traversal suffix must be discarded, not consulted", got)
	}
}

func TestEntryIDInRangeAgreesWithParseEntryID(t *testing.T) {
	for _, id := range []int{0, 1, 42, maxEntryID, maxEntryID + 1, -1} {
		want := id >= 1 && id <= maxEntryID
		if got := EntryIDInRange(id); got != want {
			t.Errorf("EntryIDInRange(%d) = %v, want %v", id, got, want)
		}
	}
}
