# Design

## Boundaries and data flow

- The room-create drawer loads the owner's tenants plus their room-plan occupancy intervals. A tenant selector offers an existing tenant or a new-tenant path. The selected `effective_month` drives availability. Any other-room plan that overlaps the selected month or a later month blocks selection, matching `validateTenantRoomMonthAvailability`.
- The UI shows the conflicting room name and a link to its occupancy editor. Disabled choices are advisory; the service makes the final decision inside a database transaction.
- Extend the composite room service to create the room, initial rent rule, and optional existing-tenant member atomically. Reuse `SaveRoomRentPlan` validation and version checking where possible. An unavailable or stale tenant rolls back the whole new-room transaction.
- For a new tenant, retain the existing `save_and_setup` flow: create a vacant room and redirect to `/tenants?add=1` with property, room, and month preselected. The tenant creation service already adds the identity and room-plan member atomically.
- In the room occupancy editor, add a visible "Create tenant" action that opens the existing tenant drawer with property, room, and displayed month preselected. Carry a validated room return context through GET and POST. On successful assignment, return to that room's occupancy editor; on validation error, reopen the tenant drawer with its selections. The user can still cancel back to the originating room.
- Room occupancy tenant options use the same conflict projection and disable tenants who overlap another room, while retaining selected current-room members. Changing `effective_month` refreshes availability client-side from server-rendered interval data. The backend remains authoritative.

## Contracts

- Room creation posts `action=save` or `action=save_and_setup`, plus optional `tenant_id` and an explicit choice between existing and new tenant. `save` creates a vacant room. `save_and_setup` with an existing tenant creates its initial member at the full room rent. `save_and_setup` with a new tenant redirects to the preselected tenant form.
- Tenant option payloads contain only owner-scoped room IDs, room labels, and plan interval dates needed for the UI. The server does not trust these payloads on POST.
- Return-to-room context contains a room ID and period, is accepted only for a room-owned preselected assignment, and never allows an external redirect.
- Avoid changing storage schema or historical plan semantics.

## Compatibility, risk, and rollback

- The current empty-room save path and tenant-page standalone create path keep their existing behavior.
- The highest-risk change is atomic creation with a selected existing tenant. A rollback assertion should verify that conflict or stale selection leaves no room behind.
- A tenant may become unavailable between render and submit; show a clear conflict error and preserve the room-create form context. Preserve financial fact locks enforced by `SaveRoomRentPlan`.
- Rollback can revert the new composite service and UI paths without a migration.
