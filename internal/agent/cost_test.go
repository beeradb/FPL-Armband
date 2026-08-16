package agent

import (
	"testing"
	"time"
)

// A realistic 20-iteration run: a large stable prefix read from cache each
// time, written once, with modest fresh input and output per call.
func sampleUsage(model string) Usage {
	return Usage{Model: model, Iterations: 20,
		Input: 40_000, Output: 25_000, CacheWrite: 12_000, CacheRead: 900_000}
}

func TestCostAndCacheAccounting(t *testing.T) {
	u := sampleUsage("claude-opus-5")

	cost, ok := u.Cost()
	if !ok {
		t.Fatal("no pricing for claude-opus-5")
	}
	uncached, _ := u.CostWithoutCache()
	if uncached <= cost {
		t.Errorf("caching should reduce cost: cached $%.3f vs uncached $%.3f", cost, uncached)
	}
	if hr := u.CacheHitRate(); hr < 0.9 {
		t.Errorf("cache hit rate %.2f lower than expected for this shape", hr)
	}
	if _, ok := (&Usage{Model: "not-a-model"}).Cost(); ok {
		t.Error("unknown model should not return a cost")
	}
	t.Logf("opus with caching $%.3f, without $%.3f", cost, uncached)
}

// Sonnet 5 carries introductory pricing that expires. The reported cost must
// track the rates actually in force, and must revert afterwards rather than
// quietly under-reporting forever.
func TestIntroPricingIsDateAware(t *testing.T) {
	u := sampleUsage("claude-sonnet-5")

	during := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	lastDay := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	after := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)

	introCost, _ := u.CostOn(during)
	lastDayCost, _ := u.CostOn(lastDay)
	standardCost, _ := u.CostOn(after)

	if introCost >= standardCost {
		t.Errorf("intro pricing should undercut standard: $%.3f vs $%.3f", introCost, standardCost)
	}
	if lastDayCost != introCost {
		t.Errorf("the final intro day should still bill at intro rates: $%.3f vs $%.3f",
			lastDayCost, introCost)
	}

	// Intro rates are $2/$10 against standard $3/$15 — a two-thirds ratio.
	if ratio := introCost / standardCost; ratio < 0.6 || ratio > 0.7 {
		t.Errorf("intro/standard ratio %.3f is not consistent with $2/$10 vs $3/$15", ratio)
	}

	// Opus has no promotion, so its price must not vary by date.
	o := sampleUsage("claude-opus-5")
	a, _ := o.CostOn(during)
	b, _ := o.CostOn(after)
	if a != b {
		t.Errorf("opus has no intro pricing but varied by date: $%.3f vs $%.3f", a, b)
	}

	t.Logf("sonnet intro $%.3f, standard $%.3f; opus $%.3f", introCost, standardCost, a)
}
