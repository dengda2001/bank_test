# Design

Reuse the shared `tenant-form-drawer` partial and tenant assignment option loader from the tenant page, with a room-scoped data object. Open it using `tenant_add=1` on `/rooms/{id}` and validate property/room ownership. Use a validated room URL as the POST return target; extend tenant success/error redirect helpers to accept that target without an open redirect. Render the room page behind one active drawer, then restore rent-plan edit on cancel or save. Send new-room setup directly to this URL.

In `room-tenant-availability.js`, compute disabled options for all candidates but build warnings only from selected options in active selectors. The room-create radio controls whether the existing-tenant selector contributes a warning. Remove broad server-rendered conflict loops. Backend occupancy validation is unchanged.
