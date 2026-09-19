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

## Scenario: Scoped Landlord Rent Repository

### 1. Scope / Trigger

- Trigger: Any landlord rent feature that reads or writes properties, rooms,
  tenancy agreements, agreement parties, rent charges, rent obligations,
  payment allocations, or property/room expenses.
- Applies to `landlordRentRepository` in
  `cmd/truelayer-demo/landlord_rent_repository.go` and its future callers.

### 2. Signatures

- `newLandlordRentRepository(db *gorm.DB) *landlordRentRepository` creates the
  account-scoped persistence boundary.
- `find*` and `list*` methods always take `ctx` and `userID`; list methods take
  typed filters such as `rentChargeQuery`, `rentObligationQuery`,
  `paymentAllocationQuery`, or `manualExpenseQuery`.
- `createProperty`, `createRoom`, `createTenancyAgreement`,
  `createAgreementParty`, `createRentCharge`, `createRentObligation`, and
  `createManualExpense` always take `ctx`, `userID`, and a typed model row.

### 3. Contracts

- `userID` is the only trusted ownership input. Create methods overwrite the
  model row's `UserID` with the explicit argument; callers cannot choose an
  account through a posted/model field.
- Single-row cross-user lookups return `gorm.ErrRecordNotFound`; list methods
  return a non-nil empty slice. `userID == 0` fails before any database call.
- Relationship creation verifies every referenced object with the same
  `userID`. A room must belong to the property, an agreement to the room, a
  party to both the agreement and tenant, and a charge to the matching
  property/room/agreement chain.
- Rent-charge and rent-obligation month filters normalize through
  `monthStart`; expense `ToDate` is an exclusive upper bound. Active rent
  facts and expenses exclude non-active records unless `IncludeVoided` is set.
- Payment-allocation writes remain owned by the transaction service because
  locking, idempotency, and projection recomputation must stay in one business
  transaction; the repository owns their user-scoped reads.

### 4. Validation & Error Matrix

- `userID == 0` -> return `errLandlordRentUserRequired`; do not dereference or
  query the database.
- Cross-user property, room, agreement, tenant, charge, obligation, or expense
  target -> return `gorm.ErrRecordNotFound` for a single target and write no
  relationship row.
- Same-user but mismatched property/room/agreement relationship -> return
  `gorm.ErrRecordNotFound`; do not trust individual foreign-key IDs.
- Unknown list filter target or missing relation -> return an empty list rather
  than falling back to room labels, addresses, or other text fields.

### 5. Good/Base/Bad Cases

- Good: `repo.findRentObligation(ctx, userID, obligationID)` includes the
  account predicate, and charge-scoped obligation queries join
  `rent_charges` with both ID and user ownership conditions.
- Base: A user with no room expenses receives `[]manualExpense{}` and can
  distinguish that from a database error.
- Bad: `db.First(&property, postedID)`, copying `row.UserID` from request data,
  or joining `rent_charges` on ID alone and relying on a single-column foreign
  key to enforce account ownership.

### 6. Tests Required

- Opt-in MySQL tests run the real migration runner, create two users with
  same-named facts, and assert cross-user reads are invisible.
- Assert cross-user relationship writes fail without creating rooms,
  agreements, parties, charges, obligations, or expenses.
- Assert month, property, room, tenant, status, and date filters exclude
  unrelated rows and preserve non-nil empty-list semantics.
- Assert missing user IDs fail before database access, and run `go vet ./...`
  plus the full backend test suite after repository changes.

### 7. Wrong vs Correct

Wrong:

```go
db.First(&charge, postedChargeID)
```

Correct:

```go
charge, err := repo.findRentCharge(ctx, sessionUserID, postedChargeID)
```

The repository makes ownership part of the query contract instead of leaving
each handler or service to remember the predicate independently.

## Scenario: Legacy JSON Storage

## Scenario: Structured Tenant Room Binding

### 1. Scope / Trigger

- Trigger: The authenticated tenant form assigns, moves, or removes a tenant
  from a structured room arrangement.
- Applies to `tenantInputFromForm`, `tenantService.updateTenant`, and
  `syncTenantRoomBindingInTx` in `cmd/truelayer-demo/tenants.go` and
  `cmd/truelayer-demo/landlord_domain_operations.go`.

### 2. Signatures

- `tenantInputFromForm(values formValues) (tenantInput, error)` parses the
  submitted `room_id`; a non-zero ID marks the input as structured.
- `(*tenantService).updateTenant(ctx, userID, tenantID, input)` persists the
  tenant profile and requested room membership in one GORM transaction.
- `syncTenantRoomBindingInTx(tx, userID, tenantID, roomID, effectiveMonth,
  monthlyRentCents, currency, dueDay, tenantStatus) error` versions source and
  destination room arrangements for the current month.
- `saveRentArrangementInTx(tx, userID, input)` remains the only writer for
  versioned room agreements and agreement parties.

### 3. Contracts

- Every tenant, room, agreement, and party query uses the session `userID`.
- A structured edit applies room membership from `monthStart(time.Now().UTC())`;
  the historical tenant `rent_start_date` must not backdate an edit.
- Selecting no room removes the tenant from active current-month room
  arrangements. Setting the tenant inactive also removes the current binding.
- Moving rooms updates the old room's party set and destination room's party
  set inside the same transaction as the profile update. Other source-room
  tenants remain assigned, and the destination's existing currency and due
  date are inherited.
- `listTenants` derives `tenantRecord.RoomID` from the active current-month
  agreement so `/tenants?edit={id}` displays the saved selection.
- All agreement writes pass through `saveRentArrangementInTx`; charges for the
  current or a later month lock the arrangement, and the entire profile/binding
  transaction rolls back on that error.

### 4. Validation & Error Matrix

- Missing `userID` or `tenantID` -> fail before changing profile or relationship
  rows.
- Posted room outside the current user's account -> `gorm.ErrRecordNotFound`
  from the scoped arrangement write; make no partial update.
- Inactive tenant with a posted room -> save the profile as inactive and clear
  its current room membership.
- Current/future `rent_charges` on any affected room -> return
  `errArrangementHistoryLocked`; redirect the form to
  `/tenants?error=tenant_room_locked` and roll back all changes.
- A destination room already occupied by other tenants -> include the tenant
  in its next arrangement while retaining the existing room total unless the
  form supplies a changed total.

### 5. Good/Base/Bad Cases

- Good: Update the tenant profile and source/destination party sets in one
  transaction and let `saveRentArrangementInTx` enforce history locks.
- Base: An unbound structured tenant with no room selected remains unbound.
- Bad: Save the profile and ignore `room_id`, or update
  `agreement_parties` directly after a rent charge has been generated.

### 6. Tests Required

- Opt-in MySQL test runs the real migrations, assigns an unbound tenant,
  verifies the edit form's room ID, moves the tenant while retaining its prior
  room member, and clears the binding.
- Insert a current-month rent charge and assert a move returns
  `errArrangementHistoryLocked`, leaves the original membership intact, and
  does not partially update tenant profile fields.

### 7. Wrong vs Correct

Wrong:

```go
tx.Model(&tenant{}).Where("id = ?", tenantID).Updates(profileFields)
// The posted room_id is silently ignored.
```

Correct:

```go
return db.Transaction(func(tx *gorm.DB) error {
	if err := syncTenantRoomBindingInTx(tx, userID, tenantID, input.RoomID, monthStart(now), rent, currency, dueDay, input.Status); err != nil {
		return err
	}
	return tx.Model(&tenantRow).Updates(profileFields).Error
})
```

The tenant profile and versioned relationship either both commit or both roll
back.

---

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

## Scenario: Dunning Mail Delivery and Attempt Audit

### 1. Scope / Trigger

- Trigger: Any monthly rent reminder or overdue email initiated from the
  authenticated Dashboard.
- The Dashboard may select only the currently rendered page of the selected
  month; delivery is always one tenant per message.
- `migrations/008_dunning_mail.sql` is the schema source for sender settings
  and immutable delivery snapshots.

### 2. Signatures

- `dunningService.listCandidates(ctx, userID, periodMonth, filters)` returns
  account-scoped candidates for the current Dashboard page.
- `dunningService.preview(ctx, userID, periodMonth, obligationIDs, now)` is a
  read-only operation that returns fixed-template messages.
- `dunningService.send(ctx, userID, periodMonth, obligationIDs, requestKey,
  forceResend, retryOfAttemptID, serviceFrom, delivery, now)` owns delivery
  state, idempotency, retry linkage, and audit snapshots.
- `POST /dunning/config`, `/dunning/preview`, and `/dunning/send` accept the
  signed v2 session user and preserve the Dashboard month/filter/page context.

### 3. Contracts

- `dunning_sender_configs` is unique per `user_id`; it stores the landlord
  display name and Reply-To address, never SMTP passwords.
- `dunning_send_attempts` stores `user_id`, tenant/obligation/month ownership,
  expected/paid/balance cents, currency, recipient, subject/body, sender
  snapshots, status, request key, operation ID, and optional retry parent.
- `(user_id, request_key, rent_obligation_id)` is unique. A repeated request
  returns the existing attempt and must not call the mail delivery twice.
- The account-scoped service re-reads effective bank allocations and confirmed
  cash receipts immediately before sending. A paid or invalid recipient is
  audited as skipped/failed without external delivery.
- SMTP service-from is explicit `DUNNING_SMTP_FROM`; landlord Reply-To is a
  separate persisted value. No Bcc/Cc recipient list is generated.
- Templates are fixed English reminder/overdue messages. The overdue day
  count and same-day resend guard use the Europe/Dublin calendar date.

### 4. Validation & Error Matrix

- Missing or invalid SMTP host, port, or service-from -> reject before any
  network call; preview remains read-only.
- Missing sender configuration -> preview explains the configuration gap and
  send does not create an external delivery.
- Empty selection, more than one Dashboard page, or an obligation outside the
  current page -> reject with an authorization-safe error.
- A successful send on the same Dublin calendar date -> skip by default;
  resend requires an explicit confirmation flag.
- Retry IDs must belong to the same user/obligation and have `failed` status;
  retry creates a new attempt linked by `retry_of_attempt_id`.
- Cross-user sender, tenant, obligation, or attempt lookup -> no data or
  delivery; posted IDs are never trusted without ownership predicates.

### 5. Good/Base/Bad Cases

- Good: Render candidates from the current projected Dashboard rows, then
  re-read the selected obligation before snapshotting and delivering one
  RFC822 message addressed only to that tenant.
- Base: Preview creates zero attempt rows; one send creates one accepted then
  sent/failed snapshot, and a same-key retry creates no second delivery.
- Bad: Sending from the landlord Reply-To address, putting multiple tenants in
  To/Cc/Bcc, trusting stale paid totals, or recording only aggregate batch
  status without per-recipient snapshots.

### 6. Tests Required

- Template and parser tests for sender fields, current-page context, preview,
  same-day confirmation, failed retry, and accessible drawer controls.
- MySQL tests run migrations twice, verify candidate and attempt ownership,
  preview no-write behavior, per-recipient headers, request idempotency,
  concurrent duplicate clicks, failed retry linkage, paid re-read, and
  cross-user isolation.
- SMTP unit tests must prove missing configuration fails before network access
  and RFC822 output has exactly one To plus a separate Reply-To.

### 7. Wrong vs Correct

Wrong:

```go
smtp.SendMail(host, auth, landlordReplyTo, allTenantEmails, body)
```

Correct:

```go
attempt, existing, err := service.reserveDunningAttempt(ctx, snapshot)
if err != nil || existing {
    return attempt, err
}
err = delivery.Send(ctx, dunningEmail{
    FromEmail: serviceFrom,
    To:        snapshot.RecipientEmail,
    ReplyTo:   snapshot.ReplyToEmail,
    Subject:   snapshot.Subject,
    Body:      snapshot.Body,
})
```

## Scenario: Cash Rent Receipt Ledger and Projection

### 1. Scope / Trigger

- Trigger: Recording, previewing, voiding, correcting, or displaying a manual
  cash rent receipt alongside bank rent for one tenant/month.
- Applies to `cash_receipts`, `cashReceiptService`, the cash receipt HTTP
  handlers, and all rent history/dashboard payment projections.

### 2. Signatures

- `cashReceiptService.previewCashReceipt(ctx, input)` is read-only and returns
  expected, current paid, current remaining, after-paid, and after-remaining
  cents.
- `cashReceiptService.recordCashReceipt(ctx, input)` locks one
  `rent_obligations` row, validates the live balance, inserts one
  `cash_receipts` row, and updates the obligation projection atomically.
- `cashReceiptService.voidCashReceipt(ctx, userID, receiptID, reason)` locks the
  obligation before the receipt row, marks the receipt voided, records the
  actor/time/reason and `void_operation_id`, and recomputes only that bill.
- HTTP contracts: `GET /cash-receipts/new`, `POST /cash-receipts/preview`,
  `POST /cash-receipts`, `GET|POST /cash-receipts/void`.

### 3. Contracts

- `cash_receipts` is a separate ledger table. It must not create a
  `payment_transactions`, `payment_allocations`, or `tenant_payers` row.
- Required receipt fields are user, tenant, obligation, positive integer
  `amount_cents`, `currency=EUR`, `received_at`, `receipt_number`,
  `operation_id`, required user-scoped `idempotency_key`, recorder, and audit
  timestamps. A void stores `void_operation_id`, `voided_at`,
  `voided_by_user_id`, and `void_reason` while preserving `operation_id`.
- `(user_id, receipt_number)` and `(user_id, idempotency_key)` are unique.
- `rent_obligations.paid_amount_cents` is always recomputed from effective bank
  rent allocations plus confirmed cash receipts; deposit and other-income
  allocations do not contribute.
- Cash history rows use source `cash`, `received_at` as the display date, and
  `receipt_number` as the reference. Bank history keeps its original source
  fields.

### 4. Validation & Error Matrix

- Missing/zero user, tenant, obligation, amount, date, or idempotency key ->
  reject before database write.
- Currency other than EUR, tenant/obligation mismatch, voided obligation, or
  amount above the live remaining balance -> reject and leave all projections
  unchanged.
- Existing idempotency key with identical facts -> return the existing receipt;
  with different tenant, obligation, amount, date, currency, or note -> reject.
- Cross-user receipt, tenant, or obligation lookup -> return no row or a safe
  domain error; never reveal another account's data.
- Repeated void of an already voided receipt -> no-op; original receipt and
  audit fields remain queryable.
- GET form and POST preview -> no payment/receipt/projection write. POST
  confirmation must resolve the obligation again and revalidate all fields.

### 5. Good/Base/Bad Cases

- Good: A EUR 400 cash receipt on a EUR 1,000 bill projects alongside an
  existing EUR 600 confirmed bank rent allocation and produces a cash history
  row without changing bank transaction counts.
- Base: Voiding the EUR 400 receipt preserves the original operation ID,
  stores a distinct void operation ID and reason, and allows a new EUR 300
  receipt with a new idempotency key.
- Bad: Creating a synthetic bank transaction for cash, trusting a posted
  obligation ID without a user predicate, or recomputing paid only from bank
  allocations after a cash receipt exists.

### 6. Tests Required

- Unit tests assert positive integer cents, EUR-only validation, effective
  confirmed/voided projection, cash source/date/reference mapping, and template
  auth gates.
- Opt-in MySQL tests assert migration idempotence, preview no-write behavior,
  user isolation, duplicate idempotency, bank-plus-cash projection, void and
  correction audit, history/dashboard source rows, and no synthetic bank row.
- Opt-in MySQL concurrency test uses two receipts competing for one balance
  and asserts exactly one succeeds and the cached paid amount remains within
  the obligation amount.

### 7. Wrong vs Correct

Wrong:

```go
// Cash has no bank source, but this pollutes bank counts and payer matching.
paymentTransaction{UserID: userID, AmountCents: cashCents}
```

Correct:

```go
// Keep cash independent, then project both effective rent sources.
cashReceipt{UserID: userID, RentObligationID: obligationID, AmountCents: cents}
projectRentObligation(obligation, bankAllocations, cashReceipts, now)
```

## Scenario: Manual Bank Transaction Matching and Rent-Month Correction

### 1. Scope / Trigger

- Trigger: Rendering bank-transaction match suggestions, explicitly matching a
  receipt to rent, or correcting the tenant/month of an existing rent match.
- Applies to `/billing`, `/billing/confirm`, `/billing/rematch`, and the
  transaction/allocation ledger. Dashboard rendering and legacy import are
  specifically read-only with respect to transaction matching.

### 2. Signatures

- `POST /billing/confirm` accepts a `transaction_id` plus either a user-scoped
  `rent_obligation_id`, or the compatible `tenant_id` and `period=YYYY-MM`
  pair.
- `POST /billing/rematch` accepts only `transaction_id`, a target `tenant_id`,
  and `period=YYYY-MM`; the server resolves the scoped rent obligation.
- `transactionService.confirmRentMatch(ctx, userID, transactionID, tenantID,
  period, rememberPayer)` and `confirmRentMatchToObligation` own initial
  matching.
- `transactionService.rematchRentAllocation(ctx, userID, transactionID,
  targetTenantID, targetPeriod)` owns the limited direct correction path.

### 3. Contracts

- Billing may calculate a remembered-payer/parsed-month suggestion in memory,
  but listing `/billing`, rendering `/rent-dashboard`, and importing legacy
  data must not change a transaction's match projection, parsed-period facts,
  or allocations.
- A rent allocation is created only by an explicit POST. The target is reloaded
  with `user_id`, and the existing allocation service validates income, EUR,
  tenant ownership, and the current obligation balance under locks. Remembering
  the payer happens only after a successful allocation.
- A direct rematch may move only a transaction with exactly one effective
  `rent` allocation. It voids the old allocation, records the revoke audit
  action, creates an equivalent allocation for the chosen obligation, and
  projects both obligations in one database transaction.
- The rematch UI exposes tenant and rent-month choices independently; it does
  not materialize a tenant × month Cartesian-product selector. The submitted
  pair is resolved and validated server-side.
- Old and new obligation rows are locked in ascending ID order before a
  rematch. This prevents two inverse corrections from deadlocking. Original
  bank source facts (provider/stable IDs, date, payer, description, reference,
  amount, currency, and raw payload) are never edited by matching.
- Split or mixed-purpose transactions use revoke then reclassify; their
  allocations are never silently merged or rewritten by direct rematch.

### 4. Validation & Error Matrix

- Missing, invalid, or cross-user transaction/obligation -> safe confirmation
  error with no write.
- Ambiguous remembered payer, absent period, paid target, over-capacity target,
  or currency mismatch -> show/select another manual target; never auto-match.
- A rematch to the same obligation, or any source with zero, multiple, or
  non-rent effective allocations -> reject without voiding the old allocation.
- A failure after the old allocation is voided (including target validation or
  insert failure) -> roll back the void, action row, source projection, and
  affected obligation projections together.

### 5. Good/Base/Bad Cases

- Good: a visible `一键匹配` suggestion posts a transaction and obligation; the
  server validates the fresh target before allocating it.
- Base: a renter changes one matched receipt from September to October; the
  September allocation remains as `voided` with a `修改匹配` audit reason, and
  an equivalent confirmed allocation appears in October.
- Bad: a GET/import writes a match decision, an action trusts an unscoped hidden
  ID, or a correction overwrites an existing allocation row or bank receipt.

### 6. Tests Required

- Template tests assert `一键匹配`, independent tenant/month-only `修改匹配`
  selectors, and revoke/reclassify guidance for split rows.
- Service/database tests assert explicit target validation, no allocation from
  billing reads after a revoke, void-plus-replacement audit history, both
  obligation projections, cross-user rejection, and split/mixed rejection.
- Concurrency tests or code review must verify deterministic old/new obligation
  lock ordering; run the MySQL suite with a disposable test DSN when available.

### 7. Wrong vs Correct

Wrong:

```go
// Rewriting history loses the old match and can leave projections inconsistent.
db.Model(&paymentAllocation{}).Where("id = ?", allocationID).
    Updates(map[string]any{"tenant_id": tenantID, "rent_obligation_id": obligationID})
```

Correct:

```go
// Preserve the old row as voided, then create an equivalent replacement atomically.
err := service.rematchRentAllocation(ctx, userID, transactionID, targetTenantID, targetPeriod)
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

## Scenario: Dashboard Manual Balance Income

### 1. Scope / Trigger

- Trigger: A landlord settles the remaining balance of one Dashboard rent
  obligation without importing a bank transaction.
- Applies to `POST /rent-dashboard/settle`,
  `transactionService.settleRentObligation`, `payment_transactions`, and the
  confirmed rent allocation/projection path.

### 2. Signatures

- `POST /rent-dashboard/settle` accepts an `obligation_id` transport field plus
  Dashboard return filters; it never accepts an amount, tenant, or currency.
- `transactionService.settleRentObligation(ctx, userID, obligationID)` creates
  the manual transaction and allocation within one database transaction.
- `manualBalanceTransactionSource = "manual_balance"` identifies these
  synthetic records; their visible description is `手动平账`.

### 3. Contracts

- Lock the user-scoped obligation first, recompute its paid amount from
  effective allocations and confirmed cash receipts, and use only that fresh
  remaining amount for the income transaction.
- The created transaction must be income, EUR, use the selected obligation
  month as `parsed_period_month`, and be allocated as confirmed `rent` to the
  same tenant and obligation before the transaction commits.
- The source transaction, allocation, and obligation projection are atomic. A
  concurrent retry waits for the obligation lock, sees no remaining balance,
  and creates no new record.
- `manual_balance` is excluded from background reconciliation. If a user
  revokes its allocation through the audit path, it must remain revoked rather
  than being automatically reallocated.

### 4. Validation & Error Matrix

- Missing, invalid, cross-user, or voided obligation -> no new transaction;
  the Dashboard returns a generic safe error state.
- Already-paid obligation or a stale/double-click request -> no new
  transaction and an informational “no balance remains” Dashboard state.
- Non-EUR currency, corrupt overpaid projection, or allocation validation
  failure -> roll back the entire operation.
- A browser POST must be protected by the normal authenticated session and
  must not trust hidden return fields for financial facts.

### 5. Good/Base/Bad Cases

- Good: a €1,000 bill with €400 confirmed creates one €600 `手动平账` income
  transaction and one €600 rent allocation, then projects the bill as paid.
- Base: no remaining balance produces no manual transaction.
- Bad: post a client-computed amount, write a transaction before allocation in
  another transaction, or let reconciliation recreate a revoked manual entry.

### 6. Tests Required

- Template test: the action is shown only when a Dashboard row has a positive
  balance and has a native confirmation prompt.
- Database test: assert exact remaining cents, transaction source/description,
  allocation target, paid projection, cross-user isolation, repeat/concurrent
  request behavior, and no reallocation after a revoke.
- Handler test: only authenticated POSTs reach the service and return filters
  are preserved on redirect.

### 7. Wrong vs Correct

Wrong:

```go
amountCents := moneyToCents(parsePostedAmount(r.Form.Get("amount")))
db.Create(&paymentTransaction{AmountCents: amountCents})
```

Correct:

```go
// The locked server-side projection is the only source of the amount.
created, err := service.settleRentObligation(ctx, userID, obligationID)
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
- Candidate matching reads active `tenant_payers`, parsed rent-period facts,
  and balance/currency evidence only to render a suggestion. It never writes a
  match or allocation without an explicit POST; arrival month is a query field,
  not a rent-period fallback.
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

## Scenario: Monthly Rent Dashboard Read Model

### 1. Scope / Trigger

- Trigger: Any authenticated `/rent-dashboard` monthly summary, bill-list
  filter, pending-income entry point, other-income entry point, or sync-state
  display.
- Applies to the database-backed monthly obligations, effective rent/cash
  payment projection, bank arrival transactions, allocations, and sync runs.

### 2. Signatures

- `rentDashboardFiltersFromQuery(url.Values)` parses and validates typed search,
  status, sort, page, and page-size input.
- `filterAndSortRentDashboardRows` and `paginateRentDashboardRows` own the
  allowlisted in-memory list transformation; they never build SQL ordering.
- `obligationService.summarizeRentDashboardWithFilters(ctx, userID,
  periodMonth, filters)` returns full-month totals plus filtered/paged rows and
  bank/sync metrics.
- `summarizeRentDashboardBankMetrics` calculates pending remainder and
  effective other-income metrics from typed transaction/allocation rows.

### 3. Contracts

- Full-month expected, paid, balance, and status counts are calculated before
  search, status filtering, sorting, and pagination. A filtered page never
  changes the cards or their denominators.
- Dashboard rows are active, user-owned obligations for the requested
  `period_month`; rent payment details reuse the effective bank allocation and
  confirmed cash receipt projection.
- Search covers tenant name, display alias, room label, and room address.
  Status values and sort values are allowlisted; default page size is 12 and
  the maximum is 50. “Unpaid” includes open, overdue, and partial bills.
- Pending metrics use income `payment_transactions.transaction_time` for the
  selected bank-arrival month and only a transaction's effective allocation
  remainder. Other-income metrics use effective `other_income` allocations and
  count each source transaction once. EUR is the only aggregated currency;
  non-EUR rows are never silently converted.
- Sync text comes from the current user's latest `bank_sync_runs` and account
  coverage. Partial/failed latest runs remain visible, with the latest
  successful coverage shown separately when available; no-run state is explicit.
- All dashboard reads include `user_id`; URL links preserve the selected month,
  filters, direction, and allocation/pending conditions where applicable.

### 4. Validation & Error Matrix

- Invalid status, sort, page, page size, or overlong search -> render the safe
  invalid-filter state and use bounded default filters.
- Invalid period -> render the current-month fallback with an explicit invalid
  period notice; do not use an arbitrary persisted month.
- No active obligations -> render the “no monthly bills” empty state; zero
  filtered rows with obligations -> render a distinct “no matches” state.
- Cross-user tenant, obligation, transaction, allocation, cash receipt, or sync
  row -> exclude through ownership predicates; never trust URL IDs as access.
- Missing latest sync, partial/failed sync, and complete sync -> show distinct
  states; sync failure must not be presented as a clean zero-balance dashboard.

### 5. Good/Base/Bad Cases

- Good: load all monthly obligations, derive cards, then filter and paginate
  typed rows; aggregate pending remainder and other income by arrival month.
- Base: three €1,000 bills paid €1,000/€400/€0 yield €3,000 expected,
  €1,400 paid, €1,600 remaining and one tenant in each status class.
- Bad: summing only the current page, grouping a September arrival into its
  September rent obligation without an explicit allocation, counting a mixed
  transaction twice as bank income, or converting GBP into EUR.

### 6. Tests Required

- Filter unit tests for allowlists, alias/room search, unpaid semantics, stable
  sorting, pagination, URL preservation, and invalid input.
- Template tests for selected filters, card/status links, pending/other-income
  entry points, sync warning/no-sync states, distinct empty states, and paging.
- Database tests should cover the three-bill totals, cross-month arrival versus
  rent period, partial remainder, effective other income, latest sync fallback,
  and cross-user isolation with a disposable MySQL DSN.
- Run `go test ./... -count=1`, `go vet ./...`, and `git diff --check` before
  committing dashboard changes.

### 7. Wrong vs Correct

Wrong:

```go
// A filtered page accidentally becomes the monthly financial summary.
summary.ExpectedCents = sumRows(pageRows)
```

Correct:

```go
allRows := loadActiveMonthlyObligations(userID, periodMonth)
summary := summarizeFullMonth(allRows)
filtered := filterAndSortRentDashboardRows(allRows, filters)
summary.Rows, summary.TotalPages = paginateRentDashboardRows(filtered, filters.Page, filters.PageSize)
```

The monthly bill remains the source of rent-period truth; bank arrival month is
used only for pending and other-income navigation.
