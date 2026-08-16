package analysis

import "fmt"

// Price forecasts, supplied rather than computed.
//
// FPL changes prices overnight on net transfers, and third parties — LiveFPL,
// Fantasy Football Fix and others — run calibrated models against the real
// algorithm with far more data than this program can reconstruct. They publish
// the decision-relevant quantity directly: the probability a given player
// changes price *tonight*.
//
// So the review layer reads one and tells the model, the same way it supplies
// team news. A homegrown estimate from public transfer flow was built and
// deleted: it predicted direction well enough (+0.26 correlation, monotone
// across deciles) and could not express "82% to rise tonight", which is the
// only form of the answer that decides whether to act before the deadline or
// after it.
//
// # This does not touch scoring
//
// A rising price says nothing about how many points someone will return; a term
// that rewarded it would be chasing popularity. It changes *when* to act, not
// *whether*: if a transfer is going to be made anyway, making it before a rise
// saves the rise, and before a fall saves the fall. Nothing here reaches Score.
//
// # It is never persisted
//
// A forecast is true for one evening. Writing it to config would be the stale-
// data failure this codebase keeps meeting — a price prediction from last
// Tuesday is worse than none, because it argues for urgency that has already
// expired. It lives on the engine for the length of a run and dies with it.
type PriceForecast struct {
	// Direction is "rise" or "fall".
	Direction string
	// Probability is the source's own confidence for tonight, 0-1. Zero when the
	// source gives a flag rather than a number.
	Probability float64
	// Source is who said so, e.g. "LiveFPL". Reported so a recommendation can be
	// audited rather than taken on faith.
	Source string
}

// Urgent reports whether a forecast is confident enough to change timing.
//
// The bar is deliberately high. Acting early on a rise that does not happen
// costs a transfer made a day sooner than it needed to be, which is usually
// harmless; acting early on a *false* signal repeatedly is how a policy churns.
func (p PriceForecast) Urgent() bool { return p.Probability >= 0.75 }

// Note renders the forecast for a human or the agent.
func (p PriceForecast) Note() string {
	conf := ""
	if p.Probability > 0 {
		conf = fmt.Sprintf(" (%.0f%%)", p.Probability*100)
	}
	src := ""
	if p.Source != "" {
		src = ", per " + p.Source
	}
	switch p.Direction {
	case "rise":
		return fmt.Sprintf("price expected to RISE tonight%s%s — buying now saves it", conf, src)
	case "fall":
		return fmt.Sprintf("price expected to FALL tonight%s%s — selling now avoids it", conf, src)
	}
	return ""
}

// SetPriceForecasts replaces what the review layer has been told about tonight's
// price changes. Keyed by element id.
//
// Copy-on-write under a lock: tool calls run concurrently through an errgroup,
// so a reader may hold the previous map while this runs.
func (e *Engine) SetPriceForecasts(f map[int]PriceForecast) {
	m := make(map[int]PriceForecast, len(f))
	for id, v := range f {
		m[id] = v
	}
	e.priceMu.Lock()
	e.priceForecasts = m
	e.priceMu.Unlock()
}

// PriceForecast returns what the review layer was told about a player, if
// anything.
func (e *Engine) PriceForecast(id int) (PriceForecast, bool) {
	e.priceMu.RLock()
	defer e.priceMu.RUnlock()
	v, ok := e.priceForecasts[id]
	return v, ok
}

// PriceForecastCount is how many forecasts are loaded, for reporting whether the
// review layer actually checked.
func (e *Engine) PriceForecastCount() int {
	e.priceMu.RLock()
	defer e.priceMu.RUnlock()
	return len(e.priceForecasts)
}
