# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

<!--
Document your project's quality standards here.

Questions to answer:
- What patterns are forbidden?
- What linting rules do you enforce?
- What are your testing requirements?
- What code review standards apply?
-->

(To be filled by the team)

---

## Forbidden Patterns

<!-- Patterns that should never be used and why -->

(To be filled by the team)

---

## Required Patterns

<!-- Patterns that must always be used -->

(To be filled by the team)

---

## Testing Requirements

### Scenario: Demo Auth Gate for Bank Data Screens

#### 1. Scope / Trigger

- Trigger: Any route that starts bank authorization, handles OAuth callbacks, refreshes bank data, or renders bank-derived UI.
- This project currently runs as a small Go HTTP demo, but bank tokens and bank transaction payloads are still sensitive.

#### 2. Signatures

- `GET /` renders the local demo login screen when unauthenticated.
- `POST /login-local` validates the configured demo administrator credential and sets the app session cookie.
- `POST /logout` clears the app session cookie.
- `GET /transactions` requires the app session and renders the bank income workspace.
- `GET /login` requires the app session and starts the TrueLayer OAuth flow.
- `GET /callback` requires the app session before exchanging an authorization code.
- `GET|POST /refresh` requires the app session before using the stored refresh token.

#### 3. Contracts

- Environment keys:
  - `APP_ADMIN_USERNAME`: demo administrator username.
  - `APP_ADMIN_PASSWORD`: demo administrator password.
  - `APP_SESSION_SECRET`: HMAC signing secret for the demo session cookie.
  - `TL_TOKEN_FILE`: local refresh-token file path.
  - `TL_LOG_FILE`: local JSONL bank payload log path.
- Session cookie:
  - `HttpOnly`.
  - `SameSite=Lax`, so the OAuth redirect back to `/callback` can carry the app session.
  - Must not contain bank tokens or raw bank payloads.
- Stored token file:
  - Stores refresh token only, never access token.
  - Uses file mode `0600`.

#### 4. Validation & Error Matrix

- Missing or invalid app session on protected route -> redirect to `/`.
- Invalid demo login credentials -> redirect to `/?error=invalid_login`.
- Missing stored refresh token on refresh -> redirect to `/transactions?reconnect=1&error=no_saved_login`.
- Refresh token rejected by provider -> redirect to `/transactions?reconnect=1&error=refresh_failed`.
- Missing or expired OAuth state -> reject callback before token exchange.

#### 5. Good/Base/Bad Cases

- Good: `/callback` checks the app session before consuming OAuth state, exchanging code, saving refresh token, or logging bank data.
- Base: `/transactions` can render with no token file and no bank log, showing a bind-bank empty state.
- Bad: Returning raw bank JSON from `/callback` to an unauthenticated browser, or exchanging OAuth code without a valid app session.

#### 6. Tests Required

- Login success sets a session cookie and redirects to `/transactions`.
- Protected routes reject missing sessions.
- Callback without app session does not consume OAuth state.
- Stored refresh token detection is covered.
- Income transaction normalization covers `CREDIT`, positive amount fallback, debit filtering, and unknown/inferred payer fields.

#### 7. Wrong vs Correct

Wrong:

```go
func (a *app) handleCallback(w http.ResponseWriter, r *http.Request) {
    token, _ := a.exchangeCode(r.Context(), r.URL.Query().Get("code"))
    _ = a.saveStoredToken(token)
}
```

Correct:

```go
func (a *app) handleCallback(w http.ResponseWriter, r *http.Request) {
    if !a.requireAuth(w, r) {
        return
    }
    if err := verifyState(r); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    // Exchange code and save refresh token only after both checks pass.
}
```

### Scenario: Transaction Description Month Parsing

#### 1. Scope / Trigger

- Trigger: Any bank transaction matching or billing-history projection that derives a billing month from `Description` or `Reference`.
- Keep date extraction in the shared `parseReferencedPeriod` path so matching and UI projections use the same interpretation.

#### 2. Signatures

- `parseReferencedPeriod(text string, transactionTime time.Time) (time.Time, bool)` returns the first recognized billing period and whether the description contained a usable period.
- `selectObligationForTransaction(tx transactionRow, obligations []obligationRow) (obligationRow, bool)` consumes the parsed period when selecting a billing obligation.

#### 3. Contracts

- Supported explicit formats include numeric month/year (`9/26`, `09/2026`), spaced English month/year (`Sep 2026`), Chinese year/month, and compact English month/year (`JULY26`, `SEP26`, `JULY2026`).
- A two-digit year in a compact English token maps to `2000 + year`.
- An explicit year in the description always takes precedence over `transactionTime`; `transactionTime` is only a fallback for month-only references such as `JULY`.
- The parser returns the first valid period in the combined description/reference text, preserving the existing matching precedence.

#### 4. Validation & Error Matrix

- Valid month and year -> return the first day of that month with `true`.
- Invalid month/year token -> ignore that token and continue with other supported formats; if none match, return the existing month-only or no-period fallback.
- Unknown description text -> return `false` without inventing a year.
- Compact token such as `JULY26` with a transaction dated in 2027 -> return July 2026, never July 2027.

#### 5. Good/Base/Bad Cases

- Good: `REMAIN50 *MOBI RENT JULY26` on a 2027 transaction matches `2026-07-01`.
- Base: `RENT 9/26` matches September 2026; `REMAIN 50 EURO *MOBI RENT 8/26` matches August 2026.
- Bad: Treating `JULY26` as month-only and deriving 2027 from the transaction timestamp.

#### 6. Tests Required

- Unit-test compact English month/year tokens with a transaction timestamp in a different year; assert both `2026-07-01` and `2026-09-01` examples.
- Retain tests for numeric, spaced English, Chinese, month-only, and no-period inputs.
- Run the billing matching and full backend test suites after changing regex precedence or fallback behavior.

#### 7. Wrong vs Correct

Wrong:

```go
// JULY26 is reduced to JULY, then the transaction year is inferred.
month := monthOnlyRegex.FindStringSubmatch(text)
return time.Date(transactionTime.Year(), monthNumber(month), 1, 0, 0, 0, 0, time.UTC), true
```

Correct:

```go
if m := compactEnglishMonthYearRegex.FindStringSubmatch(text); len(m) == 3 {
    year := parseCompactYear(m[2])
    return time.Date(year, monthNames[strings.ToLower(m[1])], 1, 0, 0, 0, 0, time.UTC), true
}
```


### Scenario: Isolated HTTP E2E Acceptance Harness

#### 1. Scope / Trigger

- Trigger: changes to `cmd/rentops-e2e/`, `scripts/run-e2e-local.sh`, or any route, response contract, or amount projection the acceptance suite asserts on.
- The suite is the only place where rent, tenant, dashboard, and dunning behaviour is verified against the running application over real HTTP.

#### 2. Signatures

- `go run ./cmd/rentops-e2e -report <path>` runs preflight only; `-execute` is required to send writes.
- `scripts/run-e2e-local.sh` provisions a disposable environment: database `rentops_e2e_<run-id>`, user `rentops_e2e_<hex>`, two accounts seeded by starting the app twice with different `APP_ADMIN_USERNAME`, then runs the suite and drops the database.
- Environment keys:
  - `RENTOPS_E2E_SKIP_DUNNING_DELIVERY=1`: skip only the real send (`/dunning/send` and its repeat).
  - `RENTOPS_E2E_CONFIRM_WRITES=I_UNDERSTAND_NON_PRODUCTION`, `RENTOPS_E2E_CONFIRM_CLEANUP=I_UNDERSTAND_DELETE_RUN_ID_ONLY`.

#### 3. Contracts

- Run IDs match `rentops-e2e-YYYYMMDD-HHMMSS-xxxxxxxx`; every fixture value, stable key, description, idempotency key, and request key starts with that run ID.
- Target name and database name must each equal an explicit allowlist; the runner re-checks `SELECT DATABASE()` before deleting.
- The report is machine-readable and redacted. `status: "passed"` means every executed assertion passed **and** `cleanup.verified`, `cleanup.http_verified`, and `cleanup.fixture_removed` are all true.
- Scenarios report `passed`, `failed`, or `skipped`. Every `skipped` scenario is also listed in the top-level `unverified` array with its reason.
- `RENTOPS_E2E_SKIP_DUNNING_DELIVERY=1` keeps dunning candidate discovery, configuration, and preview in scope while leaving `/dunning/send` unverified. A passing run with this flag must never be read as "mail delivery was verified".

#### 4. Validation & Error Matrix

- Assertion failure -> stop the remaining writes, keep the run data and report, report `status: "failed"`.
- Scenario skipped -> `unverified` entry; never counted as passed and never a failure.
- Setup failure (fixture directory, missing SMTP sink, target probe) -> fail before any write request.
- Cleanup allowlist mismatch -> stop without deleting, keep `cleanup.verified = false`.
- Cleanup succeeded -> re-check zero residue over HTTP as the second account **and** through controlled queries; leftover local fixture files fail the run.

#### 5. Good/Base/Bad Cases

- Good: a failed assertion leaves the isolated data and JSON report in place for diagnosis; cleanup only runs after every assertion passed.
- Base: with mail delivery explicitly excluded, the run reports `status: "passed"` plus `unverified: [{name: "dunning-delivery", ...}]`.
- Bad: treating `skipped` as `passed`, or reporting `passed` while the delivery path was never attempted.

#### 6. Tests Required

- Unit-test option validation, skipped-scenario recording, and that `unverified` survives the redacting report writer.
- Assert the application's real read-back contract, not what a page "ought" to render:
  - bank statement numbers are the **normalised** provider transaction id (`e2eBankTransactionFixture.StoredProviderTransactionID`), not `ProviderTransactionID`;
  - the cash preview renders the entered amount as `350.00 EUR` while the balance cards render `EUR 0.00`;
  - dashboard bank metrics are scoped by transaction date, so an allocation on a September transaction is absent from the August period view.
- Invalid dashboard filters, non-EUR cash, and cross-user ID access are asserted as controlled errors, not as crashes.
- The post-cleanup residue scan ignores the operator account name rendered by the app chrome, which itself carries the run ID prefix.
- After changing any cleanup loader/validator pair, run the live suite: a key or SQL format mismatch is invisible to unit tests.

#### 7. Wrong vs Correct

Wrong:

```go
// The preview renders "350.00 EUR"; this substring can never appear, so the
// scenario fails against a correct application.
preview.Passed = strings.Contains(body, "EUR 350.00")
```

Correct:

```go
// Assert the value the template actually emits, keeping the exact-amount intent.
cashAmount := strings.Contains(body, "350.00 EUR")
afterRemaining := strings.Contains(body, "EUR 0.00")
preview.Passed = response.StatusCode == http.StatusOK && cashAmount && afterRemaining
```

---

### Scenario: Canonical page view-model and route contract

#### 1. Scope / Trigger

- Trigger: adding or changing a page route consumed by both desktop and mobile
  renderers in `cmd/truelayer-demo`.
- Applies to canonical pages such as `/bills`, `/transactions`,
  `/properties`, `/rooms`, `/tenancies`, `/cash-receipts`, and `/bank`.

#### 2. Signatures

- `newAppMux(a *app) *http.ServeMux` is the single live/test route registry.
- Page handlers return server-rendered view models with typed IDs, cents,
  status keys, and `pageActionView` action metadata.
- `scopedPageUser(w, r)` requires a signed database session before any scoped
  read; single-row detail loaders include `user_id` in the query.

#### 3. Contracts

- Canonical GET routes may keep legacy aliases, but OAuth, refresh, and cash
  subroutes remain functional. `/billing` is retired: it was never an alias, it
  was a second page, and every `/billing*` route is gone.
- Every action exposes a stable URL/method and declares reason or confirmation
  requirements; templates never infer these from labels or amounts.
- Bank pages expose connection/sync health and account metadata, never token
  values; `/bank/sync` uses the saved refresh-token flow.

#### 4. Validation & Error Matrix

- Missing/invalid session -> redirect to `/` before parsing an entity ID.
- Cross-user detail ID -> `404`, never a serialized foreign row.
- Invalid period/status/property filters -> controlled `400`.
- Missing required mutation reason -> redirect/error without a write.

#### 5. Good/Base/Bad Cases

- Good: load a user-scoped repository model, map cents and status keys, then
  render one shared view model for both breakpoints.
- Base: empty lists render an explicit empty state with zero leakage.
- Bad: querying a posted property ID without `user_id`, or rebuilding balances
  from display strings in a mobile template.

#### 6. Tests Required

- Route authentication and method tests for every canonical endpoint.
- Template tests for semantic `data-*` IDs, required reason/confirmation
  markers, empty and partial states.
- Disposable MySQL test with two users asserting list and detail isolation;
  reset the database before and after the run.

#### 7. Wrong vs Correct

Wrong:

```go
db.First(&row, r.URL.Query().Get("property_id"))
```

Correct:

```go
row, err := repo.findProperty(ctx, sessionUserID, propertyID)
```

The route contract makes ownership and semantic page fields explicit at the
server boundary instead of leaving either concern to a renderer.

---

### Scenario: The DB-backed test group is skipped unless a DSN is set

#### 1. Scope / Trigger

- Trigger: any change to a MySQL-backed path (ledger, dunning, cash receipts,
  tenant profile, transaction actions), **and** any verification claim that
  rests on `go test ./...`.
- Several scenarios above end with "Run `go test ./... -count=1`". That command
  does **not** run the database tests. It reports `ok bank/cmd/truelayer-demo`
  while a whole group of tests never executes.

#### 2. Signatures

- `openLedgerMySQLTestDB(t)` (`ledger_mysql_test.go:16-31`) reads
  `RENTOPS_MYSQL_TEST_DSN` and calls `t.Skip("RENTOPS_MYSQL_TEST_DSN is not set")`
  when it is empty (`:18-21`). It is the single entry point for the group.
- Every DB-backed test selects through `-run 'MySQL'` (its name contains `OnMySQL`).
- `cmd/rentops-e2e/runner.go` reads the same key for its own live suite.

#### 3. Contracts

- DSN form: `root@tcp(127.0.0.1:3306)/<disposable-db>?parseTime=true&multiStatements=true`.
  `multiStatements=true` is required — the migration runner sends multi-statement
  files.
- The target database is **disposable and point-in-time**: the group calls
  `runMigrations(sqlDB, "../../migrations")` against it. Never point it at
  `rentops`, at a `rentops_audit_*` database you still need, or at anything
  reachable through the production host.
- Unset key -> every test in the group reports `SKIP`, the package reports
  `ok`. `PASS` and `SKIP` are indistinguishable in `go test ./...` output.

#### 4. Validation & Error Matrix

- Key unset -> group skipped, exit 0. Never read this as "verified".
- Key set, database missing -> `open MySQL test database` failure, not a skip.
- Key set, database reachable -> 22 tests run (counted 2026-09-20).
- Migration already applied -> `runMigrations` is idempotent; re-running against
  a used database is safe, unlike re-running the seed scripts.

#### 5. Good/Base/Bad Cases

- Good: create a named throwaway database, run
  `RENTOPS_MYSQL_TEST_DSN=... go test ./cmd/truelayer-demo/ -run 'MySQL' -count=1 -v`,
  read the per-test lines, then drop the database. The `-v` and the `-run` filter
  are both required — without `-v` you cannot tell a skip from a pass.
- Base: the group is genuinely out of scope for the change at hand. Say so
  explicitly ("MySQL group not run: no DSN") instead of quoting `go test ./...`
  as if it covered it.
- Bad: reporting "all tests pass" on the strength of `go test ./...`, or
  pointing the DSN at a database whose contents you did not create.

#### 6. Tests Required

- Before claiming a DB-backed path is verified, run the group with a DSN and
  quote the failing test names, if any.
- Two known states as of 2026-09-20, both **pre-existing** and unrelated to the
  09-20 mobile/notice fixes:
  - `TestDunningDashboardHTTPWorkflowOnMySQL` — deterministically red. It asserts
    `/rent-dashboard` contains `邮件催缴` and the tenant email; the 09-19 dashboard
    alignment changed what that page renders. Fix the assertion or the page.
  - `TestDunningSendWorkflowOnMySQL` — **flaky**, roughly 20–40% red under load,
    on trees both before and after 09-20 (measured 7/17 vs 4/18, not
    distinguishable). It fires two concurrent `service.send` calls with the same
    `request_key` and requires both to come back holding the same `sent` attempt.
    Treat a single red as inconclusive; sample it.
- Re-measure both before relying on this list.

#### 7. Wrong vs Correct

Wrong:

```bash
go test ./... -count=1
# ok  bank/cmd/truelayer-demo  1.3s
# ...quoted as "the ledger and dunning paths are verified".
```

Correct:

```bash
mysql -e "create database rentops_check_$$ character set utf8mb4"
RENTOPS_MYSQL_TEST_DSN="root@tcp(127.0.0.1:3306)/rentops_check_$$?parseTime=true&multiStatements=true" \
  go test ./cmd/truelayer-demo/ -run 'MySQL' -count=1 -v
mysql -e "drop database rentops_check_$$"
```

---

## Code Review Checklist

<!-- What reviewers should check -->

(To be filled by the team)
