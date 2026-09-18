# Mobile Figma workspace verification

## Implemented slices

- Shared mobile shell: five-item bottom navigation, Objects/More flyouts, 44px touch targets, and safe-area padding.
- Dashboard: two-column metrics and rent cards; balance adjustment requires a reason and strong confirmation.
- Tenants: mobile cards show identity, room, monthly rent, detail/edit actions, and recent billing.
- Expenses: mobile cards show date, amount, payment method, ownership, tenant note, and invoice URL.
- Transactions: each row is a card with status, amount, and processing entry; existing match, allocation, and revoke forms are reused.
- Dunning: the existing strong-confirmation flow opens as a fixed bottom sheet above the safe-area navigation.
- Objects and More pages: simple wide tables become vertical cards while property/room/tenancy/cash/bank empty states remain readable.

## Browser results

Chrome mobile emulation visited every target page at 360×960, 390×960, 430×960, and 600×960:

`/rent-dashboard`、`/bills`、`/transactions`、`/properties`、`/rooms`、`/tenants`、`/tenancies`、`/dunning`、`/cash-receipts`、`/expenses`、`/bank`。

Every page satisfied `document.documentElement.scrollWidth === clientWidth`; the bottom navigation measured 68px high, and at 600px it was centered in a 560px workspace. Data-state screenshots at 390px:

- `390-dashboard-cards.png`
- `390-billing-cards.png`
- `390-tenants-cards.png`
- `390-expenses-cards.png`
- `390-dunning-sheet.png`

## Automated checks

- `go test ./cmd/truelayer-demo/... -count=1`: only the two pre-existing tenant-date fixture failures remain:
  `TestTenantActiveInMonthUsesRentDatesWithoutProration` and `TestTenantActiveInMonthRespectsBillingStartDate`.
- `go vet ./...`: passed.
- The 390px dunning sheet measured `position: fixed`, left/right 8px, bottom 76px, and a 68px navigation bar, leaving an 8px visual gap.
- `./scripts/run-mysql-test-clean.sh ./cmd/truelayer-demo -run '^TestCanonicalPropertyPagesAreUserScopedOnMySQL$' -count=1`: passed; the test database was cleaned automatically.

The temporary property and expense created for browser verification were deleted by ID; no verification data remains in the development database.
