package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// Pricing is per-million-token rates for a model, in US dollars.
//
// Some models carry promotional pricing that expires on a date. Intro rates are
// held separately rather than baked into Input/Output so a run is always costed
// at what it will actually be billed, and so the rate silently reverts to
// standard once the promotion lapses instead of under-reporting forever.
type Pricing struct {
	Input  float64
	Output float64

	// IntroInput and IntroOutput apply until IntroUntil, if set.
	IntroInput  float64
	IntroOutput float64
	IntroUntil  string // YYYY-MM-DD, inclusive
}

// rates returns the rates in force on the given day, and whether the
// promotional pricing was the one applied.
func (p Pricing) rates(on time.Time) (in, out float64, intro bool) {
	if p.IntroUntil == "" || p.IntroInput == 0 {
		return p.Input, p.Output, false
	}
	until, err := time.Parse("2006-01-02", p.IntroUntil)
	if err != nil {
		return p.Input, p.Output, false
	}
	// Inclusive of the final day.
	if on.After(until.AddDate(0, 0, 1)) {
		return p.Input, p.Output, false
	}
	return p.IntroInput, p.IntroOutput, true
}

// modelPricing holds first-party Claude API rates. Cache writes bill at 1.25x
// the input rate and cache reads at 0.1x, which is where nearly all the saving
// in a long tool-calling loop comes from.
var modelPricing = map[string]Pricing{
	"claude-opus-5":   {Input: 5.00, Output: 25.00},
	"claude-opus-4-8": {Input: 5.00, Output: 25.00},
	"claude-opus-4-7": {Input: 5.00, Output: 25.00},
	"claude-sonnet-5": {
		Input: 3.00, Output: 15.00,
		IntroInput: 2.00, IntroOutput: 10.00, IntroUntil: "2026-08-31",
	},
	"claude-sonnet-4-6": {Input: 3.00, Output: 15.00},
	"claude-haiku-4-5":  {Input: 1.00, Output: 5.00},
	"claude-fable-5":    {Input: 10.00, Output: 50.00},
}

const (
	cacheWriteMultiplier = 1.25
	cacheReadMultiplier  = 0.10
)

// Usage accumulates token counts across every iteration of a tool-calling run.
// The API reports usage per request, and an agentic loop makes one request per
// iteration, so the per-run total is the sum rather than the final message.
type Usage struct {
	Model       string
	Iterations  int
	Input       int64 // uncached input
	Output      int64
	CacheWrite  int64
	CacheRead   int64
	WebSearches int64
}

func (u *Usage) Add(m *anthropic.BetaMessage) {
	if m == nil {
		return
	}
	u.Iterations++
	u.Input += m.Usage.InputTokens
	u.Output += m.Usage.OutputTokens
	u.CacheWrite += m.Usage.CacheCreationInputTokens
	u.CacheRead += m.Usage.CacheReadInputTokens
	u.WebSearches += m.Usage.ServerToolUse.WebSearchRequests
}

// Cost estimates the dollar cost of the run at the rates in force today.
// Returns ok=false for a model with no published rate, so callers can avoid
// printing a wrong number.
func (u *Usage) Cost() (float64, bool) {
	return u.CostOn(time.Now())
}

// CostOn prices the run at the rates in force on a given date.
func (u *Usage) CostOn(on time.Time) (float64, bool) {
	p, ok := modelPricing[u.Model]
	if !ok {
		return 0, false
	}
	inRate, outRate, _ := p.rates(on)
	perToken := inRate / 1_000_000
	cost := float64(u.Input)*perToken +
		float64(u.CacheWrite)*perToken*cacheWriteMultiplier +
		float64(u.CacheRead)*perToken*cacheReadMultiplier +
		float64(u.Output)*(outRate/1_000_000)
	return cost, true
}

// IntroPricing reports whether today's cost uses promotional rates, and when
// they lapse — so a quoted figure is never mistaken for the standing price.
func (u *Usage) IntroPricing() (until string, active bool) {
	p, ok := modelPricing[u.Model]
	if !ok {
		return "", false
	}
	if _, _, intro := p.rates(time.Now()); intro {
		return p.IntroUntil, true
	}
	return "", false
}

// CacheHitRate is the share of prompt tokens served from cache. A long run with
// a healthy cache should sit well above 0.8; near zero means something in the
// prefix is changing between requests and the cache is being rebuilt each time.
func (u *Usage) CacheHitRate() float64 {
	total := u.Input + u.CacheWrite + u.CacheRead
	if total == 0 {
		return 0
	}
	return float64(u.CacheRead) / float64(total)
}

// Summary is a one-block report suitable for printing after a run.
func (u *Usage) Summary() string {
	var b strings.Builder
	promptTokens := u.Input + u.CacheWrite + u.CacheRead

	fmt.Fprintf(&b, "%d API calls  ·  %s prompt tokens (%s cached, %.0f%% hit rate)  ·  %s output",
		u.Iterations, humanTokens(promptTokens), humanTokens(u.CacheRead),
		u.CacheHitRate()*100, humanTokens(u.Output))
	if u.WebSearches > 0 {
		fmt.Fprintf(&b, "  ·  %d web searches", u.WebSearches)
	}
	if cost, ok := u.Cost(); ok {
		fmt.Fprintf(&b, "\nestimated cost: $%.3f", cost)
		if until, intro := u.IntroPricing(); intro {
			standard, _ := u.CostOn(mustDate(until).AddDate(0, 0, 2))
			fmt.Fprintf(&b, " at introductory rates through %s; $%.3f after", until, standard)
		}
		if uncached, uok := u.CostWithoutCache(); uok && uncached > cost {
			fmt.Fprintf(&b, "\nwithout prompt caching this run would have cost about $%.2f", uncached)
		}
	} else {
		fmt.Fprintf(&b, "\nestimated cost: unknown — no published rate for %q", u.Model)
	}
	return b.String()
}

// CostWithoutCache is the counterfactual cost had every cached token been billed
// at the full input rate, for showing what caching actually saved.
func (u *Usage) CostWithoutCache() (float64, bool) {
	p, ok := modelPricing[u.Model]
	if !ok {
		return 0, false
	}
	inRate, outRate, _ := p.rates(time.Now())
	perToken := inRate / 1_000_000
	all := u.Input + u.CacheWrite + u.CacheRead
	return float64(all)*perToken + float64(u.Output)*(outRate/1_000_000), true
}

func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Now()
	}
	return t
}
