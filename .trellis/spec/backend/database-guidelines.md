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
