# /billing scrolls the page horizontally between 900px and 1440px

Found by the desktop baseline (`harness/desktop.mjs`) that this task added for its own
"desktop must not move" check — **not** by the mobile audit, which sweeps 375/390/768
and therefore steps straight over it. Recorded here so it is not lost, and deliberately
**left unfixed**: it is a desktop defect, and fixing it would move the very rendering
this task promises not to move.

## The measurement

`document.documentElement.scrollWidth` vs `clientWidth` on `/billing`, logged in as the
demo account, `harness/../probe-wide.mjs`-style walk:

| viewport | scrollWidth | overflow | root offender |
|---|---|---|---|
| 768  | 768  | 0    | none (table is contained — see below) |
| 900  | 1017 | 117  | `div.calendar-popover` |
| 980  | 1077 | 97   | `div.calendar-popover` |
| 1024 | 1163 | 139  | `div.calendar-popover` |
| 1152 | 1259 | 107  | `div.calendar-popover` |
| 1280 | 1355 | 75   | `div.calendar-popover` |
| 1440 | 1475 | 35   | `div.calendar-popover` |
| 1680 | 1680 | 0    | none |

`/tenants`, `/rent-dashboard` and `/expenses` are clean at every one of these widths.

## The cause

`calendar.go:48` positions the popover `position: absolute; left: 0` with
`width: min(360px, calc(100vw - 28px))`. On `/billing` the calendar control sits near the
right edge of the filter bar, so a left-anchored 353px panel extends past the viewport.
`opacity: 0` does not take it out of layout, so it extends `scrollWidth` even while closed.

Two things hide it from a narrower sweep:

- the `@media (max-width: 640px)` rule (`calendar.go:196`) switches the popover to
  `position: fixed` with insets, so at phone widths it cannot overflow;
- at 768px the layout is single-column and the control sits far enough left for 353px to fit.

The fix already exists one page over: `dashboard.go:192` carries
`.dashboard-toolbar .calendar-popover { left: auto; right: 0; transform-origin: top right; }`.
`/billing`'s filter bar never got the equivalent override.

## Why the table is not the cause

At 768px the table's right edge reaches 1078px while the page's `scrollWidth` is exactly
768. The table needs 1040px and `.table-wrap { overflow-x: auto }` contains it, so it
scrolls inside its own box. A naive "which element has the largest right edge" probe
labels the table a root offender; comparing against `scrollWidth` shows it is not. This
is the same trap the mobile audit documents for its own offender analysis.

## If it is picked up later

The minimal fix is the dashboard's override applied to `/billing`'s filter-bar control.
The general fix is not to left-anchor a 353px panel on a control that can sit anywhere —
right-anchoring when the control is in the right half of the viewport. Pure CSS cannot
express that condition, which is presumably why the dashboard took the page-specific route.
