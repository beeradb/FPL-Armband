package viewmodel

import (
	"fmt"
	"math"
	"reflect"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
)

// Input is everything Build needs. It is a struct rather than six parameters because the
// callers are a request handler and, later, a Wails binding, and both should be able to
// see at the call site which of these they have forgotten.
type Input struct {
	// Page is the assembled squad page. See the package comment for why this package
	// currently reads the HTML renderer's view model rather than assembling its own.
	Page present.Page
	// Boot supplies the gameweek deadlines, which are the one thing the rail needs and
	// the page does not carry.
	Boot *fpl.Bootstrap
	Cfg  config.Config
	// Now must be the same instant the page was built with, or the staleness figures in
	// the overrides will have been decided against a different clock than the one the
	// client is told about.
	Now time.Time
	// Persist reports whether writes go to config.json rather than the browser session.
	Persist bool

	// Session is what the reader has arranged and corrected, carried through so the page
	// can draw its own controls from it rather than keeping a second idea of them. Zero
	// for a caller with no session, which is every caller but the HTTP one.
	Session Session
	// Saved reports that the fifteen came from a stored team rather than being chosen
	// now, and Optimised that it is the model's best rather than a varied opening. They
	// are separate because a saved team may or may not have started as the optimum, and
	// the page says something different about each.
	Saved     bool
	Optimised bool
}

// Build translates an assembled page into the client contract.
//
// It returns an error rather than a best effort, and the only error it can raise is a
// non-finite number. That is not defensive: encoding/json REFUSES to marshal NaN or
// +Inf, so a single bad float turns the whole document into a 500 with a message about
// "unsupported value" and no indication of which player caused it. Every score here is a
// float division somewhere upstream, and a player with zero expected minutes is a
// division this model does perform. Failing here names the field.
func Build(in Input) (*State, error) {
	p := in.Page

	s := &State{
		Now:     in.Now,
		Horizon: p.Horizon,
		Clubs:   p.Teams,
		Session: Session{
			Store:     "session",
			Writable:  p.Token != "",
			Locked:    in.Session.Locked,
			Blocked:   in.Session.Blocked,
			Chips:     in.Session.Chips,
			Saved:     in.Saved,
			Optimised: in.Optimised,
		},
	}
	if in.Persist {
		s.Session.Store = "config"
	}

	s.Squad = buildSquad(p)
	s.Gameweeks = buildGameweeks(p, in.Boot)
	s.Market = buildMarket(p)
	s.Overrides = buildOverrides(p)
	if p.Reasoning != nil {
		s.Blind = p.Reasoning.Blind
		s.Policy = Policy{
			MinGainTransfer: p.Reasoning.Policy.MinGainTransfer,
			MinGainHit:      p.Reasoning.Policy.MinGainHit,
			BankUpTo:        p.Reasoning.Policy.BankUpTo,
			MaxHitsPerWeek:  p.Reasoning.Policy.MaxHitsPerWeek,
			Rules:           p.Reasoning.Policy.Rules,
		}
	}

	if err := checkFinite(s); err != nil {
		return nil, err
	}
	return s, nil
}

func buildSquad(p present.Page) Squad {
	sq := Squad{
		Formation:  p.Squad.Formation,
		Captain:    p.Squad.Captain.ID,
		Vice:       p.Squad.ViceCaptain.ID,
		XIScore:    p.Squad.XIScore,
		Expected:   p.Squad.ExpectedPoints,
		Cost:       p.Squad.TotalCost,
		Bank:       p.Squad.Remaining,
		ClubCounts: p.Squad.ClubCounts,
	}
	for _, m := range p.Squad.Players {
		sq.Players = append(sq.Players, buildPlayer(m, p))
	}
	for _, m := range p.Squad.StartingXI {
		sq.XI = append(sq.XI, m.ID)
	}
	for _, m := range p.Squad.Bench {
		sq.Bench = append(sq.Bench, m.ID)
		// A plain sum of reported scores — what a bench boost would pay. See the
		// warning on Squad.BenchScore: this is deliberately not the bench's value
		// to the optimiser, which weights each slot by the chance it is used.
		sq.BenchScore += m.Score
	}
	return sq
}

// buildPlayer turns one PlayerMetrics into the client's Player.
//
// Every field is a copy. If a value needs working out, it is worked out in
// internal/analysis and copied here — that is the whole discipline of this package.
func buildPlayer(m analysis.PlayerMetrics, p present.Page) Player {
	pl := Player{
		ID:            m.ID,
		Code:          p.Codes[m.ID],
		Name:          m.Name,
		Club:          m.Team,
		Pos:           m.Position,
		Price:         m.Price,
		XP:            m.Score,
		Per90:         m.FixtureAdjXP90,
		Minutes:       m.ExpectedMinutes,
		Role:          m.RotationRisk,
		Reliability:   m.MinutesRating,
		StartShare:    m.StartShare,
		Ownership:     m.Ownership,
		Status:        m.Status,
		News:          m.News,
		Availability:  m.AvailabilityFactor,
		AvgDifficulty: m.AvgDifficulty,
	}
	for _, f := range m.Fixtures {
		pl.Fixtures = append(pl.Fixtures, Fixture{
			Gameweek:   f.Event,
			Opponent:   f.Opponent,
			Home:       f.Home,
			Difficulty: f.Difficulty,
		})
	}
	if ov, ok := p.Overrides[m.ID]; ok {
		o := convertOverride(ov)
		pl.Override = &o
	}
	return pl
}

// buildGameweeks joins the week views the build produced to the deadlines the bootstrap
// carries.
//
// The rail's length is whatever the build was asked for. Nothing here caps it: the design
// note is explicit that the planning window slides and must not be hard-coded, and a rail
// that silently stopped at five would start lying the first time a chip was planned
// outside it — which is most of why anyone opens the page.
func buildGameweeks(p present.Page, boot *fpl.Bootstrap) []Gameweek {
	deadline := map[int]time.Time{}
	current := 0
	if boot != nil {
		for _, e := range boot.Events {
			deadline[e.ID] = e.DeadlineTime
			// IsCurrent is the week being played; IsNext is the one taking entries.
			// Preferring IsCurrent matches what the rail's NOW dot means to a reader
			// mid-week, and IsNext is the answer for the rest of the time.
			if e.IsCurrent {
				current = e.ID
			} else if e.IsNext && current == 0 {
				current = e.ID
			}
		}
	}
	out := make([]Gameweek, 0, len(p.Weeks))
	for _, w := range p.Weeks {
		var playable []ChipOption
		for _, key := range analysis.PlayableChips(w.Event) {
			playable = append(playable, ChipOption{Key: key, Label: analysis.ChipLabel(key)})
		}
		out = append(out, Gameweek{
			Number:    w.Event,
			Deadline:  deadline[w.Event],
			Current:   w.Event == current,
			Chip:      w.Chip,
			Projected: w.Expected,
			Formation: w.Formation,
			Rebuilt:   w.Rebuilt,
			Playable:  playable,
		})
	}
	return out
}

func buildMarket(p present.Page) Market {
	var m Market
	if p.Watch == nil {
		return m
	}
	m.Count = p.Watch.Count
	m.Clearing = p.Watch.Clearing
	m.Gate = p.Watch.Gate
	for _, r := range p.Watch.Rows {
		m.Rows = append(m.Rows, MarketRow{
			Player:     buildPlayer(r.Player, p),
			Delta:      r.Delta,
			ClearsGate: r.ClearsGate,
		})
	}
	for _, b := range p.Watch.Benchmarks {
		m.Benchmarks = append(m.Benchmarks, Benchmark{
			Pos:   b.Position,
			Name:  b.Name,
			Score: b.Score,
			Price: b.Price,
		})
	}
	for _, o := range p.Watch.Excluded {
		m.Excluded = append(m.Excluded, convertOverride(o))
	}
	return m
}

func buildOverrides(p present.Page) Overrides {
	var o Overrides
	if p.Reasoning == nil {
		return o
	}
	for _, v := range p.Reasoning.Overrides {
		o.Live = append(o.Live, convertOverride(v))
	}
	for _, v := range p.Reasoning.Lapsed {
		o.Lapsed = append(o.Lapsed, convertOverride(v))
	}
	// Due and Oldest are asked of the renderer's own type rather than recounted, so the
	// count on the page and the count in the API cannot disagree.
	o.Due = p.Reasoning.Due()
	o.Oldest = p.Reasoning.Oldest()
	return o
}

func convertOverride(o present.Override) Override {
	return Override{
		Kind:         o.Kind,
		Code:         o.Code,
		Session:      o.Session,
		Label:        o.Label,
		Reason:       o.Reason,
		Player:       o.Player,
		Club:         o.Team,
		Pos:          o.Pos,
		SetOn:        o.SetOn,
		Until:        o.Until,
		Checked:      o.Checked,
		NeedsCheck:   o.NeedsCheck,
		CheckAge:     o.CheckAge,
		NeverChecked: o.NeverChecked,
		Lapsed:       o.Lapsed,
		Inherited:    o.Inherited,
		InSquad:      o.InSquad,
		Affects:      o.AffectsInSquad,
		Flag:         o.Flag(),
	}
}

// checkFinite walks the built state and refuses any float that JSON cannot carry.
//
// It walks by reflection rather than checking the fields it knows about, because the
// fields it knows about are exactly the ones somebody remembered. A new projection added
// to Player two months from now is caught by this and would not be caught by a list.
//
// The path in the error is the field chain, not the player's name: the point is to send
// whoever reads the 500 to the line of Go that produced the number.
func checkFinite(v any) error {
	return walkFinite(reflect.ValueOf(v), "")
}

func walkFinite(v reflect.Value, path string) error {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		if f := v.Float(); math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("viewmodel: %s is %v, which encoding/json cannot marshal "+
				"— the whole document would fail rather than this one field", path, f)
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return walkFinite(v.Elem(), path)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if !t.Field(i).IsExported() {
				continue
			}
			if err := walkFinite(v.Field(i), path+"."+t.Field(i).Name); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := walkFinite(v.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if err := walkFinite(iter.Value(), fmt.Sprintf("%s[%v]", path, iter.Key())); err != nil {
				return err
			}
		}
	}
	return nil
}
