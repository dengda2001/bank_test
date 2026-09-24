# Implementation plan

## Order

- [x] Complete child `09-24-room-tenant-setup-corrections`: selected-only conflict warning, room-context tenant drawer, correct cancel/success/error return, room-create handoff.
- [x] Complete child `09-24-allocation-share-revocation`: exact-share service and POST handler; focused cases for split source, other source evidence, idempotency, stale/foreign rows, projection and home-queue eligibility.
- [x] Complete child `09-24-shared-danger-confirmation-dialogs`: global red in-app dialog, migrate native confirmations, add coverage for committed destructive actions, verify submitter semantics.
- [x] Complete child `09-24-login-visual-alignment`: workspace-consistent login form and responsive visual/functional checks.
- [x] Complete child `09-24-object-search-tenant-wording`: selected-month tenant searches on room/property lists, Enter submission, and rendered “责任” copy audit.
- [x] Complete child `09-24-transaction-review-search-ux`: full-width detail progress and tenant allocation panel, detail/list/review entry points, full-source correction state, evidence and defer dialogs, semantic color, full-name then word-level tenant suggestions, roommate switch month-state correction and room-detail link, clickable-name search on desktop/mobile.
- [x] Integrate child contracts, verify draft preservation and post-action return behavior, and check all six user scenarios end to end.
- [x] Run Trellis quality check and focused browser checks, review the final diff, then commit and push the isolated changes (already authorized by the user).

## Validation

- Focused Go tests for room assignment, transaction actions, full-name/word-level tenant suggestions, review model, list/detail render, and pending queue; use existing MySQL-backed tests where the repo's test harness needs MySQL.
- `go test ./...` and `go vet ./...` from `cmd/truelayer-demo` or the module root indicated by `go.mod`.
- Browser checks: room create with three occupied candidates; new tenant from room; list name click at desktop and mobile widths; detail progress and review entry; split source share revoke; deferral cancel/confirm; property/room/tenant delete cancel paths; settlement and bulk-send confirmation; login desktop/mobile and invalid credentials; room/property tenant search by Enter; updated success/error feedback.
- Check computed colors and dialog focus/close behavior in a real browser.
- Run `python3 ./.trellis/scripts/task.py validate` for the parent and children before task start/completion as applicable.

## Risk points and review gates

- `main.go`, `transaction_handlers.go`, `transaction_list_page.go`, and the tenant drawer template already contain unrelated uncommitted changes; inspect current content before each edit and stage only task hunks.
- Full-source revoke and exact-share revoke must remain distinct routes and audit actions.
- Fully allocated sources must be reviewable for correction without allowing a new allocation against zero balance.
- Room return targets must be same-origin, validated by owner scope, and must return validation errors to the room drawer.
- Shared confirmation must preserve `requestSubmit` validation and the exact submitter, including external buttons and `formaction`; eliminate duplicate page-level handlers and native prompts.
- Terminology changes are visible-copy only; reconcile overlapping transaction/room templates and update copy assertions without changing domain calculations.
- After child integration, compare effective allocation sum, source projection, obligation paid/status, and pending dashboard membership for a split-source revoke.

## Before execution

Present the PRD, design, task map, and implementation order for user review. Start the parent/child task only after that review gate, per `.trellis/workflow.md` Phase 1.4.
