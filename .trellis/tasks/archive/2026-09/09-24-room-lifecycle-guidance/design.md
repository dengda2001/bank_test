# Design: room succession and asset guidance

## Room plan flow

Reuse `SaveRoomRentPlan` for a September successor: the new version starts in September, and the previous open version ends in August. Its existing locked-facts guard continues to reject rewrites of paid or settled months. Add explanatory UI copy and a focused test for the month boundary and unchanged historical obligations. A vacant interval still uses `EndRoomRentPlan`.

## Error and deletion copy

The form route distinguishes zero/negative monthly rent from other invalid inputs before redirecting, using a dedicated error code. The resulting message says rent must be greater than zero and directs a vacant-room user to “结束入住”. Keep server validation as the final authority.

Add a nullable `deleted_at` timestamp to properties and rooms, mapped as a plain pointer rather than GORM's automatic soft-delete type so historical joins do not silently lose their asset. The user-facing delete action marks assets deleted instead of physically removing rows. Historical rent, payment, expense, and room-plan records retain their foreign keys and remain readable in historical context. Daily list and new-operation queries explicitly filter `deleted_at IS NULL`. A room with a rent plan still effective in the current or a later month cannot be soft-deleted until the user ends occupancy from an explicit month. Property soft deletion checks its rooms and guides the user through active plans first. Daily room visibility also requires an undeleted parent, so deleting a property hides its child rooms without changing each child's own deletion timestamp. The backend enforces these checks for direct POSTs. Existing `status=inactive` remains a separate temporary status; it is not presented as the delete mechanism. No restore route or control is provided; historical details remain readable through their existing links.

## Verification

Use the fixture only to test the logic; inspect the actual Arslan plan read-only if the local MySQL database becomes available. Browser-check the rent-plan drawer at mobile width.
