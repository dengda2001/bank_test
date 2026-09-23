# Room creation and occupancy tenant selection

## Goal

Make it possible to choose an existing tenant or create a new tenant while setting up a new room, and to create a new tenant from a room's occupancy and rent editor.

## Requirements

- The new-room "Save and set up tenant" path offers existing tenants and a clear way to create a new tenant.
- An existing tenant assigned to another room for the selected occupancy period is visibly flagged and cannot be selected. The UI explains that the tenant must first be removed from the other room's occupancy plan, with a link to that room when possible. It does not transfer the tenant automatically.
- A tenant available for the selected period can be assigned to the new room with its initial rent arrangement.
- The room's "Occupancy and rent" editor offers a direct path to create a new tenant for that room without requiring the user to first find the tenant page.
- Property and room selections stay scoped to the intended room in the new-tenant flow.
- The server continues to enforce tenant room exclusivity. Existing rent-plan and accounting constraints remain effective.

## Acceptance criteria

- [ ] A new room can be saved with an available existing tenant and an initial occupancy/rent plan.
- [ ] A new room can be saved and lead into creating a new tenant with the new room preselected.
- [ ] Tenants assigned to another room are highlighted, disabled for assignment, and given an actionable unbind explanation; forged or stale submissions are rejected server-side.
- [ ] From a room's occupancy/rent editor, a user can begin creating a tenant with the room and property preselected, then return to the room with the new tenant assigned.
- [ ] Relevant UI and service tests pass, including stale occupancy conflicts and the property/room preselection.

## Scope notes

- The earlier request to allow moving a tenant between rooms after confirmation was superseded by the later instruction to forbid selection until the tenant is unbound.
- Historical room plans and payments must be preserved when editing current or future occupancy.
