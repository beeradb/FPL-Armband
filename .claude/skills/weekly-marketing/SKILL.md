---
name: weekly-marketing
description: Generate this week's FPL Armband social/marketing materials — currently the xG Watch over/underperformers graphic. Run once GW fixtures are mostly played; extend with new materials as they're built.
---

# Weekly marketing materials

This is a recipe, not a research finding — it does not belong in `docs/`, `README.md`,
`AGENTS.md` or the vault, and living under `.claude/skills/` keeps it outside the
docs-review hook's watched paths on purpose. No verdict, no detection threshold, no
docs review: this is a content-production runbook.

## Cadence

Once a week, after most (ideally all) of the current gameweek's fixtures have kicked
off. Check completeness before generating — see "Data correctness" below. Don't wait
for every gameweek to fully settle (bonus points, `finished=true`) if the goal is
same-week posting; just label what's provisional.

## Materials

### xG Watch (over/underperformers)

Top 10 xG overperformers and top 10 underperformers for the gameweek, as a branded
1080×1080 square for Instagram/Reddit/Twitter.

**Data — pull live, don't reuse cached numbers:**
- `internal/fpl.Client.Bootstrap(ctx)` for `Elements` (per-player `GoalsScored`,
  `ExpectedGoals`, `Minutes`, `WebName`, `Team`, `ElementType`) and `Teams` (short
  names) — see `internal/analysis/metrics.go`'s `Finishing` field (`goals_minus_xg`)
  for the same computation inside the scoring engine.
- **Population: any player with `Minutes > 0`.** Do NOT filter to ≥90 minutes — that
  was tried once and it silently cut the real extremes (a sub who scored from a
  0.09 xG half-chance ranks above a 90-minute starter who scored from 0.30, and a
  minutes floor hides that). Rank by `GoalsScored - ExpectedGoals.Float()` descending
  for overperformers, ascending for underperformers, take the top/bottom 10.
- **Check gameweek completeness before calling it final.** Pull `client.Fixtures(ctx)`,
  filter to the current event, and count `Finished`. If any fixture hasn't kicked off
  or is only `FinishedProvisional` (final whistle, stats not yet locked), the graphic
  is provisional — say so on it (which fixture, which kickoff), don't imply the
  gameweek is over.

**Copy — descriptive only, never predictive.** Label the columns "Overperforming" /
"Underperforming" — what happened. Do not claim a player is "due a correction," "about
to bounce back," or similar — one gameweek of finishing variance is not evidence about
an individual player's next match, and this project's own standing rules already
reject that move for the scoring model itself (a measured bias does not imply a
correction exists). If the framing implies the numbers predict anything, rewrite it.

**Brand — pull from the shipped design system, don't invent a palette:**
- `internal/webui/assets/static/armband.css` — dark HUD tokens: `--bg #070B10`,
  `--panel #10171F`, `--band #2F53F0` (armband blue), `--band-hi #6A85FF`, `--acc
  #00E88A` (positive — used for overperforming), `--bad #FF4D6A` (negative — used for
  underperforming), `--ink`/`--ink2`/`--ink3` for text.
- `internal/webui/assets/static/logo.svg` — the armband-and-C mark. Inline its `<path>`
  elements directly (it's small and self-contained); don't reference the file by URL,
  since a published Artifact page blocks external requests.
- Display font falls back to system-ui (Plus Jakarta Sans/Inter aren't loadable in a
  self-contained Artifact page under its CSP — this is an accepted approximation, not a
  bug).
- A generic dataviz palette (the `dataviz` skill's default blue/red diverging pair) is
  the right choice for a *neutral analytics chart*; it is the wrong choice here — this
  is client-facing brand content and should look like the product.

**Build & ship:**
1. Pull data with a throwaway Go program under `cmd/` (imports `internal/fpl` +
   `internal/analysis`), or by hand — never commit the throwaway program, delete it
   before finishing.
2. Write the graphic as a self-contained HTML file (inline SVG logo, inline CSS, no
   external fonts/images) sized to the target canvas (1080×1080 for IG/Reddit-safe
   square; ask before building platform-specific variants — see below).
3. Screenshot it to check layout before publishing: `chromium --headless
   --disable-gpu --no-sandbox --screenshot=<path> --window-size=1080,1080
   --hide-scrollbars file://<path>`. Snap-confined chromium can't write under
   `~/.claude/`, so write the HTML and the screenshot to `$HOME` (or another
   snap-visible path) and copy back afterward. Check both light content (this brand is
   dark-only, so no light-mode pass needed) and that no label/name overflows or
   collides — widen columns or reduce a bar-reach constant rather than letting text
   clip.
4. Publish via the `Artifact` tool (`favicon: "⚽"`, stable across weeks) and also
   `SendUserFile` the rendered PNG so it's ready to post without extra steps.

### Other materials

None yet. When a new recurring marketing asset is built, add its own `###` section
here with the same three parts: data source (live, not cached/invented), brand source
(pull from `armband.css`/`logo.svg`, don't reinvent), and copy constraints (what claims
are and aren't supportable). Don't invent placeholder materials ahead of actually
building them.
