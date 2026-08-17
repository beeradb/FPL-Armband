package main

import (
	"fmt"

	"armband/internal/backtest"
	"armband/internal/config"
)

// applyOptionValue carries the option-value block from the config file onto a
// replay config.
//
// # Why this exists as a named function
//
// `SimConfig` is built from `config.Config` field by field, and a field with no
// line here is a setting that never arrives: the user turns a lever on, runs
// `armband backtest`, and gets a byte-identical result from a knob the replay
// never saw. That is the standing trap this project checks mediators for, produced
// by the very feature that added the mediators — and it is exactly what
// `BankLookahead` and `PrepareForChips` already have a paragraph about at the call
// site.
//
// One function rather than nine lines inline, so
// `TestTheOptionValueBlockReachesTheReplay` can scan a single body and name every
// field that must appear in it. A mapping spread through a struct literal cannot
// be checked that way, which is how a tenth field would go missing.
//
// ⚠️ **It maps `Enabled` and `Bar` separately and both matter.** `SimConfig`'s bar
// fields read a literal zero as a bar of zero — deliberately, since a sweep has no
// file to backfill from — while `config.ChipTrigger`'s zero means the default. The
// backfill in `config.Load` is what reconciles them, so this may copy the value
// straight across; if that backfill is ever removed, a bar of 0 arrives here as
// "play it the first week it is worth anything", which is a real setting and a
// wrong one to reach by accident.
func applyOptionValue(c *backtest.SimConfig, p config.OptionValuePolicy) {
	c.OptionPricing = p.Pricing
	c.TaperFreeTransferValue = p.TaperFreeTransferValue
	c.WildcardTrigger, c.WildcardReservation = p.Wildcard.Enabled, p.Wildcard.Bar
	c.BenchBoostTrigger, c.BenchBoostBar = p.BenchBoost.Enabled, p.BenchBoost.Bar
	c.FreeHitTrigger, c.FreeHitBar = p.FreeHit.Enabled, p.FreeHit.Bar
}

// noteUnhonouredLevers is what a LIVE command prints when a lever is on that it
// cannot act on.
//
// # A limitation stated is not the same failure as a limitation hidden
//
// The free-transfer taper has three live consumers and reaches all of them. The
// three CHIP state rules have none: there is no live "should I play a chip this
// week" surface at all — `armband transfers` recommends moves, `brief` describes a
// squad, and the chip plan is a calendar the user writes. So a chip trigger set in
// `config.json` genuinely does nothing outside a replay.
//
// That is a gap in the feature and it must not be a silent one. The rule this
// project keeps paying for is that a setting which quietly means nothing is
// indistinguishable from a setting that means what it says, so the honest form of
// "unimplemented here" is a line of output rather than an omission.
//
// ⚠️ It reports rather than refusing. Refusing would make a config that replays
// correctly unusable for the commands it does not concern, and the levers are
// deliberately independent — a user measuring a chip trigger in the replay still
// wants `armband transfers` to run.
func noteUnhonouredLevers(p config.OptionValuePolicy) string {
	on := p.ChipTriggersOn()
	if len(on) == 0 {
		return ""
	}
	return fmt.Sprintf("option_value: the %v chip trigger(s) are on and this "+
		"command cannot act on them — there is no live chip decision, only the "+
		"planned calendar in chip_plan. They reach `armband backtest` and the "+
		"replay only.", on)
}
