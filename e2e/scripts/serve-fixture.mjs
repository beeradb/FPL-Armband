#!/usr/bin/env node
// Primes a deterministic cache from the committed live capture, then starts `armband serve`
// against it — the one command Playwright's webServer runs, in both a local run and CI. See
// playwright.config.js's own comment on webServer for why this is deliberately not split into
// a CI step that backgrounds the binary and a local script that does something else.
import { spawn } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { primeCache } from './prime-cache.mjs';
import { LIVE_CAPTURE } from './live-capture.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '../..');
const e2eDir = path.resolve(__dirname, '..');

const PORT = process.env.ARMBAND_E2E_PORT || '8123';
const captureDir = path.join(repoRoot, 'data', 'captures', LIVE_CAPTURE);
const cacheDir = path.join(e2eDir, '.armband-cache');
const tokenFile = path.join(e2eDir, '.token');
const binary = process.env.ARMBAND_E2E_BINARY || path.join(repoRoot, 'armband');
const configPath = path.join(e2eDir, 'testdata', 'config.json');

// The line cmdServe prints (cmd/armband/serve.go): dim(`Serving the squad page on
// http://<addr>/?t=<token>`), where dim() is a no-op ANSI wrapper honouring NO_COLOR. Setting
// NO_COLOR below means this regex never has to strip an escape sequence.
const TOKEN_LINE = /Serving the squad page on http:\/\/[^\s]+\/\?t=([0-9a-f]+)/;

async function main() {
  const files = await primeCache(captureDir, cacheDir);
  console.error(`[serve-fixture] primed ${files.length} file(s) from ${LIVE_CAPTURE}`);

  const child = spawn(
    binary,
    ['-config', path.relative(repoRoot, configPath), '-cache-dir', path.relative(repoRoot, cacheDir),
     'serve', '-addr', `127.0.0.1:${PORT}`],
    {
      cwd: repoRoot,
      env: {
        ...process.env,
        TZ: 'UTC',
        NO_COLOR: '1',
        // Explicit, not merely unset: /gate refuses with 503 and records nothing. See
        // cmdServe's own comment on signupDSNEnv.
        ARMBAND_SIGNUPS_DSN: '',
        // The strongest guard this fixture has against quietly serving live data: if
        // priming ever misses a file, the client's next read falls through to a live
        // fetch, which now dies against a closed loopback port instead of succeeding.
        HTTP_PROXY: 'http://127.0.0.1:9',
        HTTPS_PROXY: 'http://127.0.0.1:9',
        NO_PROXY: '127.0.0.1,localhost',
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  );

  let tokenSeen = false;
  let stderrBuf = '';
  const onStderr = async (chunk) => {
    process.stderr.write(chunk);
    stderrBuf += chunk.toString('utf8');
    if (tokenSeen) return;
    const m = TOKEN_LINE.exec(stderrBuf);
    if (m) {
      tokenSeen = true;
      await mkdir(e2eDir, { recursive: true });
      await writeFile(tokenFile, m[1], 'utf8');
      console.error(`[serve-fixture] wrote token to ${path.relative(repoRoot, tokenFile)}`);
    }
  };
  child.stderr.on('data', onStderr);
  child.stdout.on('data', (chunk) => process.stdout.write(chunk));

  child.on('exit', (code, signal) => {
    if (!tokenSeen) {
      console.error(
        '[serve-fixture] the server exited before printing a line matching ' +
          `${TOKEN_LINE} — cmdServe's token line format may have changed. ` +
          `Captured stderr:\n${stderrBuf}`,
      );
    }
    process.exit(code === null ? (signal ? 1 : 0) : code);
  });
  child.on('error', (err) => {
    console.error(`[serve-fixture] failed to spawn ${binary}: ${err.message}`);
    process.exit(1);
  });

  const forward = (sig) => {
    if (!child.killed) child.kill(sig);
  };
  process.on('SIGINT', () => forward('SIGINT'));
  process.on('SIGTERM', () => forward('SIGTERM'));
}

main().catch((err) => {
  console.error(`[serve-fixture] ${err.stack || err.message}`);
  process.exit(1);
});
