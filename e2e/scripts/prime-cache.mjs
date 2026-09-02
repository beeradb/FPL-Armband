// Turns the committed live capture into the on-disk cache internal/fpl.Client reads before
// ever making a network call.
//
// The capture stores each endpoint gzipped, flattened the same way Client.get flattens a
// request path (internal/capture.FileName / the key derivation in client.go's get): a
// leading/trailing slash trimmed, interior slashes to underscores. Neither endpoint this
// suite needs (/bootstrap-static/, /fixtures/) has a "/" once trimmed, so priming is just
// "gunzip, drop the .gz, write it where the cache looks" — no re-flattening of our own,
// which would be a second implementation of a rule that already lives in Go.
import { createHash } from 'node:crypto';
import { gunzipSync } from 'node:zlib';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';

/**
 * Prime cacheDir from the manifest at captureDir. Throws, naming the file, on any hash
 * mismatch or missing entry — a corrupted or partial prime must never look like a cache
 * miss that quietly falls through to a live fetch against the dead proxy port.
 */
export async function primeCache(captureDir, cacheDir) {
  const manifestRaw = await readFile(path.join(captureDir, 'manifest.json'), 'utf8');
  const manifest = JSON.parse(manifestRaw);
  if (!Array.isArray(manifest.files) || manifest.files.length === 0) {
    throw new Error(`${captureDir}/manifest.json lists no files — nothing to prime`);
  }

  await mkdir(cacheDir, { recursive: true });

  const written = [];
  for (const entry of manifest.files) {
    if (entry.note) {
      // A capture records a failed endpoint as a note rather than dropping it (see
      // capture.Take's own comment) — real evidence that endpoint did not answer. The e2e
      // fixture has no use for a partial capture, so treat it as fatal rather than silently
      // priming fewer files than the manifest promises.
      throw new Error(
        `${captureDir}/manifest.json's ${entry.name} carries a note ("${entry.note}") ` +
          `instead of a body — this capture is partial and cannot prime a deterministic cache`,
      );
    }
    if (!entry.name.endsWith('.json.gz')) {
      throw new Error(`${captureDir}/manifest.json names ${entry.name}, which is not a .json.gz`);
    }
    const target = entry.name.slice(0, -'.gz'.length);
    const gz = await readFile(path.join(captureDir, entry.name));
    const body = gunzipSync(gz);

    const sha256 = createHash('sha256').update(body).digest('hex');
    if (sha256 !== entry.sha256) {
      throw new Error(
        `${entry.name} hashes to ${sha256}, manifest says ${entry.sha256} — the capture on ` +
          `disk does not match the one the manifest describes`,
      );
    }

    await writeFile(path.join(cacheDir, target), body);
    written.push(target);
  }
  return written;
}

// Allows `node prime-cache.mjs <captureDir> <cacheDir>` for a manual check.
if (import.meta.url === `file://${process.argv[1]}`) {
  const [, , captureDir, cacheDir] = process.argv;
  if (!captureDir || !cacheDir) {
    console.error('usage: prime-cache.mjs <captureDir> <cacheDir>');
    process.exit(2);
  }
  const files = await primeCache(captureDir, cacheDir);
  console.log(`primed ${files.length} file(s) into ${cacheDir}: ${files.join(', ')}`);
}
