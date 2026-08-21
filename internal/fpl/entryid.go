package fpl

import (
	"errors"
	"strconv"
	"strings"
)

// ErrNotAnEntryID is returned for input that is not shaped like an FPL entry (Team) id.
//
// A sentinel rather than a formatted error, for the same reason signup.ErrNotAnAddress is
// one: a caller answers a different HTTP status for "this was never going to be an id"
// (400) than for "well-formed, but FPL has never heard of it" (404) — and matching on
// message text is how that distinction quietly stops working the first time the wording
// changes.
var ErrNotAnEntryID = errors.New("that does not look like an FPL Team ID")

// maxEntryIDDigits bounds the residue BEFORE it ever reaches strconv.Atoi.
//
// PUT /api/import is a public write path, and an unbounded string reaching a numeric
// parser is the kind of thing this codebase has already paid for once — see
// signup.Clean's own length-before-parse discipline on the landing page's gate, the other
// public write path with a hand-typed field. Twelve digits is generous: it comfortably
// covers maxEntryID below with room to spare, while still refusing an obviously-not-an-id
// string (a phone number, a UNIX timestamp in milliseconds) before any further work.
const maxEntryIDDigits = 12

// maxEntryID bounds the accepted range from above.
//
// FPL hands out entry ids sequentially as managers sign up, and they currently sit in the
// low tens of millions. 99,999,999 is not a measured ceiling — it is headroom comfortably
// above today's range that still refuses an obviously bogus value (a phone number, a
// timestamp) before it ever reaches the network.
const maxEntryID = 99_999_999

// EntryIDInRange reports whether id falls inside ParseEntryID's accepted range.
//
// Split out so a caller holding an already-parsed int — cmd/armband's validateSession,
// re-checking a value pulled back out of a stored session cookie — can re-validate it
// without going through the string path a second time, and without a second copy of the
// bounds to drift out of step with ParseEntryID's own.
func EntryIDInRange(id int) bool {
	return id >= 1 && id <= maxEntryID
}

// ParseEntryID validates and normalises a Team ID a visitor pasted into the import box.
//
// Shape only, the same discipline signup.Clean applies to an email address: strip what a
// human plausibly pastes, then refuse anything that is not left as a bare, in-range
// integer. Whether the id names a real FPL manager is a question for the network, and
// answering it is the caller's job — see ErrNotAnEntryID's doc comment for why that
// failure carries a different HTTP status than this one.
//
// Two things a visitor plausibly pastes are handled specially:
//
//   - The whole points-page URL, e.g.
//     "https://fantasy.premierleague.com/entry/1234567/event/5" — the digit run
//     immediately after "/entry/" is taken and everything else (scheme, event number,
//     trailing path) is discarded.
//   - A thousands-separated number, e.g. "1,234,567" — commas and internal spaces are
//     stripped before the digit check.
//
// Everything else must already be a bare run of ASCII digits: no sign, no thousands
// separator this function does not know about, and no non-ASCII digit (Arabic-indic and
// similar decimal digits parse in some locales but must not silently become a different
// id here).
func ParseEntryID(raw string) (int, error) {
	s := strings.TrimSpace(raw)

	if idx := strings.Index(s, "/entry/"); idx != -1 {
		// A URL: take the digit run immediately following "/entry/" and drop
		// everything else the string carries — scheme, "/event/5", a trailing slash.
		// Bounded to maxEntryIDDigits below, so a pathological string here (a
		// traversal attempt, an enormous digit run) is refused rather than fed
		// anywhere: the extracted value is only ever used as an int from here on,
		// never as a path fragment.
		rest := s[idx+len("/entry/"):]
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		s = rest[:end]
	} else {
		// A plain pasted number, possibly thousands-separated. Only comma and space
		// are stripped — the two separators a human plausibly types — so anything
		// else left in the string still fails the digit check below rather than
		// being silently repaired into a different number.
		s = strings.ReplaceAll(s, ",", "")
		s = strings.ReplaceAll(s, " ", "")
	}

	// Bounded before strconv.Atoi ever sees it — see maxEntryIDDigits.
	if s == "" || len(s) > maxEntryIDDigits {
		return 0, ErrNotAnEntryID
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			// Catches a sign, a decimal point, and any non-ASCII digit — Go source
			// (and strconv) only ever treats '0'-'9' as digits, but the rule is
			// stated explicitly here so it reads as a decision rather than an
			// emergent property of the standard library.
			return 0, ErrNotAnEntryID
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, ErrNotAnEntryID
	}
	if !EntryIDInRange(n) {
		return 0, ErrNotAnEntryID
	}
	return n, nil
}
