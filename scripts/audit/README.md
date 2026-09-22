# Mobile audit harness

The scripts behind the mobile audit (`mobile-audit.md` / `report.json`, kept with the
archived task at
`.trellis/tasks/archive/2026-09/09-16-mobile-responsive-audit/research/`). They drive
the real app in a real browser at real viewport widths, because the defects this task
fixes are geometry — clipped columns, 15px-tall tap targets — and a CSS reading gets
them wrong. That is not hypothetical: the audit's own opening prediction ("fixed pixel
column widths must overflow the page") was falsified the moment it was measured.

## Running them

Needs Node 20+ and a Chrome/Chromium. `launch.mjs` is the single place that finds
one — every script launches through it, in this order:

1. `CHROME_BIN`, if set. If it points at a missing file the run fails rather than
   quietly using another browser.
2. `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome` (macOS default).
3. `chromium` / `chromium-browser` / `google-chrome` on `PATH`.

If none is found the script exits with instructions for setting `CHROME_BIN`. There
is deliberately no fallback to Playwright's bundled browser: a missing browser must
be a loud failure, not a silent switch to a build the audit did not run against.

```sh
cd scripts/audit && npm install                  # playwright (the scripts import it, not playwright-core)
# CHROME_BIN is optional when Chrome is in the default macOS location; set it to override:
export CHROME_BIN="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
export AUDIT_BASE=http://localhost:8081          # a disposable instance YOU started — see below
export AUDIT_USER=...  AUDIT_PASS=...
node smoke.mjs        # optional: prove the browser resolves and actually launches
node audit.mjs        # 8 pages x 3 viewports -> report.json
node audit-extra.mjs  # the 2 preview pages the first pass missed -> report-extra.json
node metrics.mjs      # touch targets, sub-12px text, scroll distance -> metrics.json
node verify-*.mjs     # independent DOM checks of the load-bearing claims

```

How the desktop-regression claim was made, and how to redo it: build HEAD in a
separate worktree, run that binary on a second port, and shoot both builds.

```sh
git worktree add /tmp/head-wt HEAD && (cd /tmp/head-wt && go build -o rentops-head ./cmd/truelayer-demo)
TL_ADDR=:8082 TL_LOG_FILE=/tmp/head-bank-data.jsonl TL_TOKEN_FILE=/tmp/head-tl-token.json /tmp/head-wt/rentops-head &
AUDIT_BASE=http://localhost:8082 AUDIT_OUT=/tmp/head-desktop node desktop-full.mjs
AUDIT_OUT=/tmp/final-desktop node desktop-full.mjs
node imgdiff.mjs /tmp/head-desktop/tenants.png /tmp/final-desktop/tenants.png
```

Run the HEAD build on a second port rather than swapping the binary: `cp` onto a
running executable fails with `ETXTBSY`, and swapping it means the live site serves
pre-change code for the length of the comparison. `imgdiff.mjs` takes two PNGs; for
the whole sweep compare the per-page files in a loop and read the reported bbox.

`desktop.mjs` and `probe-wide.mjs` are not part of the audit — they are the desktop
regression pair this task added. `desktop.mjs` shoots 1440x900 full-page screenshots and
logs `scrollWidth`/`clientWidth`; `probe-wide.mjs` walks every element and separates a
root offender from a child of one, which is how `desktop-wide-overflow.md` (in the
archived task's `research/`) was found. Note that probe's
parent check labels table content inside `.table-wrap` as a root offender — an element
that overflows a scroll container does not overflow the page, so compare against
`scrollWidth` before believing it.

`AUDIT_OUT` (default `/tmp/mobile-audit/out`) takes the screenshots. Full-page shots
of these pages run to 7400px and are unreadable scaled down, so `tiles.mjs` slices
them into viewport-sized tiles; `crop.mjs` cuts a single region.

The scripts written during the fix — they answer questions the audit could not:

- `desktop-full.mjs` — the desktop regression pair for the 8 pages at 1440x900,
  superseding `desktop.mjs`: it discovers ids, logs `scrollWidth`/`clientWidth`/body
  height plus every sidebar/content/panel box, and md5s the sidebar HTML.
- `imgdiff.mjs` — per-pixel PNG diff with a bounding box, in a canvas, because this
  box has no ImageMagick. How the "only one page changed on desktop" claim was made.
- `verify-p1.mjs` — asserts each named P1 item at 375 and 390: page overflow, the
  calendar trigger, chip heights, the tenant link's hit width (read off its `::after`
  insets), every control >= 44, and the named column widths.
- `verify-offenders.mjs` — walks each audit offender's ancestors and buckets it as
  `in scroller` / `off-canvas` / `PAGE-LEVEL?`, so "offenders=25" becomes a statement
  about what they are. Read it with the caveat that the count *rose* after the fix:
  the new drawer adds three off-canvas elements to every page.
- `verify-hits.mjs` — the only script that settles a hit area, by clicking it. Two
  fixes leave a small *element box* behind a large *hit area* (a `::after` extension,
  a wrapping `<label>`), and a box measurement cannot tell those from a real miss.
- `probe-controls.mjs`, `probe-dash.mjs` — dump every control with its measured box,
  to trace a sub-44px control to the rule that beats the shared one.
- `probe-rows.mjs` — real line counting via `Range.getClientRects()`, plus the
  `/billing` frozen-column geometry.
- `probe-expanded.mjs` — the collapsed vs expanded `/billing` row.
- `probe-payer.mjs`, `probe-css.mjs` — whether the payer-preview's confirm form is
  reachable with the live data (it is not), and whether the preview templates' CSS
  survives Go's `html/template` CSS escaper (it does).
- `probe-p1-4.mjs` — measures all ~20 separate claims in the audit's P1-4 section, so
  the "fixed / not fixed" status table in the archived `mobile-audit.md` is measured
  rather than recalled. Writing it is what established that several P1-4 items were
  *not* fixed.
- `probe-start.mjs` — the two design.md acceptance items nothing else covers: content
  top edge <= y=120 on every page at 375, and P2 page heights. Also re-settles
  P0-2's exact claim (that no scroll position showed the name and the action cell
  together), which is what the frozen column was chosen for.
- `shot-rows.mjs` — clips a screenshot to the first N table rows so a layout that has
  only been measured can also be looked at.

## Bringing up the instance to audit

Use `scripts/run-audit-local.sh` to start a populated disposable instance. It creates
a fresh `rentops_audit_*` MySQL database, starts the app, and seeds properties,
rooms, tenant profiles, current and future room plans, and cash receipts through the
app's own forms. It then stays running for browser checks and drops the database when
you stop it. The script refuses non-loopback MySQL and the production port.

```sh
MYSQL_ADMIN_CMD='mysql -h 127.0.0.1 -P 3306 -u root' scripts/run-audit-local.sh
```

The printed account and URL are for that one disposable run. Do not point the
harness at `:8081` / `bank.ddpl.top` or any shared database.

How the desktop-regression claim was made, and how to redo it: build HEAD in a
separate worktree, run that binary on a second port, and shoot both builds (snippet
above).

## The constraint that shaped the scripts

`seed-workspace.mjs` creates run-scoped records with normal UI writes. Do not reuse
its account against a database containing data you need; the audit instance is meant
to be disposable and is torn down as a whole.
