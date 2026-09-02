// Reads the capture manifest so the determinism assertion in smoke-boot.spec.js moves with
// the capture automatically rather than hardcoding a gameweek number that would rot the day
// LIVE_CAPTURE is repointed.
//
// Runs once, in Playwright's main process, before any worker starts — so the env vars set
// here are inherited by every worker Playwright spawns afterwards. That is the documented way
// to hand data from globalSetup to tests; a written file would work too but would be a second
// place to keep in sync with this one.
const path = require('path');
const { LIVE_CAPTURE } = require('./scripts/live-capture.js');

module.exports = async function globalSetup() {
  // prime-cache.mjs is an ES module (it shares code with serve-fixture.mjs, also ESM); this
  // file is CommonJS like the rest of the Playwright config surface, so it reaches it through
  // a dynamic import rather than `require`.
  const { readManifest } = await import('./scripts/prime-cache.mjs');
  const repoRoot = path.resolve(__dirname, '..');
  const captureDir = path.join(repoRoot, 'data', 'captures', LIVE_CAPTURE);
  const manifest = await readManifest(captureDir);

  if (!manifest.event || !manifest.event_deadline) {
    throw new Error(
      `${captureDir}/manifest.json carries no event/event_deadline — smoke-boot.spec.js's ` +
        'determinism control has nothing to compare /api/state against',
    );
  }

  process.env.ARMBAND_E2E_EXPECT_GAMEWEEK = String(manifest.event);
  process.env.ARMBAND_E2E_EXPECT_DEADLINE = manifest.event_deadline;
};
