# Room tenant setup corrections

## Goal

Keep new-tenant setup inside the originating room and show occupancy conflicts only when they apply to the tenant currently selected.

## Requirements

- “新增租客” from room rent-plan editing opens the tenant form over that room. Cancel returns to the room plan; save returns to the room with success feedback; validation errors reopen the room-bound form.
- “保存并设置租客” from new-room creation opens the same room-bound form for the newly created room.
- The room-create form shows no conflict warning while “新建租客档案” is selected. When an existing tenant is chosen, show only that tenant’s conflicting room; clear it when the selection or month changes.
- The room rent-plan editor shows conflicts only for tenants selected in its member rows. Conflicting candidates remain unavailable and server validation remains authoritative.

## Acceptance Criteria

- [ ] New tenant flow never displays the tenant list and returns to the correct room after cancel, success, and validation error.
- [ ] Starting with three occupied candidates and no selected existing tenant shows no “去解绑” list.
- [ ] A chosen conflicting tenant shows one relevant unbind link; unrelated candidates are not listed.
- [ ] Direct POST attempts to assign an occupied tenant still fail safely.

## Dependency

None. Parent integration checks the final room flow alongside transaction work.
