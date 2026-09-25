# Error Handling

> How errors are handled in this project.

---

## Overview

<!--
Document your project's error handling conventions here.

Questions to answer:
- What error types do you define?
- How are errors propagated?
- How are errors logged?
- How are errors returned to clients?
-->

(To be filled by the team)

---

## Error Types

<!-- Custom error classes/types -->

(To be filled by the team)

---

## Error Handling Patterns

<!-- Try-catch patterns, error propagation -->

(To be filled by the team)

---

## API Error Responses

<!-- Standard error response format -->

(To be filled by the team)

---

## Common Mistakes

### Scenario: Browser FormData sent to transaction actions

#### 1. Scope / Trigger

Any transaction action posted with browser `new FormData(form)`, including the matching drawer's “暂不处理” button.

#### 2. Signatures

- Browser: `fetch(form.action, {method: 'POST', body: new FormData(form)})`.
- Server: `parseTransactionForm(*http.Request) error` and `transactionActionIDFromRequest(*http.Request) (uint64, error)`.

#### 3. Contracts

`FormData` sends `multipart/form-data`; ordinary HTML form posts may send `application/x-www-form-urlencoded`. Both must populate `r.Form` before reading `transaction_id`, `reason`, or `return_to`. Transaction action handlers cap the request body at 1 MiB and use `parseTransactionForm`, which calls `ParseMultipartForm` for multipart requests.

#### 4. Validation & Error Matrix

| Request | Behavior |
| --- | --- |
| Multipart with valid positive `transaction_id` | Parse fields, then apply owner and state checks |
| URL-encoded with valid positive `transaction_id` | Same checks |
| Missing/malformed ID or malformed multipart body | Redirect with `invalid_transaction_action`; do not write an action |
| Valid form, stale match status | Return `transaction_not_pending` |

#### 5. Good / Base / Bad Cases

- Good: browser defer `FormData` carries ID 677 and a reason; server reads both and reaches `deferTransaction`.
- Base: ordinary URL-encoded POST remains supported.
- Bad: `r.ParseForm()` alone leaves multipart fields unread, so an eligible transaction appears to have no ID.

#### 6. Tests Required

`TestTransactionActionReadsBrowserFormDataAndOrdinaryForms` must assert multipart and URL-encoded IDs and reasons are parsed, while a missing ID is rejected. Check the browser notice text for a useful recovery step.

#### 7. Wrong vs Correct

Wrong: call `r.ParseForm()` alone in a handler reached by `new FormData(form)`.

Correct: bound the request body, call `parseTransactionForm(r)`, then validate the positive transaction ID before invoking the action service.

### Scenario: TrueLayer manual refresh transaction range

#### 1. Scope / Trigger

- Trigger: TrueLayer transaction refresh uses an external bank consent boundary and can fail when asking for history older than the refresh-token access window.
- Applies to `cmd/truelayer-demo` refresh flows that call `/data/v1/accounts/{account_id}/transactions`.

#### 2. Signatures

- `handleCallback(w http.ResponseWriter, r *http.Request)` fetches bank data immediately after a fresh authorization-code exchange.
- `handleRefresh(w http.ResponseWriter, r *http.Request)` refreshes the access token and fetches bank data using the saved refresh token.
- `refreshTransactionFrom(configuredFrom string, now time.Time) string` returns the refresh-safe transaction start date.
- `fetchDemoResultWithOptions(ctx context.Context, accessToken, from string, failOnTransactionError bool) (demoResult, error)` controls whether transaction endpoint failures are fatal.

#### 3. Contracts

- `TL_FROM=YYYY-MM-DD` remains the configured historical start date for a fresh bank authorization callback.
- Manual refresh must cap the transaction `from` date to the later of `TL_FROM` and the UTC date 90 days before refresh.
- Manual refresh failures use `/transactions?error=data_fetch_failed` for transaction data-fetch failure.

#### 4. Validation & Error Matrix

- `TL_FROM` empty, invalid, future-dated, or older than 90 days on refresh -> use the UTC 90-day cutoff.
- `TL_FROM` within the last 90 days on refresh -> use the configured date.
- Accounts fetch fails -> return a top-level data fetch error.
- Transaction fetch fails during callback -> preserve the existing partial-result behavior and record the account error.
- Transaction fetch fails during refresh -> return an error, do not append a new bank-data log entry, and do not redirect with `message=refreshed`.

#### 5. Good/Base/Bad Cases

- Good: Fresh authorization uses `TL_FROM=2026-01-01`; later refresh on `2026-09-08` uses `from=2026-06-10`.
- Base: `TL_FROM=2026-08-01`; refresh on `2026-09-08` keeps `from=2026-08-01`.
- Bad: Refresh sends the original historical `TL_FROM` and receives provider `access_denied` / SCA-style errors.

#### 6. Tests Required

- Unit test `refreshTransactionFrom` for old and recent configured dates.
- Handler test for refresh transaction failure: redirects to `/transactions?error=data_fetch_failed`, does not append a log line, and does not use the uncapped historical date.
- Template test for the transaction list error notice.

#### 7. Wrong vs Correct

Wrong:

```go
result, err := a.fetchDemoResult(r.Context(), token.AccessToken)
```

Correct:

```go
from := refreshTransactionFrom(a.cfg.From, time.Now())
result, err := a.fetchDemoResultWithOptions(r.Context(), token.AccessToken, from, true)
```
