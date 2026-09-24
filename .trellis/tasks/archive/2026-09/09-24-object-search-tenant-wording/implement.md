# Implementation

- [x] Add owner-scoped selected-month tenant real-name and alias search data for room and property rows without N+1 queries.
- [x] Extend property search to linked room/tenant terms and room search to both tenant name forms.
- [x] Add true submit buttons and separate “筛选” toggles to both list forms; verify Enter behavior.
- [x] Audit rendered “责任” copy across workspace, use tenant/rent wording where clearer, and update affected copy assertions.
- [x] Verify search filters, sorting, account scope, and desktop/mobile layout with focused Go/browser tests.

Validation: focused page/filter tests, `go test ./...`, and desktop/mobile browser searches. The wording pass must preserve financial amounts and backend domain contracts.
