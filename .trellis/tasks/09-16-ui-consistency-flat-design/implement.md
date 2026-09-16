# UI consistency and Flat Design implementation plan

## Planning exception

The user explicitly authorized skipping the blocked API live-gate task and proceeding with this UI child task. The API task remains `in_progress`; no API live data or production environment is used by this UI work.

## Ordered slices

### Slice 1 — shared consistency foundation

- [x] Start the UI task after reviewing this design and the PRD.
- [x] Baseline all actual templates/routes and capture desktop plus narrow screenshots.
- [x] Normalize shared workspace tokens, button/input states, headings, notices, panels, tables, badges, focus treatment, and motion reduction without changing the visual direction.
- [x] Normalize standalone login/revoke/payer-preview controls to the same primitive sizes and focus behavior.
- [x] Add or update template assertions for shared accessibility hooks and stable classes.
- [x] Run `go test ./... -count=1`, `go vet ./...`, `git diff --check`.
- [x] Browser-check all actual pages and commit Slice 1 alone.

### Slice 2 — Flat Design visual system

- [x] Replace dark glass, gradients, blur, noise, and element shadows with the light flat token system.
- [x] Apply color-block hierarchy to workspace shell, navigation, metrics, notices, tables, forms, dunning drawer, and standalone pages.
- [x] Preserve every route, form action, field name, data attribute, keyboard interaction, and empty/error state.
- [x] Browser-check desktop pages and accessibility structure; commit Slice 2 alone.

### Slice 3 — mobile adaptation and final audit

- [x] Verify each page and state at 320, 360, 375, 390, and 412 CSS px, plus 768, 1024, and 1440 px.
- [x] Eliminate body-level horizontal overflow, clipping, overlap, unreachable controls, and drawer/form/table failures.
- [x] Verify keyboard focus order, expandable rows, dunning drawer open/close, form controls, and table alternatives.
- [x] Run the complete quality gate and commit Slice 3 alone.
- [x] Update task evidence, add session journal entry, and only then archive/finish the UI task.

## Evidence

- Work commits: `8bf1c8a` (shared interaction primitives), `0fb4cdb` (flat design system), and `9a47273` (responsive dashboard follow-up).
- Quality gate: `go test ./... -count=1`, `go vet ./...`, and `git diff --check` passed.
- Browser audit: 10 rendered templates at 320, 360, 375, 390, 412, 768, 1024, and 1440 CSS px; all matched viewport width with no body overflow. Chromium reported no console errors and computed panel/card shadow and blur as `none`.
- Interaction audit: keyboard expansion for dashboard and tenants, dunning open/close, calendar open, and billing allocation-row add all passed in Chromium.

## Validation commands

- `go test ./... -count=1`
- `go vet ./...`
- `git diff --check`
- Start a local app with explicit non-production test configuration where runtime rendering is needed.
- Use a real browser to inspect DOM, screenshots, console, network, accessibility tree, and computed layout at the required breakpoints.

## Stop points

- Stop before changing markup if a form action, field name, route, or data attribute would change.
- Stop and diagnose if a browser console error, body overflow, clipped control, or keyboard-inaccessible interaction appears.
- Do not report final completion until every route in the inventory and all required mobile widths have evidence.
