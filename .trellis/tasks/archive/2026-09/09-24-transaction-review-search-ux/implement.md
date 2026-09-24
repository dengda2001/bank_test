# Implementation

- [x] Build detail's full-width progress and tenant allocation panel; remove the right-side quick-match card and unused layout rules.
- [x] Add review drawer to detail page and align list/detail rent actions with the shared entry point.
- [x] Support fully allocated source review with correction-only state.
- [x] Add evidence share-revoke dialog through the shared confirmation component and refresh behavior after the allocation child contract is ready.
- [x] Move defer beside confirm and use the shared confirmation component.
- [x] Add payer/tenant name search controls and mobile keyword visibility.
- [x] Adjust smart, roommate, and month-hint styling; verify smart marker clearing.
- [x] Add full-name-miss word-level payer suggestions with deterministic ranking, no result cap, and name-only buttons; cover `Eider`, multiple candidates, and inferred-name safety in focused tests.
- [x] Distinguish roommate month context from explicit month lookup; test that tenant switch loads the month without a `Viewed` badge while explicit lookup sets it.
- [x] Add owner-scoped room detail URL to each roommate group and a “查看房间” link; verify correct room/month navigation.
- [x] Verify successful/failed saves and desktop/mobile keyboard/dialog behavior.

Validation: focused render/action tests, browser checks at desktop and mobile widths, then `go test ./...` and `go vet ./...`. Inspect existing edits in transaction list, handlers, and detail before modifying them.
