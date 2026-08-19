package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
)

// cmdServe hosts the squad page over HTTP.
//
// It is the same page `armband squad -html` writes to disk, built by the same
// pipeline (buildSquadPage) — the difference is that it is live, and once the
// page carries write actions those actions POST back to this listener.
//
// # Why the page re-renders on every GET
//
// The alternative is a cached page invalidated on POST, and a cache with
// invalidation is this project's signature bug class: the page silently shows
// last week's squad, or last week's overrides. Rebuilding costs the optimiser's
// seconds, which a person clicking through a page will not notice, and it means
// the page can never be stale — the one correct path, no fallback.
//
// A single mutex serialises requests. This is a one-user tool and the engine
// was not written for concurrent access; the mutex is cheaper than auditing it.
func cmdServe(ctx context.Context, cfg config.Config, cfgPath string,
	client *fpl.Client, e *analysis.Engine, weeks int) error {

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "address to serve on (loopback only)")
	if err := fs.Parse(flag.Args()[1:]); err != nil {
		return err
	}
	if err := validateServeAddr(*addr); err != nil {
		return err
	}
	token, err := newServeToken()
	if err != nil {
		return err
	}

	s := &squadServer{
		token:   token,
		cfg:     &cfg,
		cfgPath: cfgPath,
		client:  client,
		engine:  e,
		weeks:   weeks,
	}

	fmt.Fprintf(os.Stderr, "\n%s\n", dim("Serving the squad page on http://"+*addr+"/?t="+token))
	fmt.Fprintf(os.Stderr, "%s\n", dim("Open that exact URL — the token gates the page's write actions."))
	fmt.Fprintf(os.Stderr, "%s\n\n", dim("Ctrl-C stops the server."))
	return http.ListenAndServe(*addr, s)
}

// squadServer is the whole server: the page, and later the write actions.
type squadServer struct {
	mu    sync.Mutex
	token string
	cfg   *config.Config
	// cfgPath is where POSTs persist changes. It may be empty, which makes the
	// server read-only in effect — an override that cannot be saved must not be
	// applied, because the page would show it and the next run would not.
	cfgPath string
	client  *fpl.Client
	engine  *analysis.Engine
	weeks   int
}

func (s *squadServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		s.page(w, r)
	default:
		http.NotFound(w, r)
	}
}

// page renders the full squad page, rebuilt from scratch for this request.
func (s *squadServer) page(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := buildSquadPage(r.Context(), *s.cfg, s.client, s.engine, s.weeks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := present.Render(w, b.Page); err != nil {
		// The template is static text compiled once; an execution error is a
		// programming error, and it arrives after the response has started, so
		// the only honest report is to the server's own stderr.
		fmt.Fprintf(os.Stderr, "render: %v\n", err)
	}
}

// newServeToken is a per-startup secret that gates the write actions.
//
// The token is printed once in the URL to open, and the page embeds it in its
// action forms. Anything else on the network — or a hostile page open in the
// same browser, which can fire an unauthenticated POST at localhost — does not
// have it, and the write path checks it before touching config.
func newServeToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate serve token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// validateServeAddr accepts only loopback listeners.
//
// The served page can write config.json, so the listener is the outer
// perimeter of that write path. Bound to anything but loopback, the token is
// the only gate between a mutation and every host that can reach the port, and
// a random per-startup token was never meant to carry a perimeter alone. A
// non-loopback bind is always a decision, so refusing it early is the cheap
// half of the threat model.
func validateServeAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("serve -addr must be host:port, got %q: %w", addr, err)
	}
	switch host {
	case "127.0.0.1", "::1", "localhost":
		return nil
	}
	return fmt.Errorf("serve refuses %q: the page can write config.json, so it binds "+
		"loopback only — use -addr 127.0.0.1:<port>", addr)
}

// tokenOK compares a submitted token against the server's, in constant time.
//
// The length guard is not an optimisation — ConstantTimeCompare leaks length,
// and length leaks nothing worth having here, so the early return keeps the
// comparison on equal-length inputs where its timing says nothing.
func (s *squadServer) tokenOK(got string) bool {
	if len(got) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}
