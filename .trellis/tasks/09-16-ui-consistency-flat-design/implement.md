# UI consistency and Flat Design implementation plan

## Planning exception

The user explicitly authorized skipping the blocked API live-gate task and proceeding with this UI child task. The API task remains `in_progress`; no API live data or production environment is used by this UI work.

## Ordered slices

### Slice 1 — shared consistency foundation

- [ ] Start the UI task after reviewing this design and the PRD.
- [ ] Baseline all actual templates/routes and capture desktop plus narrow screenshots.
- [ ] Normalize shared workspace tokens, button/input states, headings, notices, panels, tables, badges, focus treatment, and motion reduction without changing the visual direction.
- [ ] Normalize standalone login/revoke/payer-preview controls to the same primitive sizes and focus behavior.
- [ ] Add or update template assertions for shared accessibility hooks and stable classes.
- [ ] Run `go test ./... -count=1`, `go vet ./...`, `git diff --check`.
- [ ] Browser-check all actual pages and commit Slice 1 alone.

### Slice 2 — Flat Design visual system

- [ ] Replace dark glass, gradients, blur, noise, and element shadows with the light flat token system.
- [ ] Apply color-block hierarchy to workspace shell, navigation, metrics, notices, tables, forms, dunning drawer, and standalone pages.
- [ ] Preserve every route, form action, field name, data attribute, keyboard interaction, and empty/error state.
- [ ] Browser-check desktop pages and accessibility structure; commit Slice 2 alone.

### Slice 3 — mobile adaptation and final audit

- [ ] Verify each page and state at 320, 360, 375, 390, and 412 CSS px, plus 768, 1024, and 1440 px.
- [ ] Eliminate body-level horizontal overflow, clipping, overlap, unreachable controls, and drawer/form/table failures.
- [ ] Verify keyboard focus order, expandable rows, dunning drawer open/close, form controls, and table alternatives.
- [ ] Run the complete quality gate and commit Slice 3 alone.
- [ ] Update task evidence, add session journal entry, and only then archive/finish the UI task.

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
