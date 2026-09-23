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
