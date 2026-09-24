# Implementation plan

1. Read the room-plan, tenant-assignment, backend database, frontend UI, and cross-layer specs. Add focused tests for tenant availability projection, atomic room creation with an existing tenant, and safe room return redirects.
2. Implement the room-create tenant choice, conflict display, month-driven availability, and atomic service/route handling. Keep the vacant and new-tenant routes functional.
3. Add the new-tenant action in the room occupancy editor, preselect its property/room/month, and return to the room after a successful assignment. Show assignment errors in the tenant drawer.
4. Apply the same conflict display and disabled state to existing-tenant choices in the room occupancy editor, including changes to the effective month. Keep server-side conflict validation.
5. Run formatting, `go test ./cmd/truelayer-demo/...`, `git diff --check`, and the repository quality commands required by the specs. Run MySQL integration tests if a test DSN is available. Verify desktop and mobile drawer flows in a browser if the local runtime is available.
6. Review the implementation against the PRD and cross-layer contracts, update the relevant Trellis spec and the open-requests checklist, then record completion through Trellis.

## Review gate

- Confirm the final plan matches the user request before `task.py start`.
- Pay special attention to plan intervals that start in the future, stale form data, and preserving the room drawer context after an error.

## Verification evidence

- `go test ./... -count=1`, `go vet ./...`, `node --check` for the new availability script, and `git diff --check` passed.
- Focused MySQL service tests passed against an isolated disposable MySQL 8.4 server: atomic room/tenant creation, conflicting and unknown tenant rollback, removal of the final occupant, existing new-tenant assignment, and current/future room conflicts. The temporary server and data directory were removed afterward.
- Headless Chrome checked desktop previews and true 390px and 320px mobile emulation. The new-room and occupancy drawers had no document overflow. A month-switch probe re-enabled a tenant after its old plan ended; a radio probe enabled the existing-tenant picker while preserving the disabled conflict option.
- The mobile entity drawer title initially rendered above its header because the long form shrank the header flex item. `entity-drawers.css` now keeps the mobile header at `flex: 0 0 auto`, and the 390px rerender showed the title inside the drawer.
