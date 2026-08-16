package analysis

import "fmt"

// BudgetTrust says whether the money the model is working with is real.
//
// FPL pays you what you paid plus half of any price rise, never the market
// value. Without a session the model cannot see purchase prices and has to
// assume it can sell at market — which means it thinks it has money it does
// not, and recommends transfers that would be rejected at the deadline.
//
// This is deliberately a reported state rather than a log line. An unverified
// budget is silent, plausible and wrong: every number downstream still renders,
// the squad still looks affordable, and nothing about the output says the
// arithmetic rests on an assumption. The replay puts the cost at about 31
// points a season.
type BudgetTrust struct {
	// Verified is true when selling prices came from FPL.
	Verified bool
	// Reason explains why not, when not. Empty when verified.
	Reason string
	// Assumed is how much the model may believe it has and does not, in tenths.
	// Zero when unknown, which is the usual case — establishing it needs the
	// purchase prices that are missing in the first place.
	Assumed int
}

// VerifiedBudget is the state when the selling prices are known to be right.
//
// "Verified" means checked, not merely sourced. Reconstructed prices earn it by
// summing to the team value FPL itself reports; a private endpoint earns it by
// being the authority. Both are stronger than an assumption, and the
// reconstruction is the stronger of the two, because it can be proved wrong.
func VerifiedBudget() BudgetTrust { return BudgetTrust{Verified: true} }

// DriftingBudget is the state when a reconstruction did not reproduce FPL's own
// team value. The prices are close but not exact, so they are reported as an
// assumption: a squad that is £0.3m poorer than the model thinks will refuse a
// transfer at the deadline just as firmly as one that is £3m poorer.
func DriftingBudget(drift int) BudgetTrust {
	return BudgetTrust{
		Reason: fmt.Sprintf("Reconstructed selling prices are £%.1fm off FPL's own "+
			"team value, so at least one purchase price is wrong.", float64(drift)/10),
		Assumed: drift,
	}
}

// AssumedBudget is the state when it did not.
func AssumedBudget(reason string) BudgetTrust {
	return BudgetTrust{Reason: reason}
}

// Warning is a one-line statement of the problem, or "" when verified.
func (b BudgetTrust) Warning() string {
	if b.Verified {
		return ""
	}
	return fmt.Sprintf("BUDGET NOT VERIFIED — sales are priced at market value, "+
		"which overstates what this squad can raise. %s", b.Reason)
}

// Label is a short form for compact output, including tool JSON that is
// replayed on every subsequent API call.
func (b BudgetTrust) Label() string {
	if b.Verified {
		return "verified from FPL"
	}
	return "ASSUMED — sales priced at market, budget overstated"
}

// AssemblyBudget is the money a fifteen can be assembled with today, in tenths,
// and a short statement of where the figure came from.
//
// £100m is what you had in August. Once the season starts the number that
// answers "what is the best squad available to me" is the squad's selling value
// plus the bank — the wildcard budget — and it drifts from £100m in both
// directions: a squad whose players have risen can afford more, one that has
// bled value can afford less and is being shown players it cannot buy.
//
// # Why an unknown budget is an error and not a default
//
// The tempting fallback is £100m, and it is wrong for the same reason the bank
// defaulting to zero was wrong: the output still renders, the squad still looks
// legal, and nothing says the money is imaginary. Worse, here it fails in the
// expensive direction — a squad built with money you do not have is a
// recommendation you cannot act on at the deadline.
//
// So the two cases are separated by whether a squad is being tracked at all. No
// Entry means nobody's money is in question and HypotheticalBudget answers it.
// An Entry whose squad could not be priced is a broken input — an unreachable
// API or a wrong id — and says so.
func (e *Engine) AssemblyBudget() (tenths int, source string, err error) {
	switch {
	case e.SquadValue != nil && e.Bank != nil:
		v, b := *e.SquadValue, *e.Bank
		return v + b, fmt.Sprintf("£%.1fm squad value plus £%.1fm in the bank",
			float64(v)/10, float64(b)/10), nil

	case e.Entry != 0 && e.GameweeksPlayed() == 0:
		// Pre-season nothing has been bought, so the allowance is the budget and
		// it needs no reconstruction to know.
		return DefaultBudget, "the pre-season allowance, nothing having been bought yet", nil

	case e.Entry != 0:
		return 0, "", fmt.Errorf("cannot establish the budget for entry %d: the squad's "+
			"selling value and bank could not be read, so there is no way to know what it "+
			"can afford. Check that entry_id in config.json is correct and that the FPL "+
			"API is reachable", e.Entry)

	case e.HypotheticalBudget > 0:
		return e.HypotheticalBudget, fmt.Sprintf(
			"£%.1fm from config, with no entry_id to price a real squad",
			float64(e.HypotheticalBudget)/10), nil

	default:
		return DefaultBudget, "£100.0m, the standard allowance, with no entry_id " +
			"to price a real squad", nil
	}
}
