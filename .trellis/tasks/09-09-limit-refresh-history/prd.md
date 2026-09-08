# Limit refreshed transaction history

## Goal

Manual refresh should work reliably after the initial bank authorization even when `TL_FROM` is set to an older historical date. The first authorization may still request the configured historical range, but later refreshes should use a provider-safe recent window.

## Requirements

- Keep the initial callback behavior compatible with `TL_FROM=YYYY-MM-DD` so a new consent can fetch as much historical data as the bank permits.
- When refreshing with a saved refresh token, cap the transaction `from` date to no earlier than 90 days before the refresh time.
- If `TL_FROM` is already within the last 90 days, refresh should keep that configured date.
- If a transaction fetch fails, the refresh flow must not append a misleading successful result or redirect with `message=refreshed`.
- The billing page should show a clear error notice for refresh data-fetch failures.
- Documentation should explain that `TL_FROM` is for initial/history fetches and refreshes intentionally use a recent 90-day window.

## Acceptance Criteria

- [x] Unit tests cover the 90-day refresh transaction window logic.
- [x] A refresh-time transaction API failure returns the user to `/billing` with an error instead of a success message.
- [x] Existing initial callback query behavior remains unchanged.
- [x] `go test ./...` passes.
- [x] README documents the initial-vs-refresh transaction range behavior.

## Notes

- TrueLayer and bank SCA behavior can limit refresh-token transaction history to the recent consent window. This task handles that locally by avoiding full historical refresh requests.
- Out of scope: changing local storage from JSONL snapshots into a merged transaction database.
