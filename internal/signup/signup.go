// Package signup records the addresses the landing page collects.
//
// # Why this is a package rather than four lines in the handler
//
// The gate handler's job is to decide whether a submission is well formed and what to
// answer. Where the address goes is a separate concern with a separate lifetime: it
// started as "nowhere", it is Postgres now, and the Google sign-in flow will write to it
// from a second call site. A one-method interface is what keeps the handler from learning
// any of that.
//
// # What is deliberately NOT here
//
// No read side. Nothing in the running application ever lists the addresses it has
// collected, because nothing in the application has a reason to — the page does not show
// them and the agent does not reason over them. A list that can only be written by the
// server, and read by a human holding database credentials, is a materially smaller thing
// to get wrong than one with a query path in front of it.
package signup

import (
	"context"
	"errors"
	"net/mail"
	"strings"
)

// Source names the control that produced a submission. It is stored rather than inferred
// because the two carry different evidence: an address typed into a form is a claim, and
// an address from an identity provider is a fact that provider is willing to stand behind.
type Source string

const (
	// SourceForm is the landing page's email field.
	SourceForm Source = "form"
	// SourceGoogle is the Sign in with Google flow.
	SourceGoogle Source = "google"
)

// Record is one submission.
type Record struct {
	// Email as the reader gave it, less surrounding whitespace. The original case is
	// kept: the local part of an address is case-sensitive by the spec, so whatever
	// eventually sends mail should use what it was given rather than a version this
	// package tidied.
	Email string
	// Source is the control it came from.
	Source Source
	// Verified says whether the address was proved rather than asserted. Only an
	// identity provider can set this; a typed address is never verified, however
	// plausible it looks.
	Verified bool
}

// ErrNotAnAddress is returned for input that is not shaped like an email address.
//
// A sentinel rather than a formatted error, because the handler answers a different HTTP
// status for it than for a storage failure — and matching on message text is how that
// distinction quietly stops working.
var ErrNotAnAddress = errors.New("that does not look like an email address")

// Store is where submissions go.
//
// Add must be safe for concurrent use and must be idempotent on the address: the same
// person submitting twice is the ordinary case rather than an error, and the caller has
// nothing useful to do about it either way.
type Store interface {
	Add(ctx context.Context, r Record) error
	Close()
}

// Clean validates and normalises a submitted address.
//
// The check is SHAPE ONLY, and deliberately so: the alternative is a gate that refuses
// addresses it merely disapproves of, and every regular expression written for this job
// has a real address it rejects. Whether an address receives mail is answered by sending
// mail to it, which is not this program's job.
func Clean(raw string) (string, error) {
	addr := strings.TrimSpace(raw)
	// mail.ParseAddress accepts a display name — `Bee <bee@example.com>` parses — so the
	// parsed Address is taken rather than the input. Keeping the raw string would let a
	// submission carry a display name into the list.
	parsed, err := mail.ParseAddress(addr)
	if err != nil {
		return "", ErrNotAnAddress
	}
	// ⚠️ Taking parsed.Address is NOT sufficient on its own, and the failure is the
	// opposite of the one above: it UNQUOTES a quoted local part. `"a,b"@example.com` is
	// a legal address, and parsed.Address returns `a,b@example.com` — which is not the
	// same address, is not a legal addr-spec at all, and reads as two recipients the
	// moment anything builds a mail header from it. Measured, not assumed: the same
	// happens for an embedded space and an embedded @.
	//
	// So the parsed form must survive being parsed again, unchanged. Anything that does
	// not is refused rather than repaired: these addresses are vanishingly rare, a
	// person who has one can supply another, and silently storing a mangled address is
	// worse than declining it. This is the one place the shape check is more than shape,
	// and it is here because the alternative is corrupting the data on the way in.
	again, err := mail.ParseAddress(parsed.Address)
	if err != nil || again.Address != parsed.Address {
		return "", ErrNotAnAddress
	}
	// ⚠️ mail.ParseAddress enforces NO LENGTH LIMIT, and this route is the one write path
	// on a public server. Measured: a 4000-character address parses, survives the
	// re-parse above, and would be stored twice — once as email and once as email_key —
	// on a hostPath the deployment shares with the FPL archive, the proxy cache and
	// Traefik's acme.json. Every distinct string is a fresh key, so nothing dedupes it.
	// A filled disk there takes certificate renewal down with the signup list.
	//
	// RFC 5321: 64 octets for the local part, 254 for the whole address. Both are
	// checked, because a 64-octet local part with a 4000-octet domain passes the first
	// bound and fails the second.
	if len(parsed.Address) > maxAddress {
		return "", ErrNotAnAddress
	}
	if local, _, ok := strings.Cut(parsed.Address, "@"); !ok || len(local) > maxLocalPart {
		return "", ErrNotAnAddress
	}
	return parsed.Address, nil
}

// The RFC 5321 bounds, in octets. They are here rather than inline because the schema
// carries the same numbers as a CHECK constraint — the Google sign-in flow will reach the
// store through a second call site, and a bound enforced only in this function is a bound
// that call site can forget.
const (
	maxAddress   = 254
	maxLocalPart = 64
)

// Key is the value uniqueness is decided on.
//
// Lowercasing the whole address is technically wrong — the local part is case-sensitive,
// so `Bee@example.com` and `bee@example.com` may in principle be two mailboxes — and it is
// what every provider that matters actually does. The cost of being wrong this way is
// mailing one person once; the cost of the other way is a list that quietly accumulates
// duplicates of the same human. This treats the address as an identity rather than as a
// routing token, which is what a signup list is for.
func Key(address string) string {
	return strings.ToLower(address)
}
