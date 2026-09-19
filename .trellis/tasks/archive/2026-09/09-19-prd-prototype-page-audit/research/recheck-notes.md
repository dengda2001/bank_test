# Recheck notes

## Browser environment

- App bound to 127.0.0.1:18090.
- Database/account were created by the local audit runner with the rentops_audit_ prefix and cleaned when the runner stopped.
- The primary route-capture dataset used one synthetic property, room, and tenant; no real customer records were used.
- Browser used Chrome through the existing local Playwright package.

## Captured states

The full route capture script recorded every available dashboard view, property/room/tenant list and drawer/detail state, bills, dunning, and transactions at 1440×900 and 1366×768; mobile equivalents at 390×844; plus the empty-account dashboard. Results are in screenshots/app-after-summary.json.

All requested routes returned HTTP 200. Captured documents had scrollWidth equal to viewport width at all three sizes. On mobile, the workspace begins at y=72, object tabs display on property/room/tenant lists and create/edit forms, and property/room details expose their bottom actions.

The single 404 console message observed on the first 1440px dashboard capture did not recur in the independent response check; that check observed no 4xx/5xx responses.

## Interactions verified

- Property search narrows the list and no-match search shows the empty-result state.
- Property detail return link restores period, status, and search.
- Property edit save returns to the same detail page with period and list-origin filters preserved.
- Room search by tenant, property filter, and status are applied.
- Room detail return link restores period, property, status, and search.
- Room edit save returns to detail with the source-list filters intact.
- Dashboard dimension tabs remain available on an empty account.
- Both transaction month pickers remain inside 1440px and 1366px desktop viewports when opened.
- Property and room edit routes retain period=2026-09 after save.

## Go checks

Passed:

```text
GOCACHE=/private/tmp/rentops-go-cache-audit go test ./cmd/truelayer-demo/... ./cmd/rentops-e2e/...
ok bank/cmd/truelayer-demo
ok bank/cmd/rentops-e2e
```

Also passed git diff --check.

## Transaction detail and mobile More follow-up

- Added `/transactions?detail={id}`. List links preserve filters, sort, and page; the return link targets the originating row. Detail reads are scoped by authenticated user ID.
- Added markup assertions for matching, allocation, processing history, source details, escaped source data, and list-state URLs. Added projection checks for room/property context and voided allocations.
- Added `/more` as a standalone mobile feature hub; the bottom navigation highlights it and the Objects menu remains the only flyout.
- Full Go tests and `go vet` pass after allowing the test process to bind its local `httptest` loopback port. The opt-in MySQL binding integration test remains unexecuted because `RENTOPS_MYSQL_TEST_DSN` is unset.
- The in-app browser connection could not start in this environment (`privileged native pipe bridge is not available; browser-client is not trusted`), so these two new visual states have not been screenshot-checked.

## Nested dashboard tree follow-up

- The follow-up audit account contained imported legacy tenants/transactions but no structured properties, so it could not exercise the new tree through live account data. I rendered the production Go template with temporary synthetic property/room/tenant rows for browser verification; the fixture files lived under `/private/tmp` and were removed after capture.
- In Chrome, clicking the property summary opened its room; clicking the room summary opened both tenant responsibility rows. The rooms-view summary also opened its tenant responsibility list.
- Desktop 1440×900 and mobile 390×844 both reported `scrollWidth == clientWidth`; the fixture browser run reported no page errors.
- Screenshots: `app-final-desktop-dashboard-tree.png` and `app-final-mobile-dashboard-tree.png`.
- The workspace service's MySQL integration test now checks that each child tenant remains attached to its matching room and property in both property and room views.
