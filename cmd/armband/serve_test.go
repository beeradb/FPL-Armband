package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServeRefusesANonLoopbackAddr pins the outer perimeter of the write path.
//
// The served page can write config.json, so a listener bound to a network
// interface hands the mutation path to every host that can reach the port,
// behind nothing but a random per-startup token that was never meant to carry
// a perimeter alone. The refusal is the cheap half of the threat model: a
// non-loopback bind is always a decision, so it fails here rather than being
// weighed later.
func TestServeRefusesANonLoopbackAddr(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:8080", "localhost:9999", "[::1]:8080", "127.0.0.1:0"} {
		if err := validateServeAddr(ok); err != nil {
			t.Errorf("loopback %q was refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"0.0.0.0:8080", ":8080", "192.168.1.5:8080", "[::]:8080", "8080"} {
		if err := validateServeAddr(bad); err == nil {
			t.Errorf("non-loopback %q was accepted; it would expose the config write "+
				"path to the network", bad)
		}
	}
}

// TestServeTokenIsPerStartupAndCheckedExactly pins the two facts the token's
// whole job rests on: a fresh server gets a fresh token, and a submitted token
// must match in full — any prefix or suffix difference is a reject.
func TestServeTokenIsPerStartupAndCheckedExactly(t *testing.T) {
	a, err := newServeToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newServeToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two startups drew the same token; the write gate would be reusable")
	}
	if len(a) != 32 { // 16 bytes, hex
		t.Fatalf("token is %d chars, want 32", len(a))
	}

	s := &squadServer{token: a}
	if !s.tokenOK(a) {
		t.Fatal("the correct token was rejected")
	}
	for _, bad := range []string{"", a[:31], a + "0", strings.ToUpper(a), "0" + a[1:]} {
		if s.tokenOK(bad) {
			t.Errorf("token %q was accepted; only the exact token may pass", bad)
		}
	}
}

// TestServeAnswers404OffTheTwoRoutes. Every other path must 404 rather than
// fall through to the page — a served page at every URL would be a second,
// undiscovered surface for whatever the page later learns to do.
func TestServeAnswers404OffTheTwoRoutes(t *testing.T) {
	s := &squadServer{}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/other", nil))
	if w.Code != 404 {
		t.Fatalf("GET /other answered %d, want 404", w.Code)
	}
}
