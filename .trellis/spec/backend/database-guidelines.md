# Database Guidelines

> Database patterns and conventions for this project.

---

## Overview

The application uses MySQL-compatible storage through GORM. SQL files under
`migrations/` are the schema source of truth and are applied by the startup
migration runner; production code must not call GORM `AutoMigrate`.

JSON and JSONL files remain compatibility inputs for the one-time
`POST /import-legacy` import and for legacy fallback tests. The authenticated UI
uses account-scoped database rows after a v2 user session is established.

Every business query and write must include the current `user_id`. Bank refresh
tokens belong to `bank_connections.user_id` and are encrypted with
`BANK_TOKEN_ENCRYPTION_KEY`; plaintext storage is only an explicit non-live
development escape.

## Scenario: MySQL/GORM Persistence

### 1. Scope / Trigger

- Trigger: Any new persistent tenant, rent, transaction, allocation, expense,
  bank connection, or account behavior.
- Models use GORM for queries, but schema changes are explicit SQL migrations.
- Monetary values are stored as integer cents; transaction direction is stored
  separately as `income` or `expense`.

### 2. Signatures

- `MYSQL_DSN`: preferred native MySQL DSN.
- `DATABASE_URL`: optional `mysql://...` compatibility URL.
- `MIGRATIONS_DIR`: migration directory, default `migrations`.
- `BANK_TOKEN_ENCRYPTION_KEY`: raw or base64-encoded 32-byte AES key.
- `ALLOW_PLAINTEXT_TOKENS=1`: sandbox/development-only escape hatch.
- `ImportLegacyFiles(ctx, db, userID, cfg)`: idempotent JSON/JSONL import.

### 3. Contracts

- `users` is the real account source. Environment variables only seed the
  configured default user if its username is absent; they never overwrite an
  existing password.
- `tenants`, `rent_obligations`, `payment_transactions`,
  `payment_allocations`, `manual_expenses`, and `bank_connections` all carry
  `user_id` and must be filtered by it.
- `payment_transactions` uses `(user_id, stable_transaction_key)` for idempotent
  bank ingestion.
- Rent obligations are monthly in phase 1, but retain interval fields and a
  reusable batch generation service for later expansion.
- Payer ID is preferred for matching. Exact name matches are candidates until
  a user confirms them; confirmation may backfill the tenant payer ID.

### 4. Validation & Error Matrix

- Missing database configuration -> startup fails; do not silently switch the
  primary UI back to JSON.
- Unapplied migration failure -> startup fails.
- Live environment without token encryption key -> configuration fails.
- Cross-account tenant, transaction, allocation, expense, or token lookup ->
  returns no row or an authorization-safe error.
- Duplicate bank import -> no duplicate transaction rows.

### 5. Good/Base/Bad Cases

- Good: HTTP handlers obtain `userID` from a signed v2 session and pass it into
  service methods; services use GORM with explicit ownership predicates.
- Base: A fresh MySQL database starts with migrations and seeds one configured
  default user.
- Bad: Reading a global token file or JSON ledger for an authenticated user's
  database page, using AutoMigrate, or trusting a posted tenant/transaction ID
  without an ownership condition.

### 6. Tests Required

- DSN precedence and token-storage configuration.
- bcrypt login and signed user session parsing.
- Stable transaction normalization and duplicate-key behavior.
- Payer ID/name matching, month hint parsing, obligation status, and manual
  confirmation ownership.
- Legacy JSON/JSONL import mapping and repeat-import idempotence.

## Scenario: Legacy JSON Storage

### 1. Scope / Trigger

- Trigger: Any feature that persists demo data outside the TrueLayer bank log or
  token file.
- Current examples: compatibility fallback when no v2 database session exists.

---

### 2. Signatures

- `RENTOPS_TENANT_FILE`: JSON array file for `[]tenantRecord`; default
  `rentops-tenants.json`.
- `RENTOPS_EXPENSE_FILE`: JSON array file for `[]expenseRecord`; default
  `rentops-expenses.json`.
- `loadTenants() ([]tenantRecord, error)` and `saveTenants([]tenantRecord) error`
  own tenant persistence.
- `loadExpenses() ([]expenseRecord, error)` and `saveExpenses([]expenseRecord) error`
  own expense persistence.
- `readJSONFile(path string, out any) error` treats missing or empty files as an
  empty data set.
- `writeJSONFile(path string, value any) error` writes indented JSON with file
  mode `0600`.

---

### 3. Contracts

Tenant record fields:

- `id`: generated local record id.
- `name`: required, trimmed.
- `monthly_rent`: required positive number.
- `currency`: optional, defaults to `EUR`.
- `room_address`: required, trimmed.
- `created_at`: UTC RFC3339 timestamp.

Expense record fields:

- `id`: generated local record id.
- `description`: required, trimmed.
- `category`: optional, defaults to `General`.
- `amount`: required positive number.
- `currency`: optional, defaults to `EUR`.
- `expense_date`: `YYYY-MM-DD`; invalid or empty input falls back to current UTC
  date.
- `payment_method`: optional, defaults to `Manual`.
- `created_at`: UTC RFC3339 timestamp.

These files are legacy compatibility data. They must not store bank access
tokens or raw OAuth payloads; bank data is imported into the account-scoped
database transaction table.

---

### 4. Validation & Error Matrix

- Missing tenant file -> render empty tenant list.
- Empty tenant file -> render empty tenant list.
- Missing expense file -> render empty expense list.
- Empty expense file -> render empty expense list.
- Tenant POST missing `name`, `monthly_rent`, or `room_address` -> redirect to
  `/tenants?error=invalid_tenant`.
- Expense POST missing `description` or positive `amount` -> redirect to
  `/expenses?error=invalid_expense`.
- Invalid expense date -> save with current UTC date rather than rejecting the
  whole expense.
- File read/write failure -> return HTTP 500 from the route handler.

---

### 5. Good/Base/Bad Cases

- Good: Form handlers validate once at the HTTP boundary, append a typed record,
  then call the storage owner.
- Base: A fresh checkout with no ledger files can open `/tenants` and
  `/expenses` without creating files until the first POST.
- Bad: Rendering code parses untyped JSON maps directly, or form handlers write
  ad hoc JSON strings.

---

### 6. Tests Required

- Protected ledger routes redirect unauthenticated requests to `/`.
- Tenant POST persists tenant name, monthly rent, and room address to a temp file.
- Expense POST persists description, amount, category, and date to a temp file.
- Tests must use `t.TempDir()` for ledger files; do not create
  `rentops-tenants.json` or `rentops-expenses.json` in the repository root.

---

### 7. Wrong vs Correct

Wrong:

```go
body := fmt.Sprintf(`{"name":%q,"monthly_rent":%q}`, name, rent)
_ = os.WriteFile("rentops-tenants.json", []byte(body), 0o644)
```

Correct:

```go
tenants, err := a.loadTenants()
if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
tenants = append(tenants, tenantRecord{
    Name:        name,
    MonthlyRent: monthlyRent,
    RoomAddress: roomAddress,
})
if err := a.saveTenants(tenants); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
```

---

## Common Mistakes

- Do not use default ledger paths in tests or browser automation that submits
  forms. Use temp files through `RENTOPS_TENANT_FILE` and
  `RENTOPS_EXPENSE_FILE` so verification does not create untracked demo data in
  the repo.

## Scenario: Tenant Billing History Projection

### 1. Scope / Trigger

- Trigger: Any tenant-facing or landlord-facing view that displays multiple
  billing months and payment details together.
- Applies to the database-backed `/tenants` history projection and similar
  bounded rent-history views.

### 2. Signatures

- `obligationService.listTenantBillingHistory(ctx, userID, endMonth, monthCount)`
  returns `map[tenantID][]tenantBillingMonth`.
- `buildTenantBillingHistory(tenants, obligations, paymentRows, now)` owns the
  typed in-memory grouping from monthly bills to confirmed payment details.
- The persisted hierarchy is `rent_obligations` (parent) →
  `payment_allocations` (zero or many children) → `payment_transactions`
  (source detail).

### 3. Contracts

- A three-month request normalizes `endMonth` with `monthStart` and queries the
  inclusive current month plus the preceding two months.
- Each eligible tenant/month has at most one `rent_obligations` parent because
  `(user_id, tenant_id, period_month)` is unique.
- `tenantBillingMonth.PaidAmount` uses the existing obligation paid total,
  which is maintained from confirmed allocations and must match the child
  detail sum. Child rows are joined with the original transaction for amount,
  time, description, reference, and confirmation source.
- Only `payment_allocations.status = 'confirmed'` joined to income
  `payment_transactions` contributes to actual rent received. Unmatched,
  candidate, and needs-review transactions remain excluded.
- History view models are typed presentation data; templates must not query or
  expose GORM database structs directly.

### 4. Validation & Error Matrix

- `userID == 0` or `monthCount < 1` -> return a service error; do not query.
- Cross-user tenant, obligation, allocation, or transaction -> exclude it from
  the projection through ownership predicates and joins.
- Tenant not active for a month, before billing/rent start, or after rent end ->
  do not lazily create or display an obligation for that month.
- Eligible month with no confirmed allocation -> display the obligation with
  paid amount zero and an explicit empty detail state.
- Repeated GET/refresh -> obligation generation remains idempotent through the
  existing unique key; no payment allocation or transaction rows are written.

### 5. Good/Base/Bad Cases

- Good: Batch-fetch the bounded obligations and child detail rows, group in
  memory, and render `tenant → month → payment` with both levels collapsed.
- Base: A tenant with three confirmed transfers in one month gets one monthly
  summary and three child detail rows.
- Bad: Adding a separate monthly payment-group table, summing all bank income
  regardless of confirmation, or querying one allocation list per tenant/month.

### 6. Tests Required

- Projection unit test asserts newest-first month order and one parent per
  month, including three children under one month.
- Projection unit test asserts zero-payment months remain visible and
  obligations for other tenants are ignored.
- Database/service tests should assert three-month bounds, active-date rules,
  confirmed-status filtering, and user isolation.
- Template/browser tests should assert collapsed `aria-expanded` toggles,
  independent month expansion, and no nested-toggle event bubbling.

### 7. Wrong vs Correct

Wrong:

```go
// Rebuilds a second parent concept and counts every incoming bank transaction.
monthlyGroup := groupTransactionsByCalendarMonth(allIncomeTransactions)
```

Correct:

```go
// Use the existing persisted bill as parent and confirmed allocations as children.
history := buildTenantBillingHistory(tenants, obligations, confirmedPaymentRows, now)
```

## Scenario: Remembered Payer Association and Explicit Rent Month

### 1. Scope / Trigger

- Trigger: Manual income-to-tenant binding, automatic reconciliation, or any
  change to monthly rent dashboard totals.
- Applies to the database-backed `/billing` and `/rent-dashboard` flows.

### 2. Signatures

- `POST /billing/confirm` accepts `transaction_id`, `tenant_id`, and an optional
  `period=YYYY-MM`.
- `transactionService.confirmRentMatch(ctx, userID, transactionID, tenantID,
  period)` keeps tenant identity association separate from rent allocation.
- `payment_transactions.matched_tenant_id` stores a user-scoped remembered
  tenant association; `tenants.payer_name_hint` stores the exact normalized
  payer-name rule after confirmation.

### 3. Contracts

- A manual binding stores the payer name hint and associates all same-user,
  exact-normalized-name, unconfirmed income transactions that have not already
  been allocated. Existing confirmed allocations are never overwritten.
- A unique remembered payer name may automatically identify future transactions.
  It may allocate only when the transaction reference or transaction month
  selects an unpaid obligation. The nearest-open-month fallback is forbidden.
- If identity is known but month or amount cannot be safely allocated, the
  transaction remains `needs_review` with `matched_tenant_id` set. The UI must
  offer a month choice with expected, paid, and remaining amounts.
- A submitted month is checked for user/tenant ownership, currency and current
  remaining balance before allocation; allocation remains idempotent.

### 4. Validation & Error Matrix

- Missing or cross-user transaction/tenant -> safe confirmation error; no write.
- Same payer name maps to multiple remembered tenants -> `needs_review`; no
  automatic allocation.
- Paid referenced month with another unpaid month -> retain tenant association
  and require the user to select a month.
- Amount greater than selected obligation balance or currency mismatch -> keep
  pending review; do not increase paid amount.
- Duplicate confirmation or refresh -> no duplicate allocation or amount.

### 5. Good/Base/Bad Cases

- Good: One binding remembers the payer name, allocates eligible same-name
  payments, and exposes ambiguous payments for one-time month selection.
- Base: A payer ID still has priority; exact payer-name confirmation remains the
  fallback when the bank does not provide a stable ID.
- Bad: Automatically assigning an ambiguous payment to the closest historical
  unpaid month, or treating a name match as confirmed without user action.

### 6. Tests Required

- Matching unit tests assert remembered exact names use `auto_name` and paid
  referenced months return no automatic obligation.
- Service tests should assert batch association, sequential partial payments,
  duplicate confirmation, user isolation, conflict handling and selected-month
  validation against fresh balances.
- Template tests assert Chinese labels and the month-choice form show expected,
  paid and remaining values.

### 7. Wrong vs Correct

Wrong:

```go
// silently redirects a payment to the nearest unpaid month
obligation, ok := selectNearestOpenObligation(transaction, tenantID)
```

Correct:

```go
// associate identity first; allocate only the explicit or eligible month
if decisionNeedsReview {
    setMatchedTenant(transactionID, tenantID)
    setMatchStatus(transactionID, "needs_review")
}
```

## Scenario: Tenant Profile, Name-Only Payers, and Lifecycle History

### 1. Scope / Trigger

- Trigger: Adding tenant profile fields, maintaining payer relationships,
  changing rent validity dates, or rendering the independent tenant history
  page.
- Applies to database-backed `/tenants` and `/tenants/{id}` flows after
  migration `004_tenant_profile_and_payers.sql`.

### 2. Signatures

- `tenantService.createTenant(ctx, userID, tenantInput)` creates the tenant
  and any legacy-compatible initial payer relationship in one transaction.
- `tenantService.updateTenant(ctx, userID, tenantID, tenantInput)` locks the
  tenant and applies early-rent-end validation and future-obligation voiding.
- `tenantService.addTenantPayer(ctx, userID, tenantID, tenantPayerInput)` and
  `removeTenantPayer(ctx, userID, tenantID, payerID, removedBy, reason)` are
  account-scoped and soft-delete relationships.
- `obligationService.listTenantBillingHistoryPage(ctx, userID, tenantID,
  fromMonth, toMonth, page, pageSize)` returns typed, paginated month rows.
- `GET /tenants/{id}` accepts `from_month`, `to_month`, `page`, and
  `page_size`; payer forms use `/tenants/{id}/payers` and
  `/tenants/{id}/payers/remove`.

### 3. Contracts

- `tenant_payers.payer_name_original` is required and
  `payer_name_normalized` is the lower-case, whitespace-collapsed lookup key;
  `payer_id` is nullable. A bank payload containing only
  `meta.counter_party_preferred_name` (for example `Mike`) is valid and must
  not receive a fabricated ID.
- `tenant_payers.removed_at` preserves relationship history. Active payer
  sharing is detected by normalized name or stable payer ID across tenants;
  shared/conflicting rows cannot auto-select a tenant.
- `billing_start_date` defaults to `rent_start_date` only when omitted on
  create. It must be on/after rent start and on/before rent end when an end
  exists; editing must preserve the stored value unless explicitly changed.
- An end-date month remains billable. Only active obligations after the end
  month are candidates for voiding; any effective rent allocation blocks the
  entire tenant update. Unpaid candidates are voided with actor, timestamp,
  and reason in the same transaction.
- Tenant history reads include applicable zero-payment months and use only
  confirmed rent allocations joined to income transactions. All reads and
  writes include `user_id` ownership predicates.

### 4. Validation & Error Matrix

- Empty payer name or a field longer than 191 runes -> validation error; no
  relationship row is written.
- Cross-user tenant, payer, obligation, or allocation -> safe not-found or
  authorization-safe error; no data is changed.
- Invalid email, date ordering, non-EUR currency, or non-positive rent ->
  tenant form validation error.
- Early end with an effective future rent allocation -> reject the whole
  update and leave every obligation active.
- Early end with only unpaid future obligations -> void all affected rows;
  the end month remains active and historical rows stay queryable.
- A page number beyond the available range returns an empty page without an
  integer-overflow panic; history ranges over 120 months are rejected.

### 5. Good/Base/Bad Cases

- Good: Store `Mike` with a nullable payer ID, preserve its original spelling,
  and mark two active tenants with normalized `mike` as shared.
- Base: Existing `tenants.payer_name_hint` is backfilled into
  `tenant_payers`; old bank JSON remains readable and historical allocations
  are untouched.
- Bad: Treat `counter_party_preferred_name` as a stable ID, physically delete
  a removed relationship, hide old obligations when a tenant is inactive, or
  void some future obligations before discovering a payment on another one.

### 6. Tests Required

- Unit tests for name normalization, name-only validation, shared name/ID
  detection, date defaults, date boundaries, source labels, pagination, and
  large page values.
- Template/route tests for alias/email fields, independent history controls,
  payer add/remove forms, authentication, and lifecycle conflict messaging.
- Optional MySQL tests must run migration `004` twice, verify nullable payer
  ID and required original name, and assert unpaid future voiding versus
  paid-future all-or-nothing rejection.

### 7. Wrong vs Correct

Wrong:

```go
// Bank data has no stable payer ID here; inventing one makes future matching unsafe.
payerID := "payer:" + normalizeTenantPayerName(bankName)
```

Correct:

```go
// Keep the observed name and leave the stable identity absent.
tenantPayer{
    PayerID:             nil,
    PayerNameOriginal:   bankName,
    PayerNameNormalized: normalizeTenantPayerName(bankName),
}
```

## Scenario: EUR-Only Audited Rent Ledger

### 1. Scope / Trigger

- Trigger: Creating or changing a rent obligation, confirming a bank
  allocation, projecting paid/status totals, or applying migration `003`.
- Phase 1 accepts EUR only. Currency columns remain three-character fields so
  a later multi-currency implementation can add policy and FX without
  changing the ledger shape.

### 2. Signatures

- `normalizeLedgerCurrency(value string) (string, error)` canonicalizes the
  only supported phase-1 currency to `EUR`.
- `validateLedgerAllocation(ledgerAllocationCheck) error` validates ownership,
  source budget, obligation balance, kind, and currency before a write.
- `ledgerPaidAmount([]paymentAllocation) int64` sums effective rent
  allocations only; legacy rows with an empty kind are treated as rent.
- `projectLedgerObligation(rentObligation, []paymentAllocation, now)` derives
  the cached paid amount and Dublin-local status from effective allocations.
- `migrations/003_rent_ledger_foundation.sql` adds allocation kind, operation
  and idempotency metadata, void/audit fields, nullable obligation linkage for
  non-rent allocations, and obligation record status.

### 3. Contracts

- Valid allocation kinds are `rent`, `deposit`, and `other_income`; only
  `status = confirmed` rows are effective. Voided rows remain queryable audit
  history and never contribute to totals.
- Rent allocations require the same user and tenant as the target obligation,
  EUR on both source and obligation, and an amount no greater than the fresh
  obligation balance. Every allocation kind consumes the source transaction's
  remaining amount budget.
- `payment_allocations.rent_obligation_id` is nullable for deposit/other rows;
  migrated rows default to `rent` and retain their original bank transaction.
- `rent_obligations.record_status = voided` excludes the obligation from active
  dashboard totals while preserving the row and void metadata for history.
- Monetary values are integer cents. No EUR/non-EUR conversion or mixed-currency
  aggregate is permitted in phase 1.

### 4. Validation & Error Matrix

- Empty, non-EUR, or conflicting currencies -> reject the write with an
  EUR-only/currency-mismatch error; do not coerce or silently convert.
- Zero/negative amount, invalid source budget, or source over-allocation ->
  reject the entire operation.
- Cross-user or cross-tenant allocation -> reject without revealing or
  changing another user's rows.
- Rent amount greater than the freshly loaded obligation balance or allocation
  against a voided obligation -> reject and leave prior allocations intact.
- Duplicate operation retry -> use the idempotency key/operation boundary and
  do not add another effective allocation.

### 5. Good/Base/Bad Cases

- Good: Load current source and obligation balances inside one write
  transaction, validate EUR and ownership, insert an auditable allocation, and
  recompute the obligation projection from effective children.
- Base: Pre-migration confirmed allocations have an empty kind; migration
  defaults them to rent and projection still counts them exactly once.
- Bad: Add a second allocation without checking source balance, sum deposits in
  rent paid totals, overwrite a confirmed row to correct it, or convert GBP to
  EUR for display/aggregation.

### 6. Tests Required

- Unit tests for EUR normalization, non-EUR rejection, ownership and both
  source/obligation budget limits.
- Projection tests for legacy, confirmed, deposit/other, and voided rows plus
  Dublin due-day/next-day status behavior.
- Migration contract test covering nullable non-rent linkage, audit fields,
  dropped legacy uniqueness, and record status.
- Database tests should execute migration `003` against MySQL and verify
  duplicate/idempotent retries and concurrent over-allocation protection before
  production rollout.

### 7. Wrong vs Correct

Wrong:

```go
// A payer match alone changes paid rent and ignores currency and remaining balance.
obligation.PaidAmountCents += transaction.AmountCents
```

Correct:

```go
// Validate a fresh EUR allocation, then derive the projection from effective rows.
if err := validateLedgerAllocation(check); err != nil {
    return err
}
projected := projectLedgerObligation(obligation, allocations, now)
```

## Scenario: Bank Receipt Allocation, Correction, and Coverage

### 1. Scope / Trigger

- Trigger: Any authenticated bank transaction query, matching, allocation,
  ignore/restore action, revoke, or TrueLayer synchronization coverage view.
- Applies to `payment_transactions`, `payment_allocations`,
  `payment_transaction_actions`, `bank_sync_runs`, and
  `bank_sync_run_accounts`.

### 2. Signatures

- `transactionService.allocateTransaction(ctx, userID, transactionID, drafts, idempotencyKey, confirmationSource)` is the single write entry point for rent, deposit, and other-income allocations.
- `transactionService.ignoreTransaction`, `restoreTransaction`, and
  `revokeTransactionAllocations` own correction actions and audit rows.
- `transactionService.listTransactionsPage` and
  `listTransactionPageRowsWithTotal` own filtered, paginated transaction reads.
- `bankSyncStore.startRun` and `finishRun` persist requested and actual
  account-level coverage; `latestBankSyncCoverage` is the authenticated page
  read model.

### 3. Contracts

- `payment_transactions` preserves provider identifiers, arrival timestamp,
  description, reference, payer fields, parsed period facts, and raw payload;
  allocation state never overwrites this source record.
- Every query and write is scoped by `user_id`; posted transaction, tenant,
  obligation, and allocation IDs are reloaded with ownership predicates.
- Effective allocations are confirmed rows with `rent`, `deposit`, or
  `other_income`; legacy empty allocation kind is interpreted as rent.
- One source transaction has one shared integer-cent budget. A request first
  validates all drafts, then inserts them in one database transaction and
  recomputes affected rent obligations from effective allocations.
- Rent allocations require the same tenant as the obligation and an open
  period; deposit and other-income allocations do not update rent paid totals.
  Other income requires a non-empty note. Only EUR can become effective.
- Ignore, restore, and revoke are separate append-only action records with a
  required reason, operation ID, actor, and optional idempotency key. Revoke
  voids current effective allocations but never deletes the source or audit.
- Automatic matching reads active `tenant_payers`, requires an explicit parsed
  rent period and balance/currency evidence, and never auto-selects a shared
  payer. Arrival month is a query field, not a rent-period fallback.
- Sync coverage is authoritative only from persisted run/account rows. A
  partial or failed account result must not be shown as a complete yearly sync.

### 4. Validation & Error Matrix

- Missing or invalid user/transaction ownership -> no row or a safe domain
  failure; never query by a posted ID alone.
- Income allocation with non-positive amount, non-EUR currency, cross-tenant
  drafts, over-budget source, closed/voided obligation, or overpaid obligation
  -> reject the entire request and leave no new allocation rows.
- Allocation from an ignored transaction -> reject until it is restored.
- Ignore/restore/revoke without a trimmed reason -> reject; ignore/restore
  with effective allocations -> require the revoke flow first.
- Repeated allocation or action idempotency key -> return the original logical
  result without creating a second effect; reuse for different facts -> reject.
- Invalid filter date, month, status, direction, sort, tenant, or pagination
  input -> render the safe invalid-filter state; sort values are allowlisted.
- Non-EUR or ambiguous/shared-payer matches remain pending with a safe reason;
  they are never silently converted, assigned to arrival month, or batch-applied.

### 5. Good/Base/Bad Cases

- Good: lock the source and relevant obligations in one transaction, validate a
  €2,000 same-tenant split, insert all effective rows, then project both ledger
  and transaction state.
- Base: a €1,000 effective allocation leaves the source `partial` with a
  remainder and later allocation consumes only that remainder.
- Bad: trusting a hidden tenant/obligation ID, adding to `paid_amount_cents`
  directly, treating a voided allocation as effective, or using arrival month
  to infer an automatic rent month.

### 6. Tests Required

- Migration runner idempotence and presence of sync coverage, parsed-period,
  allocation-note, and action-audit structures.
- Allocation unit/integration tests for mixed uses, same-tenant split,
  remainder continuation, EUR-only, over-budget rollback, idempotency, and
  obligation projection.
- Correction tests for reason validation, ignored/restored transitions,
  revoke audit, affected-obligation recomputation, and old-revoke/new-match
  isolation.
- Matcher tests for payer ID/name precedence, shared payer conflicts, explicit
  month requirement, `JULY26`, overpayment, and non-EUR handling.
- Query/template tests for all filters, allowlisted sorting, pagination,
  internal/provider IDs, coverage labels, revoke preview, historical
  one-by-one preview, and unauthenticated mutation routes.
- Run `go test ./... -count=1`, `go vet ./...`, and `git diff --check`; use a
  disposable MySQL DSN for locking and migration integration tests.

### 7. Wrong vs Correct

Wrong:

```go
db.Model(&rentObligation{}).Where("id = ?", postedID).
    Update("paid_amount_cents", gorm.Expr("paid_amount_cents + ?", amount))
```

Correct:

```go
service.allocateTransaction(ctx, userID, transactionID, drafts, requestKey, "manual")
```

The service reloads every fact with `user_id`, locks the source and
obligations, validates the complete batch, inserts effective allocations
atomically, and recomputes the ledger projection from those rows.
