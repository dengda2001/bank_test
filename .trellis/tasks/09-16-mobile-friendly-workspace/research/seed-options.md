# Research: reproducible browser-audit data set (seed options)

- **Query**: What already exists for seeding a disposable RentOps DB that a browser audit could run against; enumerate `cmd/rentops-e2e` fixtures, the render branches they cover, whether the fixture machinery is reusable, and the work/gaps of four seed approaches.
- **Scope**: internal (Go, shell, migrations at `/Users/dd/projects/bank`). No external search.
- **Date**: 2026-09-17
- **Related task**: `/Users/dd/projects/bank/.trellis/tasks/09-16-mobile-friendly-workspace`
- **Predecessor context**: `research/verification-infra.md` §6 (the 10 production tenants are not reproducible from the repo).

All paths are absolute; line numbers are from the current working tree. No credential values are recorded here.

---

## 1. What `cmd/rentops-e2e/` actually creates

There are two distinct layers: (a) files materialized from the in-memory manifest, and (b) rows created over HTTP by the scenarios. The DB rows are the audit-relevant part.

### 1a. Fixture files materialized on disk

`materializeE2ELegacyFixtures(manifest, directory)` — `cmd/rentops-e2e/legacy_fixture.go:59`. It requires an empty `chmod 700` directory (`:71-93`) and writes exactly three `0600` files (`:94-98`):

| File | Content |
|---|---|
| `bank-results.jsonl` | one `e2eLegacyResult` JSON object + `\n` (`:104-108`), built by `legacyResultFromManifest` (`:146-183`) from `manifest.Bank` |
| `tenants.json` | literal `[]\n` (`:109`) |
| `expenses.json` | literal `[]\n` (`:110`) |

So the disk fixtures seed **only bank transactions**; tenants and expenses are deliberately empty and are created later through the HTTP API.

`e2eLegacyTransaction` fields emitted (`legacy_fixture.go:43-57`, populated at `:150-167`): `transaction_id`, `normalised_provider_transaction_id`, `provider_transaction_id`, `timestamp`, `description`, `amount` (json.Number, e.g. `950.00`), `currency`, `transaction_type` (set to the fixture's `TransactionType`, e.g. `CREDIT`), `transaction_category` (same value), `reference`, optional `payer_id`, `payer_name`, and `meta.provider_reference` + `meta.counter_party_preferred_name`.

The fixture facts come from `newE2EFixtureManifest(runID, now)` — `cmd/rentops-e2e/manifest.go:90-218`, validated at `:220-297`.

### 1b. In-memory manifest entities

| Entity | Fields | Location |
|---|---|---|
| Main tenant (`manifest.Tenant`) | name, display_alias, email, payer_id, payer_name_hint, monthly_rent = **950.00 EUR**, interval_unit=month, interval_count=1, billing_start_date=2026-08-01, due_day=5, rent_start_date=2026-08-01, status=active, room_label, room_address | `manifest.go:95-110` |
| Dunning tenant (`manifest.DunningTenant`) | same shape, monthly_rent = **500.00 EUR**, billing_start_date=2026-09-01, rent_start_date=2026-09-01 | `manifest.go:112-127` |
| Payer (`manifest.Payer`) | name, payer_id | `manifest.go:111`; type `e2ePayerFixture` `:32-35` |
| Bank account | account_id, account_name, currency=EUR, batch_id | `manifest.go:60-66`, `:128-132` |
| Bank transactions (5) | see table below | `manifest.go:133-193` |

Bank transactions (`manifest.go:133-193`): all `Direction: "income"`, `TransactionType: "CREDIT"`:

| # | id | amount | timestamp | description | payer | parsed period implication |
|---|---|---|---|---|---|---|
| 0 | `-tx-eur-full` | 950.00 EUR | 2026-09-03 | `… rent full payment` | payer set | none (no month text) |
| 1 | `-tx-eur-partial` | 400.00 EUR | 2026-09-04 | `… rent partial payment` | payer set | none |
| 2 | `-tx-gbp` | 25.00 GBP | 2026-09-05 | `… foreign currency 2026-09` | none | 2026-09 (number) |
| 3 | `-tx-eur-cross-month` | 300.00 EUR | 2026-09-01 | `… rent 2026-08 cross-month` | none | 2026-08 |
| 4 | `-tx-eur-pending` | 50.00 EUR | 2026-09-06 | `… pending income` | none | none |

Expected snapshot: 2 tenants, 2 payers, 5 bank transactions, EUR income 1700.00, GBP income 25.00, RentAllocationCents 95000, PartialRentCents 40000, PendingIncomeCount 3 (`manifest.go:203-212`).

### 1c. Rows created over HTTP (the actual DB contents)

`runE2EBusinessScenarios` — `cmd/rentops-e2e/business_workflow.go:20-109`, in order:

| Entity | Created by | Fields set |
|---|---|---|
| Main tenant | `POST /tenants` via `tenantScenario` — `tenant_scenario.go:16-141`; form `e2eTenantFormValues` `:188-207` | name, display_alias, email, payer_id, payer_name_hint, monthly_rent, currency, interval_unit, interval_count, billing_start_date, due_day, rent_start_date, status, room_label, room_address, property_hint. Then a second POST updates `display_alias` and `room_address` (both + `" updated"`) — `:100-137` |
| Dunning tenant | `POST /tenants` via `dunningTenantScenario` — `tenant_scenario.go:143-186` | same form, no update |
| Payer relation | `POST /tenants/{id}/payers` — `tenant_scenario.go:61-79` | `payer_name`, `payer_id` (one relation for the main tenant) |
| Bank transactions | `POST /import-legacy` — `bank_scenario.go:16-30`, reads `TL_LOG_FILE` → 5 rows; ingest defaults each to `match_status = "unmatched"` (`legacy_import.go:57` → `transactions.go:137`) | see §1b |
| Rent obligations | lazily generated by `ensureMonthlyObligations` when pages/history load — `obligations.go:164-201` | user_id, tenant_id, period_month, due_date (due_day), expected=monthly_rent, paid=0, currency, status=`open`, record_status=active, generated_by=lazy (`obligations.go:181-192`) |
| Allocations / match status | `/billing/confirm` (`ledger_scenario.go:57-76`) and `/billing/allocate` (`:78-173`) | `payment_allocations` rows; `payment_transactions.match_status` is rewritten by `updateTransactionProjection` (`transaction_actions.go:370-379`, value from `summarizeTransactionAllocations` `transaction_allocation.go:43-50`) |
| Ignore | `/billing/ignore` on the GBP tx — `ledger_scenario.go:175-210` | `match_status = "ignored"` |
| Revoke + re-confirm | `/billing/revoke` then re-confirm tx0 — `ledger_scenario.go:212-287` | back to `matched` |
| Cash receipts | `/cash-receipts/preview` + `/cash-receipts` + `/cash-receipts/void` — `cash_scenario.go:16-327` | 300.00 EUR (voided), then corrected 350.00 EUR for period 2026-08 |
| Dunning | `POST /dunning/config`, `POST /dunning/preview`, `POST /dunning/send` (skipped when `RENTOPS_E2E_SKIP_DUNNING_DELIVERY=1`) — `dunning_scenario.go:15-134` | sender config + preview + optional send attempts |
| Expenses | **none** — `expenses.json` is `[]` (`legacy_fixture.go:110`); no `/expenses` POST anywhere in `cmd/rentops-e2e` | — |

### 1d. Final stored `match_status` after a passing run

Because every allocate/confirm step allocates the transaction's **full** amount, `summarizeTransactionAllocations` (`transaction_allocation.go:43-50`) resolves remainder 0 → `matched`:

- tx0 `matched` (full 950 to 2026-09), tx1 `matched` (400 → 300 rent 2026-08 + 100 deposit), tx2 `ignored`, tx3 `matched` (300 → 2026-08 rent), tx4 `matched` (50.00 → other income).
- No row is left `partial`, `unmatched`, `candidate`, or `needs_review` at rest.
- `candidate` / `needs_review` are never written to `payment_transactions.match_status` by any code path: the only writers are ingest (`transactions.go:137`, always `unmatched`), the action projector (`transaction_actions.go:55-67`), and manual balance (`dashboard_manual_balance.go:98`). `candidate`/`needs_review` are derived **read-side only** in `matching_service.go:140-163` / `matching.go`. Migrations never write them (`grep match_status migrations/*.sql` → only the column + index, `001_initial_mysql.sql:94,100`).

### 1e. The runner deletes everything before it finishes

`executeE2ERun` — `cmd/rentops-e2e/execution.go:12-92`: after business scenarios it calls `cleanupE2ERun` (`:62`), which `deleteE2ERunRows` deletes every row for the run user across 13 tables plus the user itself (`cleanup.go:639-669`), then verifies zero residue (`:671-690`; residue table list `:82-96`). Only **after** that does `run-e2e-local.sh` drop the DB (or keep it with `KEEP_DATABASE=1`, by which point the run data is already gone).

**Consequence**: "point the audit at the DB it creates" cannot be done after the runner exits — the fixture rows no longer exist.

---

## 2. Coverage gap vs. render branches

### 2a. `/billing` `match_status` branches

Filter options are `""`(全部), `pending`, `matched`, `partial`, `candidate`, `unmatched`, `needs_review`, `ignored` — `cmd/truelayer-demo/billing_page.go:174`; validation `transactions.go:315`. `pending` expands to `pendingMatchStatuses = {"candidate","needs_review","unmatched","partial"}` and applies only to income (`transactions.go:76`, `:593-596`). Row badge/label map at `transactions.go:444-451`.

| Branch | Covered at rest? | Evidence |
|---|---|---|
| `matched` | **yes** (tx0/tx1/tx3/tx4) | §1d |
| `ignored` | **yes** (tx2) | `ledger_scenario.go:175-210` |
| `pending` (synthetic: any of the 4) | **no** at rest | all 5 rows are matched/ignored after the run |
| `partial` | **no** at rest (transient only) | full-amount allocations resolve to `matched` |
| `unmatched` | **no** at rest (present only between import and the ledger scenario) | ingest `transactions.go:137` |
| `candidate` | **no** — never stored | §1d |
| `needs_review` | **no** — never stored | §1d |
| expense row (`Direction = "expense"`, `<tr class="expense">`) | **no** — manifest is all CREDIT; no DEBIT anywhere | `manifest.go:133-193`, `transactions.go:187-202` |
| filter-error notice (`?error=` / `invalid_filter`) | **no** — error redirects are asserted as `Location`, the error page is not re-fetched | `main.go:644-646`; `ledger_scenario.go:162-165` |

### 2b. `/rent-dashboard` `status` branches

Allowed values `all/paid/partial/unpaid/open/overdue/needs_review` — `dashboard_filters.go:64-73`; `unpaid` → `open|overdue|partial` (`:85-88`); labels `obligations.go:582-586`; status derived by `obligationStatus` (`obligations.go:157-162`) from expected/paid/due vs now, with `needs_review` only when the obligation row's own status is `"needs_review"`.

| Branch | Covered at rest? | Why |
|---|---|---|
| `all` | **yes** (default) | — |
| `paid` | **yes** | main tenant 2026-08 reaches 950/950 (600 allocations + 350 cash correction); 2026-09 reached 950/950 via tx0 |
| `unpaid` | **yes** via `overdue` | dunning tenant |
| `overdue` | **yes** | dunning tenant 2026-09 (expected 500, paid 0, due 2026-09-05 < run date 2026-09-17) |
| `partial` | **no** at rest (transient while cash scenario runs) | by the final read 2026-08 is fully paid |
| `open` | **no** | requires due date in the future; fixtures due on the 5th, run date the 17th |
| `needs_review` | **no** | no path sets `rent_obligations.status = "needs_review"` for these fixtures (§1d) |
| invalid-filter error notice (`筛选条件无效`) | **yes** | `dashboard_scenario.go:90-107` |

### 2c. Requested edge cases

| Edge case | Covered? | Evidence |
|---|---|---|
| Very long Chinese name | **no** | all fixture names are `rentops-e2e-YYYYMMDD-HHMMSS-xxxxxxxx tenant` (ASCII, ~45 chars) — `manifest.go:96`, `:113` |
| Long English name | **no** (only the ~45-char run-ID marker) | same |
| Long address | **no** (`marker + " address"` / `" dunning address"`) | `manifest.go:109`, `:126` |
| Long bank description | **no** (`marker + " rent full payment"` etc.) | `manifest.go:139,152,165,176,187` |
| Large amount | **no** — max is 950.00 | `manifest.go:101,140` |
| Small / non-round decimal | **partial** — 25.00, 50.00, 1.00 (rejected) exist, but no sub-cent or odd-decimal (e.g. 12,345.67) | `manifest.go:166,188`; `cash_scenario.go:291` |
| Empty list | **no** — the runner never renders an empty list; the second account is empty but never browsed | `business_workflow.go:97-104` |
| Error state | dashboard **yes**, billing **no** | §2a/§2b |
| Multiple payment records per tenant | **yes** | main tenant 2026-08 has 3 confirmed rent allocations + cash receipt (`ledger_scenario.go:78-126`, `cash_scenario.go`) |
| Cross-month split allocation | **yes** (transient; tx3 timestamp 2026-09-01 allocated to 2026-08) | `ledger_scenario.go:104-126` |

Also **no expenses at all** for `/expenses` (0 rows) and no `/tenants` second-page/empty state. Only 2 tenants exist, both machine-named.

---

## 3. Is the e2e fixture machinery reusable from a browser audit?

**No, not as a library, and not as a runnable seed-only step.**

- All entry points are **unexported in `package main`** of a separate module binary: `newE2EFixtureManifest` (`manifest.go:90`), `materializeE2ELegacyFixtures` (`legacy_fixture.go:59`), `executeE2ERun` (`execution.go:12`), `cleanupE2ERun` (`cleanup.go:98`). A browser harness (Node/Playwright) cannot import them; a Go caller cannot import `package main`.
- The binary exposes only these flags — `cmd/rentops-e2e/main.go:19-24`:
  ```go
  flag.StringVar(&options.BaseURL, "base-url", ...)
  flag.StringVar(&options.RunID, "run-id", ...)
  flag.StringVar(&options.ReportPath, "report", ...)
  flag.StringVar(&options.TargetName, "target", ...)
  flag.BoolVar(&options.Execute, "execute", false, ...)
  ```
  There is **no seed-only / fixture-only flag**. `-execute` always runs `executeE2ERun` (`main.go:63-74`), which always continues into cleanup (`execution.go:62`).
- The only seam that materializes fixtures without cleanup is a unit test, e.g. `TestMaterializeE2ELegacyFixturesCreatesExclusivePrivateInputs` (`runner_test.go:1201-1229`), which calls the unexported func directly. Not usable outside the package.
- Configuration is env-driven (`e2eOptionsFromEnv`, `runner.go:127-147`) and the runner refuses to run without a write confirmation, isolated DSN, run-ID-prefixed username, etc. (`runner.go:158-256`).
- `options.FixtureDir` must be an **empty** directory with no group/other access (`legacy_fixture.go:71-93`), and `executeE2ERun` removes the three files afterwards (`execution.go:79`, `:106-121`).

Conclusion: the fixture machinery is **welded to the acceptance-runner control flow** and specifically designed to leave nothing behind. Driving it independently would require new code (a seed-only mode or an exported package), not a flag.

---

## 4. Existing seed-ish paths for tenants/transactions

### 4a. `RENTOPS_TENANT_FILE` / `RENTOPS_EXPENSE_FILE`

- Parsed in `loadConfig`: `cmd/truelayer-demo/main.go:451-452`; defaults `rentops-tenants.json` / `rentops-expenses.json`. `TL_LOG_FILE` (bank JSONL) at `:449`.
- **No startup import.** `main()` only calls `a.auth.seedDefaultUser` (`main.go:389`); `ImportLegacyFiles` is reachable only through `POST /import-legacy` (`main.go:411` → `legacy_import.go:23-41`).
- Formats (`readJSONFile`, `main.go:1983-1998` → `json.Unmarshal`):
  - `TL_LOG_FILE`: JSONL stream of `demoResult` objects — `main.go:101-106`; decoded by `readDemoResults` (`legacy_import.go:113-138`).
  - `RENTOPS_TENANT_FILE`: JSON **array** of `tenantRecord` — `main.go:185-209` (note `MonthlyRent` is a `float64`, e.g. `950.0`, not cents).
  - `RENTOPS_EXPENSE_FILE`: JSON **array** of `expenseRecord` — `main.go:211-225`.
  - These same files also back the **no-DB fallback** pages via `a.loadTenants` / `a.loadExpenses` (`main.go:1959-1981`), which is why the run script sets them to empty arrays (`run-e2e-local.sh:87-88`).
- Import semantics: transactions are ingested with `MatchStatus: "unmatched"` and **no allocations** (`legacy_import.go:56-61` → `transactions.go:137,147-158`); tenants via `tenantStore.createTenant` (`legacy_import.go:83`); expenses via `expenseStore.createExpense` (`:105`). Tenant input defaults: currency EUR, interval month, count 1, due day 1, status active (`legacy_import.go:147-172`).
- This path **seeds no rent obligations, no allocations/confirmed payments, no cash receipts, no dunning attempts, and no `matched`/`partial` rows**. Obligations would appear as `open`/`overdue` with paid 0 after page loads (lazy generation).

### 4b. `POST /import-legacy` and `legacy_import.go`

- Handler `handleLegacyImport` (`legacy_import.go:23-41`) requires auth and **ignores the request body entirely**; it reads `a.cfg.LogFile/TenantFile/ExpenseFile`. So a committed JSON fixture can drive it **only if the running app's env points at those files**.
- `ImportLegacyFiles` (`legacy_import.go:46-111`) is idempotent: transactions use `ON CONFLICT (user_id, stable_transaction_key) DO NOTHING` (`transactions.go:150-155`); tenants matched on payer_id or (name+room_address) (`:74-82`); expenses on description+amount+date (`:100`).
- A committed fixture could therefore create tenants + bank transactions (realistic names/amounts are possible — the schema is free-form), but it cannot create the action-driven states the audit needs (`matched`/`partial`/`ignored`/cash receipts) unless additional POSTs are made.
- Repo-root `bank-data.json` and `log.json` exist locally but are **gitignored/untracked** (`.gitignore` lines 3-4; `git ls-files` confirms neither is tracked), so they are not committed fixtures. `demo/rent-management-v1-dashboard.html` is tracked but is a static mock, not seed data.

---

## 5. The disposable-DB runner (`scripts/run-e2e-local.sh`)

Read in full. What it does:

| Aspect | Detail | Line |
|---|---|---|
| Port | `APP_PORT` default `18080`; `BASE_URL=http://127.0.0.1:18080` | `:18-19` |
| Target name | `rentops-e2e-local` | `:20` |
| Run ID / DB name | `rentops-e2e-<date>-<hex>` → DB name same with `-`→`_`; refuses unless `rentops_e2e_*` | `:22-43` |
| MySQL | `MYSQL_HOST` default `127.0.0.1`, `MYSQL_PORT` default `53306`; creates DB + random user/password via `sudo mysql` | `:23-27`, `:58-68` |
| DSN | `user:pass@tcp(host:port)/db?charset=utf8mb4&parseTime=True&loc=UTC` | `:70` |
| Fixture dir | `${RUNTIME_DIR}/fixtures`, `mkdir -p` + `chmod 700` | `:35`, `:73-74` |
| App env | `TL_ADDR`, `TL_ENV=sandbox`, `TL_LOG_FILE=${FIXTURE_DIR}/bank-results.jsonl`, `RENTOPS_TENANT_FILE=…/tenants.json`, `RENTOPS_EXPENSE_FILE=…/expenses.json`, `TL_TOKEN_FILE`, `MYSQL_DSN`, `MIGRATIONS_DIR`, `BANK_TOKEN_ENCRYPTION_KEY`, `APP_SESSION_SECRET`, `APP_ADMIN_USERNAME/PASSWORD` | `:81-96` |
| Account seeding | `start_app` sets `APP_ADMIN_USERNAME=<RUN_ID>-primary`; the app's `seedDefaultUser` (`auth.go:37`, called `main.go:389`) creates it. It is run **twice**: once for the second account (`:121-124`), then for the primary (`:126-127`) | `:76-127` |
| Readiness | polls `GET /login-local` until `405`/`200` | `:99-113` |
| Runner invocation | full env block `RENTOPS_E2E_*` incl. `RENTOPS_E2E_MYSQL_DSN`, `RENTOPS_E2E_FIXTURE_DIR`, primary + second credentials, `SKIP_DUNNING_DELIVERY=1`, both confirmations; runs `rentops-e2e -execute -report` | `:130-145` |
| Teardown | on pass, drops DB+user unless `KEEP_DATABASE=1`; on failure keeps DB+runtime dir | `:154-167` |

Key nuance for a browser harness: the script runs the runner **synchronously and then the runner cleans the rows** (§1e), and on success the script drops the DB. Even `KEEP_DATABASE=1` leaves an effectively **empty** DB (only the leftover second account survives, because `deleteE2ERunRows` only deletes the primary user). There is no hook, background mode, or "pause before cleanup" in the script, and the port/data only exist between `start_app` (`:127`) and `cleanup` (`:148`).

**Answer**: a browser harness cannot simply "attach" to a completed `run-e2e-local.sh` run; it would need either a modified/second script that starts the app on a disposable DB and seeds it **without** running the destructive acceptance suite, or a seed-only mode injected before `cleanup`.

---

## 6. Recommendation input: work required and what each approach would NOT give

### (a) Extend `cmd/rentops-e2e` fixtures and point the audit at the DB it creates

**Work**: extend `newE2EFixtureManifest` (`manifest.go:90`) with the missing shapes (a third/fourth tenant with long CJK/English name/address, a large amount, a DEBIT/expense transaction, a deliberately unallocated `unmatched` row, a `partial` row) and extend the scenarios to leave them in that state; then add a **new mode/flag** (`main.go:19-24`) that materializes + seeds but **stops before `cleanupE2ERun`** (`execution.go:62`), plus a runner-script variant that starts the app, seeds, and blocks (or exports the DSN/runtime dir) so Playwright can attach.

**Would NOT give us**:
- Anything without modifying the runner — today `-execute` always deletes every row (`cleanup.go:639-669`) and removes the fixture files (`execution.go:79`).
- `candidate` / `needs_review` stored statuses: no code path writes them (`§1d`), so even fixtures cannot make those badges/branches appear on `/billing`.
- A realistic "empty list" or error state unless specifically scripted (the runner never asserts/renders them — `§2c`).
- Reuse as a library: funcs are unexported `package main` (`§3`).
- Realistic data — every value is force-prefixed with the run ID by validation (`manifest.go:230-280`), so long-name / natural-text branches stay untested.

### (b) A new dedicated seed (Go CLI or SQL file) for audit data

**Work**: add e.g. `cmd/seed-audit/` or `scripts/seed-audit.sql`. A SQL seeder must match the migration schema (`migrations/001`–`008`) and respect per-user scoping, `payment_allocations` effective-status rules (`ledger.go`, `transaction_allocation.go`), and obligation invariants. A Go seeder cannot import the app package (`cmd/truelayer-demo` is `package main`), so it must either live inside `cmd/truelayer-demo` behind a new flag or duplicate the service logic. The audit runner script must then invoke it against the disposable DB.

**Would NOT give us**:
- The read-side derived flags are **not stored**: `CanConfirm`, `NeedsMonthChoice`, `ManualMatchOptions`, `CanRematch`, `RematchTenantOptions/MonthOptions` are computed per request in `matching_service.go:136-169`; a SQL seeder can only influence them indirectly via payers/obligations/allocations.
- Free cleanup/validation safety nets: the e2e cleanup owns/validates everything by user ID (`cleanup.go:210-637`); a new seeder would need its own idempotence and teardown.
- Nothing for the acceptance suite — it is a separate artifact with its own maintenance.

### (c) Drive `POST /import-legacy` with a committed JSON fixture

**Work**: commit `bank-results.jsonl` (+ optionally `tenants.json` / `expenses.json`) in the `demoResult` / `tenantRecord` / `expenseRecord` formats (`main.go:101-106,185-225`); run the app with `TL_LOG_FILE` / `RENTOPS_TENANT_FILE` / `RENTOPS_EXPENSE_FILE` pointed at them; POST `/import-legacy` as an authenticated user. Idempotency already exists (`legacy_import.go:46-111`).

**Would NOT give us**:
- Any confirmed/allocated payments or match states beyond `unmatched` (`transactions.go:137`); no `matched`, `partial`, `ignored`, `candidate`, `needs_review` rows.
- Any cash receipts, dunning attempts, or payer relations (payer hint fields land on the tenant but `tenant_payers` relations are created elsewhere — `tenant_scenario.go` does it via `/tenants/{id}/payers`; `legacyTenantInput` only sets `PayerID`/`PayerNameHint`).
- No obligation statuses other than lazy `open`/`overdue` with paid 0 (`obligations.go:164-201`).
- The POST body is ignored (`legacy_import.go:23-41`), so the fixture must already be at the configured path when the app starts.

### (d) `RENTOPS_TENANT_FILE` / `RENTOPS_EXPENSE_FILE` JSON

**Work**: none beyond committing the JSON arrays and pointing the env vars at them — **if** you also drive an import path. Note there is **no auto-import**; `main()` only seeds the admin user (`main.go:389`), and the files are otherwise only read by `/import-legacy` (`legacy_import.go:64,90`) or the no-DB fallback pages (`main.go:1959-1981`).

**Would NOT give us**:
- Any DB rows unless `/import-legacy` is POSTed (or the app runs without a DB, which is not the audit target).
- Bank transactions unless `TL_LOG_FILE` is also supplied; expenses only (no obligations/payments) if only the expense file is supplied.
- Everything listed under (c): no matched/partial/ignored states, no cash receipts, no dunning.
- Because `readJSONFile` returns nil on missing/empty files (`main.go:1983-1998`), a misconfigured path silently seeds nothing.

---

## Caveats / Not Found

- Stored `match_status` values `candidate` and `needs_review` appear to be **unreachable** in `payment_transactions` with the current writers (ingest = `unmatched`; actions = `matched`/`partial`/`ignored`/`unmatched`). If confirmed, the `/billing` filter options for those two statuses can never match a row. Reported as an observation, not a recommendation.
- `/rent-dashboard` `needs_review` depends on `rent_obligations.status == "needs_review"` (`obligations.go:280,371`); no code path found that sets that value for these fixtures — only `main.go:1768` (legacy in-memory fallback) and `dashboard_manual_balance`-adjacent paths. Not exhaustively proven across every service.
- The exact set of stale transient states mid-run (e.g. `partial` between the ledger and cash scenarios) depends on scenario order in `business_workflow.go:36-84`; the marked coverage is for the **final at-rest** state.
- `demo/rent-management-v1-dashboard.html` is a static design mock and is not a data seed.
- No external references were needed.
