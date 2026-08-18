# Review: shipping the measured configuration

**What was reviewed:** the shipped-default change on the user's ruling — `bank_transfers_lookahead`
on, `review_policy.early_floor` {1.0, 0.2} through GW8 (configurable off), `EffectiveFloor` on
the live path, and the record line in `AGENTS.md`. Commit range
`origin/development..d4afeb4` on branch `ship-the-measured-levers-configuration`.

**Reviewers:**

- **fpl-code-review — ran**, on the config persistence + agent wiring: found the missing
  `hasKey` migration the codebase's own comment mandated, the banking-rule divergence, the
  unmapped `armband backtest`, the stale display surfaces, and the unvalidated schedule
  values — six findings below.
- **fpl-security-review — covered by the code review** on this surface: the new live input
  (`early_floor`) is validated in `Load` (non-positive charge/gain refused, the same guards
  the flat constants already carry); no new network or persistence surface was added.
- **fpl-findings-audit — the code review verified the AGENTS.md line figure-by-figure** against
  both findings (every number matches its source exactly, "ships ON by the user's ruling,
  accepted rather than resolved" honest against the finding's own words). No separate pass
  owed.

**Findings, ranked by severity:**

1. `BankTransfersLookahead: true` shipped without the `hasKey` migration the field's own
   comment mandates — every pre-existing config file would silently gain the lever.
2. The accuracy snapshot's key predated the final tree (guard failing).
3. The live banking rule (`adviseBanking`) still priced the flat 2.0/0.4 while the swap tool
   and the replay used the schedule.
4. `armband backtest` silently replayed the flat machine with the shipped schedule — a
   byte-identical null for anyone editing the knob.
5. The system prompt and the brief printed the flat bar while the tools applied the schedule.
6. A written negative `early_floor` charge survived Load and would make the gate accept
   anything through GW8.

**What was applied:** all six, verified before editing. 1: key-probe migration — absent key
loads OFF (old files keep their behaviour), the shipped file's key reads ON, and the
banking-settings pin test now pins the new contract. 2: snapshot re-rendered over the final
tree (`2026-08-18-d4afeb4`). 3: `adviseBanking` applies `EffectiveFloor` before the taper —
schedule first, curve second, matching `decide` and the swap tool. 4: `armband backtest`
maps `EarlyFloor` (the measurement `sweepConfig` deliberately does not — the sweep baseline
stays the flat machine every banked cell was measured on, stated in both places). 5: the
prompt prints the imminent week's scheduled bar with the schedule stated; the brief lists the
flat pair plus the early-floor row. 6: `Load` value-guards the written schedule.

**What was declined:** nothing. The reviewer's note that `FPL_WEIGHT=free_transfer_value` is
masked through GW8 on the live path stands as recorded behaviour — the knob sweeps the replay,
which stays flat by design.

**What could not be checked on this harness:** the shipped configuration's live-season
behaviour (GW1 of 2026-27 has not happened); the schedule's points value remains unresolved
as its finding records.
