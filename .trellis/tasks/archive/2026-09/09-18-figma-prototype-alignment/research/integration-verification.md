# Figma integration verification

## Child delivery status

The existing child tasks were completed in the dependency order recorded in
`implement.md` and archived under `.trellis/tasks/archive/2026-09/`:

1. `09-19-figma-domain-operations`
2. `09-19-figma-page-data-routes`
3. `09-19-figma-desktop-workspace`
4. `09-19-figma-mobile-workspace`

No duplicate task was created for this integration pass.

## Automated gates

- `GOCACHE=/tmp/rentops-gocache go test ./... -count=1` — passed.
- `go vet ./...` — passed.
- `./scripts/run-mysql-test-clean.sh ./cmd/truelayer-demo -run '^TestCanonicalPropertyPagesAreUserScopedOnMySQL$' -count=1` — passed and cleaned its test database.

The two tenant-date failures that existed before this integration pass were
caused by incomplete fixtures without a monthly rent. The fixtures now include
the required EUR rent, preserving the production rule that an unbound tenant
without rent must not generate an obligation.

## Browser gates

Using the local authenticated demo and Chrome DevTools:

- At 390px, all canonical routes (`/rent-dashboard`, `/bills`,
  `/transactions`, `/properties`, `/rooms`, `/tenants`, `/tenancies`,
  `/dunning`, `/cash-receipts`, `/expenses`, `/bank`) reported
  `scrollWidth === clientWidth` and a 68px fixed bottom navigation.
- Property, room, and tenant create forms exposed the expected binding fields;
  room creation includes `property_id`, and tenant creation includes `room_id`.
- `/billing` rendered 12 transaction rows as mobile cards with no page overflow.
- `/dunning` opened a fixed sheet at 8px side insets, 76px above the viewport
  bottom, with an opaque surface, scrollable content, and the strong send
  confirmation intact.
- At 1366px, the desktop shell kept its relative sidebar and hid the mobile
  bottom navigation; the bank settings screenshot is captured in
  `desktop-bank.png`.

Mobile and sheet evidence is captured in `mobile-dunning-sheet.png`.
