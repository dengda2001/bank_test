// Single Chromium resolution entry point for the audit harness.
//
// Every script must launch the browser through this module, so there is exactly
// one place that knows how to find a browser. Resolution order:
//
//   1. `CHROME_BIN`, if set. If it is set but points at nothing, that is an
//      error — the user asked for a specific binary and silently using a
//      different one would hide the mistake.
//   2. Well-known macOS Chrome install path (/Applications/Google Chrome.app).
//   3. `chromium`, `chromium-browser`, `google-chrome`, `google-chrome-stable`
//      on PATH.
//
// If nothing resolves, the process exits with an actionable message. There is
// deliberately no fallback to Playwright's bundled browser: a missing browser
// must be a loud failure, not a silent switch to a build the audit did not run
// against.

import fs from 'node:fs';
import { execFileSync } from 'node:child_process';
import { chromium } from 'playwright';

const MACOS_CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const PATH_CANDIDATES = ['chromium', 'chromium-browser', 'google-chrome', 'google-chrome-stable'];

function onPath(bin) {
  try {
    const out = execFileSync('which', [bin], { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
    return out || null;
  } catch {
    return null;
  }
}

function fail(message) {
  console.error('launch.mjs: ' + message);
  console.error('');
  console.error('Set CHROME_BIN to a Chrome/Chromium executable, e.g.:');
  console.error('  export CHROME_BIN="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"');
  process.exit(1);
}

// Returns { executablePath, source }. Exits the process if no browser is found.
export function resolveChrome() {
  if (process.env.CHROME_BIN) {
    const bin = process.env.CHROME_BIN;
    if (!fs.existsSync(bin)) {
      fail(`CHROME_BIN is set to "${bin}" but that path does not exist.`);
    }
    return { executablePath: bin, source: 'CHROME_BIN' };
  }

  if (fs.existsSync(MACOS_CHROME)) {
    return { executablePath: MACOS_CHROME, source: 'macOS Google Chrome' };
  }

  for (const bin of PATH_CANDIDATES) {
    const found = onPath(bin);
    if (found) return { executablePath: found, source: `PATH (${bin})` };
  }

  fail('no Chrome/Chromium executable found (checked CHROME_BIN, macOS Chrome, and PATH).');
}

// Options object for `chromium.launch(...)` / `chromium.launchPersistentContext(...)`.
// Callers pass through anything extra they need; the resolved executable and the
// container-safe args win unless explicitly overridden.
export function launchOptions(overrides = {}) {
  const { executablePath } = resolveChrome();
  return {
    executablePath,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
    ...overrides,
  };
}

// Convenience helper for scripts that just want a browser.
export async function launchBrowser(overrides = {}) {
  return chromium.launch(launchOptions(overrides));
}

// Logged once when a script is run directly, so a run records which binary it used.
export function describeChrome() {
  const { executablePath, source } = resolveChrome();
  return `${executablePath} (${source})`;
}
