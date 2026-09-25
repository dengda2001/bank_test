# Bug analysis: deferred transaction rejected as invalid request

## Root cause category

Cross-layer contract and test coverage gap. `new FormData(deferForm)` sends multipart data, while `handleTransactionAction` used `r.ParseForm()` only. That leaves `transaction_id` unread; the request fails before ownership or match status checks.

## Why the earlier fix missed it

The prior change improved error codes and diagnostics after the defer service runs. This request never reached that service. Batch matching and share revocation already used the shared multipart-capable parser; the defer action did not.

## Prevention

| Priority | Mechanism | Action | Status |
| --- | --- | --- | --- |
| P0 | Shared parser | Reuse `parseTransactionForm` in transaction actions | Done |
| P0 | Regression test | Assert browser multipart and ordinary URL-encoded action forms both parse | Done |
| P1 | Code spec | Record the browser-to-handler content-type contract in backend error handling and the cross-layer checklist | Done |

## Scope audit

The drawer's batch confirmation and share revocation handlers already use `parseTransactionForm`. Other transaction action posts share `handleTransactionAction` and receive the same fix. Unrelated in-progress task edits remain untouched.
