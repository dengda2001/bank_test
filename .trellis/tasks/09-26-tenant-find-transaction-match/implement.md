# Implementation plan

Implemented and accepted on the disposable local audit instance.

1. Add focused tests for the finder query: owned pending income only, current/previous calendar-month boundaries, partial remainder, deferred exclusion, older date scopes, payer/description search fields, token clues, stable pagination, and cross-user isolation.
2. Implement the read-only finder query and view model. Reuse existing transaction status and allocation-summary helpers; cap page size and validate query values.
3. Add tenant-row entry URLs and finder rendering to `/rent-dashboard`, including desktop/mobile actions, URL-preserved workspace filters, date and search buttons, readable receipt cards, empty states, and focus handling.
4. Embed the selected receipt's evidence and allocation controls inside the same finder drawer, reusing the existing review data and draft logic. Prefill the tenant/month/amount within live balances; keep the draft/confirmation endpoint unchanged. Support returning to the list, guarding an unsaved source switch, and refreshing results after success without a second dialog.
5. Apply the agreed placement and label for the existing synthetic manual-balance action. Ensure the finder labels candidate matches as clues, not confirmed payer identity.
6. Add rendered-template/route tests for both desktop and mobile entry points, evidence fields, URL context, one-dialog behavior, bounded draft prefill, no automatic allocation, selected-source error return, and existing receipt-first compatibility.
7. Run focused Go tests, `go test ./... -run '^$' -count=1`, `go vet ./...`, and `git diff --check`. Use the browser audit environment for a desktop/mobile drawer pass, keyboard focus, no horizontal overflow, search switching, and review/return flow.

## Verification

- `go test ./...` passed, including finder templates and existing receipt-first tests.
- `go vet ./...` and `git diff --check` passed.
- Focused finder query test passed against a disposable MySQL schema with ownership, status, defer, date range, name clue, payer search, and partial-balance fixtures.
- Chrome audit on a disposable seeded app passed for tenant-row entry, date scope, name-token and payer search, single selected review drawer, tenant/month/amount prefill, `确认分配`, return to the filtered list, and 390px viewport without horizontal overflow. A full receipt confirmation changed the ledger and removed that source from the pending finder list.

## Risk and rollback points

- Keep the finder read-only; a rollback can remove its entry and query handling without migrating ledger data.
- Changes to the shared review partial/URL builder can affect `/transactions` as well as `/rent-dashboard`. Check both routes before marking the task ready.
- Preserve existing uncommitted workspace and review-drawer edits while modifying nearby files.
