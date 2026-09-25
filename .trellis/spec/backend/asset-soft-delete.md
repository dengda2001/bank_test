# Asset soft deletion and room succession

## Current contract

`properties.deleted_at` and `rooms.deleted_at` are nullable timestamps mapped as plain `*time.Time`. Never use GORM's automatic soft-delete type here: historical rent and expense lookups must still be able to find the original asset by ID.

- The user-facing property and room delete actions set `deleted_at`; they never physically delete an asset, room plan, rent charge, obligation, allocation, or expense. No restore route or control exists.
- A room with a plan effective in the current or a future month cannot be hidden. A property cannot be hidden while any child room has such a plan. The landlord must end occupancy from an explicit month first; locked rent facts can still prevent ending it.
- Default property and room lists, rent workspace views, creation selectors, and match location selectors exclude hidden assets. A room under a hidden property is hidden from daily lists even if its own `deleted_at` is null.
- Direct historical property and room detail reads may include hidden assets. The page data loaders must opt into `IncludeDeleted` for property and room lists and `loadWithAssetHistory` for workspace financial summaries; normal lists use the default hidden view. Historical pages are read-only and show that history is retained. Expense history resolves names with `IncludeDeleted` rather than losing the asset label.
- New room plans, expenses, and bank debit attributions reject a hidden property or room on the server even when an old ID is posted directly.

For a room succession, save a new room plan from the successor month with the old tenant removed and new tenant added. `SaveRoomRentPlan` closes the prior plan at the end of the previous month and leaves earlier rent facts and payments intact. To make the room vacant instead, call `EndRoomRentPlan` with the first vacant month. Monthly room rent in a saved plan must be greater than zero; zero is not the vacancy action.
