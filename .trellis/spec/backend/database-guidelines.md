# Database Guidelines

> Database patterns and conventions for this project.

---

## Overview

The application uses MySQL-compatible storage through GORM. SQL files under
`migrations/` are the schema source of truth and are applied by the startup
migration runner; production code must not call GORM `AutoMigrate`.

The authenticated UI uses account-scoped database rows after a signed-in user
session is established. JSON tenant/expense ledgers and the legacy import
route are retired; they are not an authenticated UI fallback.

Some lower sections retain pre-migration-014 notes. For current landlord rent
behavior, follow “Room Rent Plans, Asset State, and Monthly Facts” below.

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
- `runMigrations(db, dir)`: applies ordered SQL migrations before app startup.

### 3. Contracts

- `users` is the real account source. Environment variables only seed the
  configured default user if its username is absent; they never overwrite an
  existing password.
- `tenants`, `rent_obligations`, `payment_transactions`,
  `payment_allocations`, `manual_expenses`, and `bank_connections` all carry
  `user_id` and must be filtered by it.
- `payment_transactions` uses `(user_id, stable_transaction_key)` for idempotent
  bank ingestion.
- Rent obligations are monthly facts generated from room rent plans; tenant
  profiles do not carry rent amounts, due days, or rent validity dates.
- Payment identity/matching data belongs to `tenant_payers`, not duplicated
  fields on tenant profiles.

### 4. Validation & Error Matrix

- Missing database configuration -> startup fails; do not silently switch the
  primary UI back to JSON.
- Unapplied migration failure -> startup fails.
- Live environment without token encryption key -> configuration fails.
- Cross-account tenant, transaction, allocation, expense, or token lookup ->
  returns no row or an authorization-safe error.
- Duplicate bank ingestion -> no duplicate transaction rows.

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
- Room rent-plan migration, validation, fact materialization, locking, and
  account ownership.

## Scenario: Migration Authoring with the Flat SQL Runner

### 1. Scope / Trigger

- Trigger: writing or editing any file under `migrations/`.
- `applyMigration` in `cmd/truelayer-demo/db.go` executes each migration inside
  one transaction, but the statement splitter is a plain text split.

### 2. Signatures

- `splitSQLStatements(sqlText string) []string` splits on `;` and drops every
  fragment whose trimmed text starts with `--`.
- `runMigrations(db *sql.DB, dir string)` orders files by filename, skips
  versions already recorded in `schema_migrations`, and records each applied
  version in the same transaction.

### 3. Contracts

- Migration files carry **no comments**. A `--` fragment is discarded silently,
  so a comment line above a statement deletes that statement with no error and
  no log line. A `;` inside a comment still splits the file, so a block comment
  containing one produces an unterminated `/*` and a syntax error.
- A single statement may not contain `;`, which rules out stored procedures,
  `SIGNAL` self-checks, and multi-statement bodies. Re-entrant helper state
  belongs in a plain table created with `CREATE TABLE` after a
  `DROP TABLE IF EXISTS`, and cleaned up at the end of the file.
- DDL implicitly commits, so a migration is not atomic once it alters a table.
  Put `ALTER TABLE` after the data statements the migration must not lose, and
  make every earlier step idempotent so a rerun after a failed `ALTER` succeeds.
- Uniqueness is declared in SQL, never by `AutoMigrate`. When a later migration
  drops a key an earlier one added, the Go code that relied on it must be
  updated in the same change or it silently degrades (`009` dropped
  `idx_rent_obligations_user_tenant_period`, and `ensureMonthlyObligations`
  stopped deduplicating because `OnConflict.Columns` is not read by the MySQL
  driver).
- A new migration file must never edit an earlier one; existing migration
  contract tests read those files verbatim.

### 4. Validation & Error Matrix

- Comment present in a migration file -> statement skipped or syntax error;
  reject in review.
- Data repair that adds a unique key -> the repair itself must run in the same
  migration, or `ADD UNIQUE KEY` fails on any database that already holds
  duplicates and the application cannot start.
- Repair that deletes rows referenced by cascading foreign keys -> remap the
  references onto the surviving row first; a raw delete destroys history.
- Statement relying on session variables -> valid, because one migration runs
  on one pinned connection.

### 5. Good/Base/Bad Cases

- Good: `migrations/013_dedupe_rent_obligations.sql` is comment-free, uses a
  plain `_mig013_keep` helper table, remaps every cascading reference, and
  leaves the `ALTER TABLE` that adds the constraint last.
- Base: a fresh database applies every file in order and a second run is a
  no-op through `schema_migrations`.
- Bad: `-- repair duplicates` above an `UPDATE`, a helper table created with
  `CREATE TEMPORARY TABLE` plus a `SIGNAL` guard, or an `ALTER TABLE` placed
  before the data repair.

### 6. Tests Required

- Static test asserting the migration text contains no `--`/`/*` comment and
  splits into non-empty statements through `splitSQLStatements`.
- Opt-in MySQL test that applies migrations twice, and a test that rebuilds the
  pre-migration state, seeds the dirty rows the migration repairs, applies it,
  and asserts both the repaired facts and the new constraint.
- Run `go test ./... -count=1`, `go vet ./...`, and `git diff --check`.

### 7. Wrong vs Correct

Wrong:

```sql
-- Repair duplicate lazy obligations before adding the key.
DELETE FROM rent_obligations WHERE id NOT IN (SELECT MIN(id) FROM rent_obligations GROUP BY user_id, tenant_id, period_month);
```

Correct:

```sql
DROP TABLE IF EXISTS _mig013_keep;
CREATE TABLE _mig013_keep (...);
INSERT INTO _mig013_keep (...) SELECT o.id, ... FROM rent_obligations o WHERE o.rent_charge_id IS NULL AND ...;
UPDATE payment_allocations pa JOIN _mig013_keep k ON ... SET pa.rent_obligation_id = k.obligation_id;
DELETE o FROM rent_obligations o JOIN _mig013_keep k ON ... WHERE o.id <> k.obligation_id;
ALTER TABLE rent_obligations ADD UNIQUE KEY ...;
```

## Scenario: Room Rent Plans, Asset State, and Monthly Facts

This post-migration-014 contract supersedes any other sections in this file that
describe `tenancy_agreements`, `agreement_parties`, structured tenant binding,
tenant rent fields, JSON rent fallbacks, or `POST /import-legacy`. Those
descriptions refer to retired behavior.

### 1. Scope / Trigger

- Trigger: changing property/room lifecycle, rent-plan timelines, monthly rent
  facts, or any payment/cash/dunning write that references those facts.
- Applies to `roomRentPlanService`, `monthlyRentFactsService`,
  `landlordRentRepository`, migration 014, and the room/rent-workspace handlers.

### 2. Signatures

- `SaveRoomRentPlanCommand` carries `UserID`, `RoomID`,
  `EffectiveMonth`, `MonthlyRentCents`, `Currency`, `DueDay`,
  `Members`, and `ExpectedTimelineVersion`.
- `(*roomRentPlanService).SaveRoomRentPlan(ctx, command)` replaces a room's
  rent-plan timeline from the selected month and returns the saved plan and new
  version.
- `EndRoomRentPlanCommand` carries `UserID`, `RoomID`,
  `VacantFromMonth`, and `ExpectedTimelineVersion`.
- `(*roomRentPlanService).EndRoomRentPlan(ctx, command)` ends occupancy from
  the selected rent-plan month.
- `(*monthlyRentFactsService).ensureMonthlyRentFacts(ctx, userID, periodMonth,
  intent)` is the only business entry point that generates obligations.
  `intent` is `rentFactsIntentRead` or
  `rentFactsIntentExplicitPayment`.
- UI writes normally use `POST /rooms/{id}/rent-plan`. Two creation-time
  composite paths may call that same plan service: creating a room creates its
  first empty-member plan, and creating a tenant may add the new tenant to a
  selected room plan. Neither path writes rent columns on `rooms` or `tenants`.

### 3. Contracts

- Every plan, member, charge, obligation, tenant, room, and property lookup is
  scoped by the authenticated `user_id`. Request model `UserID` is not trusted
  in place of the session identity.
- Properties and rooms are physical assets. Their `status` is their current
  administrative state; neither has an effective-from/effective-to period.
  Monthly rent and due day belong to a room rent plan, whose
  `effective_from_month` and inclusive `effective_to_month` define occupancy
  and responsibility over time.
- Current asset status does not rewrite the rent-plan timeline or historical
  facts. Rent fact selection follows the plan's month interval, not a property
  or room validity date.
- A saved plan requires positive EUR rent and a due day from 1 through 31.
  An empty member list is a valid vacant-room rent rule: it keeps the timeline
  and schedule but creates no charge or obligation. With members, a zero
  responsibility means “unspecified”: all-zero members split the total evenly;
  positive entries are fixed and the positive remainder is split evenly between
  the unspecified members; when every member is positive, their sum must equal
  the room total.
- Plan writes lock the room and compare `ExpectedTimelineVersion`. The
  replacement, unlocked-fact deletion, member writes, version increment, and
  applicable fact materialization run in one transaction. A current or past
  non-empty plan materializes its facts; an empty plan and a future plan do not.
- `createRoomWithRentPlan` creates the physical room and its zero-member plan
  in one transaction. `createTenantWithRoomPlan` creates the tenant, locks the
  selected room, re-reads its active plan and member IDs, then writes the
  complete new plan in the same outer transaction. The browser's property,
  rent, due-day, plan members, and timeline version are hints only; the service
  revalidates ownership and uses the stored plan schedule.
- A tenant may belong to at most one room in any rent month. `SaveRoomRentPlan`
  locks the active tenant rows before checking other-room plans from the
  effective month onward; an overlap returns `ErrTenantRoomMonthConflict`.
  Plans that ended before the target month are excluded, and the page maps the
  sentinel to a stable tenant-already-occupied message. MySQL coverage must
  include current, future, inclusive end-month, ended-before-target, and
  concurrent two-room assignments.
- A normal read materializes the requested current or historical month. A
  future read is preview-only. Explicit payment intent is the only path that may
  materialize a future month.
- Allocations, confirmed cash receipts, and dunning attempts lock a plan month.
  A plan edit from that month onward must fail without changing the timeline or
  deleting facts.
- Financial writers lock their source payment transaction first where present,
  then room rows by ascending ID, charge rows by ascending ID, and obligation
  rows by ascending ID. After waiting, they re-read and validate the
  room/property/charge/plan/member/tenant/month chain; a changed chain returns
  `ErrRentFactsConflict` so the caller can ask the user to retry.
- Migration 014 is an intentional destructive schema transition, not a data
  migration. Before applying it, all pre-014 business tables must be empty. The
  migration refuses non-empty databases and does not delete rows. There is no
  data-level rollback; code rollback requires restoring a pre-014 backup or
  rebuilding a pre-014 database.

### 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Missing user, room, effective month, rent, or due day | `ErrInvalidRentPlan`; no writes |
| Duplicate member, foreign-account tenant, fixed total that leaves no positive remainder, or final responsibility sum mismatch | Reject; no partial writes |
| Empty member plan | Persist only the vacant-room rent rule; do not create a charge or obligation |
| Timeline version differs from `rooms.rent_plan_version` | `ErrStaleRentPlanTimeline`; refresh before retry |
| Composite tenant write has a changed active member set or mismatched property | Reject as stale/invalid and roll back the new tenant and payer |
| Overlapping or duplicate plan interval | `ErrRentPlanTimelineConflict`; transaction rolls back |
| Allocation, cash receipt, or dunning history exists from edited month onward | `ErrRentPlanFactsLocked`; preserve facts and timeline |
| Financial fact chain changed while acquiring locks | `ErrRentFactsConflict`; roll back and offer retry |
| Future month requested by ordinary read | Return preview data; do not write facts |
| Migration 014 sees non-empty business tables | Refuse migration; require explicit development/test database rebuild |

### 5. Good/Base/Bad Cases

- Good: change monthly rent from the room's rent-plan page, or create a room
  or tenant through its composite command; each route delegates to the same
  room-plan service and atomically checks locks and timeline version.
- Base: deactivate a physical room without inventing an asset effective date;
  its rent-plan history remains tied to plan months.
- Bad: persist `active_from`, `inactive_from`, rent, or due-day fields on a
  property/room/tenant profile, or update tenant rent without replacing the
  room-plan timeline through the plan service.
- Bad: materialize a future obligation during a dashboard preview or delete a
  charge after an allocation/cash/dunning record has referenced it.

### 6. Tests Required

- Unit tests cover empty-plan preservation, member splitting (including partial
  fixed amounts), sum validation, month normalization, timeline replacement,
  version conflicts, lock detection, and future-read previews.
- MySQL integration tests use a disposable database to apply migrations from
  fresh state, reapply them, and prove migration 014 refuses non-empty
  pre-014 business tables without deleting their rows.
- MySQL service tests cover save/end transaction boundaries, allocation/cash/
  dunning locks, and concurrent writers returning a retryable conflict.
- Handler/template tests assert property and room forms contain no asset
  validity fields, room creation has required rent/month/due-day inputs, tenant
  creation (but not tenant edit) has the optional room-plan fields, `/tenancies`
  GET and POST are 404, and desktop/mobile workspace views preserve tenant/room
  filtering.
- Run focused, package, and repository checks plus MySQL integration tests when
  `RENTOPS_MYSQL_TEST_DSN` is configured. Do not mark the Trellis task complete
  until its desktop/mobile and migration gates have evidence.

### 7. Wrong vs Correct

Wrong:

```go
// Persist rent on the physical room instead of creating a room-plan rule.
roomInput.ActiveFrom = submittedMonth
roomInput.MonthlyRentCents = submittedRent
```

Correct:

```go
created, plan, err := newLandlordDomainService(db).createRoomWithRentPlan(ctx,
    userID, roomInput,
    roomRentPlanSetupInput{EffectiveMonth: month, MonthlyRentCents: rentCents,
        Currency: ledgerCurrencyEUR, DueDay: dueDay},
)
```

Keep asset identity/status in `properties` and `rooms`; keep occupancy,
responsibility amounts, and rent timing in the room rent-plan timeline.

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

> Historical pre-014 guidance below includes tenant rent dates and a separate
> billing-history projection. Current tenant profiles contain identity/payer
> details; rent responsibility comes from room rent plans above.

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

## Scenario: Monthly Rent Fact Materialization

> Historical pre-014 guidance below describes dual structured/legacy
> obligation generation. It is retired; use “Room Rent Plans, Asset State, and
> Monthly Facts” above for the current materialization contract.

### 1. Scope / Trigger

- Trigger: Any operation that needs persistent rent responsibilities for one
  account and month, including workspace/history reads, matching, cash receipt,
  explicit future-month allocation, and dunning.
- Applies to structured room arrangements and the controlled legacy tenant
  fallback; it does not replace rent obligations with view-only calculations.

### 2. Signatures

- `newMonthlyRentFactsService(db *gorm.DB) *monthlyRentFactsService` creates the
  shared materialization service.
- `(*monthlyRentFactsService).ensureMonthlyRentFacts(ctx, userID, periodMonth,
  intent) error` is the sole business entry point for generating monthly rent
  responsibilities. `intent` is `rentFactsIntentRead` or
  `rentFactsIntentExplicitPayment`.
- `rentLedgerService.ensureRentCharge(ctx, userID, propertyID, roomID,
  periodMonth)` creates or loads one room charge and its tenant obligations.
- `obligationService.ensureMonthlyObligations(ctx, userID, periodMonth)` is the
  legacy fallback and must not be called as an independent business entry
  point.

### 3. Contracts

- Normalize `periodMonth` with `monthStart`. All database reads and writes are
  scoped by the authenticated `userID`.
- Read intent materializes only the current or a historical month. A future
  month is preview-only; explicit payment intent may materialize that future
  month before allocation so allocations always reference stable obligation IDs.
- For each active structured room/month, create or load one `rent_charge` and
  its persisted `rent_obligations` from the effective agreement and parties.
  Room total and individual responsibilities are integer cents and party
  responsibilities must reconcile to the charge total.
- Empty rooms do not get a new charge. Existing charge rows remain targets so
  retries can load their persisted obligations.
- After structured rooms, run the legacy generator. A tenant with a structured
  party or charge-backed obligation for that month is skipped by legacy lazy
  generation. A legacy-only tenant continues to receive one lazy obligation.
- Existing legacy obligation versus new structured responsibility for the same
  tenant/month is a conflict. Never duplicate, delete, or silently reassign
  historical obligations, allocations, cash receipts, or dunning attempts.
- A future preview must not write a charge or obligation; all financial views
  consume persisted obligation facts rather than recomputing amounts from
  template display values.

### 4. Validation & Error Matrix

- `userID == 0`, zero month, nil service/database, or unsupported intent ->
  return a validation error before materialization.
- Read intent with a future month -> return success without writes.
- Explicit payment intent with a future month -> materialize before allocation.
- Active structured room with invalid relationship, multiple active
  agreements, invalid responsibility totals, or a legacy/structured month
  collision -> return an error without writing inconsistent facts for that
  room/tenant. Earlier room transactions in the same month may already have
  committed; retry remains idempotent.
- Database failure -> propagate the error; do not fall back to a second
  generator.

### 5. Good/Base/Bad Cases

- Good: workspace and payment paths call `ensureMonthlyRentFacts` with their
  intent, then read/allocate against the persisted obligation IDs it ensures.
- Base: an empty structured room creates no charge; a legacy tenant still gets
  one lazy obligation; a future workspace read creates neither.
- Bad: calling `ensureMonthlyObligations` directly from a second feature path,
  generating structured obligations from `tenants.monthly_rent_cents`, or
  inserting both legacy and charge-backed rows for one tenant/month.

### 6. Tests Required

- Unit tests for intent/month boundaries, including future preview and explicit
  future payment materialization.
- Database tests for structured charge/party generation, empty rooms, legacy
  fallback, conflict rejection, idempotent retries, and concurrent calls.
- Call-site tests must assert that matching, cash, workspace, and history use
  the shared service and retain stable obligation IDs.
- Run `go test ./... -count=1`, `go vet ./...`, and `git diff --check`; run
  MySQL-backed locking/migration tests when `RENTOPS_MYSQL_TEST_DSN` is set.

### 7. Wrong vs Correct

Wrong:

```go
newObligationService(db).ensureMonthlyObligations(ctx, userID, month)
```

Correct:

```go
newMonthlyRentFactsService(db).ensureMonthlyRentFacts(
	ctx, userID, month, rentFactsIntentRead,
)
```

The shared entry point decides between preview, structured facts, and the
guarded legacy fallback so consumers cannot independently double-generate.

## Scenario: Monthly Rent Dashboard Read Model

### 1. Scope / Trigger

- Trigger: The monthly rent/dunning read model and its bank-arrival metrics.
  `/rent-dashboard` is the canonical room-centric workspace; `/bills` GET is
  only a compatibility redirect to its tenant view.
- Applies to the database-backed monthly obligations, effective rent/cash
  payment projection, bank arrival transactions, allocations, and sync runs.

> **Surface note.** `GET /bills` redirects to
> `/rent-dashboard?view=tenants`; `/tenancies` GET and POST are unregistered and
> return 404.
> The old bill/dunning template helpers may remain for compatibility tests and
> server-side dunning reads, but they are not navigation destinations. Do not
> use them as the canonical rent UI; see the frontend
> "Rent Workspace Navigation and Legacy Routes" contract.

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
  **The former bill template is no longer a live page:**
  `renderRentDashboard` still fills `rentDashboardPageData.SyncCoverage` /
  `.SyncStatus` /
  `.LastSuccessfulSyncCoverage` (`dashboard.go`), but no template reads those
  three fields any more — the inline sync notice lived only in the deleted
  legacy template. `/bank` (`bankPageTemplate`) renders sync separately from its
  own `bankPageData.SyncCoverage` (`page_data_routes.go`). Restoring the
  dashboard sync notice is part of the dashboard-alignment subtask, not this
  read model.
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

The persisted monthly obligation remains the source of rent-period truth; bank
arrival month is used only for pending and other-income navigation.

## Scenario: Room-Centric Rent Workspace Read Model

> Historical pre-014 guidance below attributes rent through agreement parties
> and includes a legacy fallback. It is retired; use “Room Rent Plans, Asset
> State, and Monthly Facts” above for current room and tenant totals.

### 1. Scope / Trigger

- Trigger: Any authenticated `/rent-dashboard` request while `a.db != nil`
  (`handleRentDashboard`, `dashboard.go`) — that is, every database-backed
  session. A session without a database now returns an explicit `503`; the
  legacy no-database fallback was deleted
  (`09-19-legacy-dashboard-template-removal`).
- Governs the monthly summary cards, the per-property cards, the room tree and
  its per-room tenant counts, and the room status chips.

### 2. Signatures

- `app.renderRentWorkspaceDashboard(w, r)` — `rent_workspace_page.go:91`.
- `rentWorkspaceData` / `rentWorkspaceRoomAggregate` — the loader's typed input
  and the per-room aggregate.
- `rentWorkspaceService.load(ctx, userID, filters)` calls
  `monthlyRentFactsService.ensureMonthlyRentFacts` with read intent before its
  reads; see "Monthly Rent Fact Materialization" above.
- `roomIDByTenant` / `ambiguousTenant` / `ambiguousRoom`
  (`rent_workspace.go:399-417`) — the tenant-to-room attribution built from
  active agreement parties, plus the tenants that map to more than one room and
  the rooms they touch.
- `rentLedgerService.ensureRentCharge(ctx, userID, propertyID, roomID, period)`
  — `landlord_rent_ledger.go:244`; the shared materializer uses it for
  structured room facts.

### 3. Contracts

- **Room totals sum persisted obligations.** The workspace reads active,
  user-owned obligations for the requested `period_month`
  (`rent_workspace.go`) after the shared materializer has created structured
  room charges/obligations and legacy lazy obligations as appropriate. A
  structured tenant is never independently lazy-generated from
  `tenants.monthly_rent_cents`.
- **Room attribution is tenant → active party → agreement room.** A tenant is
  attributed to a room when an active `agreement_parties` row joins an active
  agreement whose dates cover the month (`rent_workspace.go:399-417`). A tenant
  that maps to two different rooms in the same month is ambiguous: it is counted
  in **neither** room (`rent_workspace.go:463`) and **both** rooms are flagged
  `needs_review` (`rent_workspace.go:544`), because such a room's total is
  knowingly short. The flag is load-bearing, not decorative: without it a room
  shared by an ambiguous tenant and a settled one would show a quietly small
  number with a normal status, and only a room whose sole occupant is ambiguous
  would happen to look wrong. A tenant with no mapping is not counted at all.
  Room money is the sum of its tenants' `expected_amount_cents`, never room rent
  × headcount.
- Occupancy remains the independent `activePartyCountByRoom` computation, so
  tenant count and money share no source. Structured room obligations carry a
  `rent_charge_id`; legacy fallback obligations do not.
- A room with an active agreement and active party but **no attributable
  obligation** (including a row skipped for a non-EUR currency) is reported as
  `needs_review` (`rent_workspace.go:537-542`), never as `vacant` and never as
  settled. The room-level due date is the earliest attributed obligation's due
  date (`rent_workspace.go:533`).
- `rent_charges` is a production fact written by
  `rentLedgerService.ensureRentCharge`; it is not a user-maintained document.

### 4. Validation & Error Matrix

- Room with attributed EUR obligations -> expected/paid/balance are the sums of
  those obligations; the room is never `vacant`.
- Room touched by an ambiguous tenant -> `needs_review` even when its other
  tenants' obligations were counted. Its total is knowingly short of the truth,
  and the status is the only thing that says so.
- Room with an active agreement and active party but no attributable obligation
  -> `needs_review` with zero amounts; never render it as vacant or settled.
- No agreement and no party -> `vacant`; amounts render as `—`, not `0.00`.
- Non-EUR obligation currency -> skip that row and mark the room
  `needs_review`; never convert into EUR silently.
- A tenant attributed to two rooms in one month -> excluded from both, so the
  read is never double counted; both rooms are flagged `needs_review` so the
  shortfall is visible rather than silently absent.
- `userID == 0` -> the service refuses before any read or write.

### 5. Good/Base/Bad Cases

- Good: let the loader call the shared materializer, attribute each persisted
  obligation to its tenant's room through the active party relationship, and
  sum the obligations per room.
- Base: a shared room with three tenants each on €500 reports €1,500 expected,
  not the room rent multiplied by three.
- Bad: reading an all-zero room total as "no rent is due this month", or
  calling the legacy generator directly for a structured tenant.

### 6. Tests Required

- Database tests must assert that structured room totals come from
  charge-backed responsibilities and legacy-only totals from lazy
  responsibilities, and that `load` invokes the shared materializer.
- Unit tests must pin a shared room's total as the sum of its tenants'
  obligations, a vacant room's empty state, and an ambiguous tenant counting in
  neither room. The ambiguous case must use a **mixed** room — an ambiguous
  tenant sharing with a settled one — because a room whose sole occupant is
  ambiguous is flagged by the empty-obligation branch anyway, so it cannot tell
  the `needs_review` flag apart from that branch. Assert the settled tenant's
  money is still counted **and** the room is flagged.
- Run `go test ./... -count=1` and `go vet ./...` after changing workspace
  reads.

### 7. Wrong vs Correct

Wrong:

```go
// Bypass structured room facts and independently lazy-generate every tenant.
newObligationService(db).ensureMonthlyObligations(ctx, userID, periodMonth)
```

Correct:

```go
newMonthlyRentFactsService(db).ensureMonthlyRentFacts(
	ctx, userID, periodMonth, rentFactsIntentRead,
)
```

This prevents structured/legacy double-generation while preserving the
legacy-only fallback.

## Scenario: Local Test Data Seeding

> The Rosewood SQL fixtures described below use the pre-014 tenant-rent schema.
> They are not a valid seed path for the room-rent-plan schema until regenerated
> for `room_rent_plans` and `room_rent_plan_members`.

### 1. Scope / Trigger

- Trigger: Preparing or refreshing a local, disposable database so the
  authenticated pages have rows to render.
- Applies to `test-data/**` fixtures and `scripts/seed-*.sh` loaders. It does
  not apply to production data, and never to `:8081` / the live host.

### 2. Signatures

- `scripts/seed-rosewood.sh --stage={data|match|all} [--database=...]
  [--mysql-host=...] [--mysql-port=...]` — idempotent two-stage loader.
- `test-data/rosewood/extract.py` — regenerates the fixture JSON and the two
  SQL stages from `收租明细_Rosewood_20260916.xlsx`.
- Fixture outputs: `properties.json`, `rooms.json`, `tenants.json`,
  `agreements.json`, `payments.json`, `review.json`, `seed-data.sql`,
  `seed-match.sql`.

### 3. Contracts

- Fixtures are **generated, never hand-edited**. Change `extract.py` and rerun
  it; a hand edit is lost on the next regeneration.
- Every stage is idempotent: rerunning produces identical row counts. Verified
  for `--stage=all` on 2026-09-19 (4/25/58/40/63/31/186/53/307 unchanged).
- Deletion is **scoped, never a truncate**: only the four known property
  addresses, the known tenant-name set, and rows carrying the `ROSEWOOD-`
  prefix are removed. An unscoped `DELETE` here would destroy hand-made local
  data.
- The loader refuses `:8081`, non-loopback hosts, database names outside the
  local `rentops*` set, unknown stages, unknown flags, and invalid or missing
  user ids.
- Seeding cascades: `rent_obligations` rows are deleted and lazily rebuilt on
  the next load of `/rent-dashboard`, `/tenants`, or `/tenants/{id}`. A freshly
  seeded database therefore has **no** obligations until one of those pages is
  requested.
- The fixture writes `tenants.monthly_rent_cents > 0`, which selects the lazy
  obligation path. `/rent-dashboard` now aggregates those lazy obligations per
  room, so on a freshly seeded database the room tree stays empty until the
  first load of a page materializes the month's obligations, after which room
  amounts match the workspace tenant view for the same month — see "Scenario: Room-Centric Rent
  Workspace Read Model".
- `tenants.room_label` / `room_address` / `property_hint` are denormalized
  copies and must agree with `properties` + `rooms`; the page list and the room
  tree read different sources.

### 4. Validation & Error Matrix

- Non-loopback host or `:8081` -> refuse and exit non-zero before any write.
- Database name outside `rentops*` -> refuse.
- Unknown stage, unknown flag, bad or missing uid -> refuse.
- Rerun on an already-seeded database -> no row-count change; not an error.

### 5. Good/Base/Bad Cases

- Good: regenerate from `extract.py`, run `--stage=all` against local `rentops`,
  then load a page to let the lazy obligations rebuild.
- Base: rerunning `--stage=all` leaves all counts identical.
- Bad: hand-editing `seed-data.sql`, truncating tables instead of scoped
  deletion, or pointing the loader at a shared database.

### 6. Tests Required

- `bash -n scripts/seed-rosewood.sh` plus a rerun-idempotency check comparing
  per-table counts before and after.
- After seeding, assert zero duplicate `(user_id, tenant_id, period_month)`
  groups among lazy obligations.
- Run `go test ./... -count=1` and `go vet ./...` before committing loader or
  fixture changes.

## Scenario: Dashboard Transaction Deferral and Match Lifecycle

### 1. Scope / Trigger

- Trigger: changing the home-page manual-review queue, transaction defer/undefer
  actions, or allocation behavior that changes a deferred transaction's state.
- The dashboard queue is a projection over bank transactions and the append-only
  action log; it must not rewrite the bank transaction to make an item disappear.

### 2. Signatures

- `POST /transactions/defer` and `POST /transactions/undefer`; legacy aliases
  are `POST /billing/defer` and `POST /billing/undefer`.
- Forms submit `transaction_id`, a non-empty `reason`, and an optional local
  `return_to` URL. Return paths are allowlisted by `transactionReturnTarget`.
- `transactionService.deferTransaction` / `undeferTransaction` append
  `payment_transaction_actions` rows with action kinds `defer` / `undefer`.
- `rentWorkspacePendingTransactions(db, ctx, userID, period)` is the shared
  query for both the dashboard count and its three displayed records.

### 3. Contracts

- Action writes and source lookups are scoped by both `user_id` and transaction
  ID. A manual defer is valid only for a pending income transaction.
- Deferring appends an action row. It does not change `match_status`, allocations,
  or rent-obligation balances; the transaction remains visible and matchable from
  the transaction list.
- The queue selects income transactions in the requested transaction month whose
  match status is pending, then excludes a transaction when its latest defer is
  newer than its latest undefer. The count and the `ORDER BY ... LIMIT 3` read use
  this same query.
- A successful allocation appends an undefer action in the same DB transaction
  when the source was deferred. A resulting `partial` transaction can therefore
  return to the manual-review queue; a fully matched transaction is naturally
  excluded by its match status.
- `reason` is trimmed, required, and limited to 512 runes through
  `normalizeTransactionActionReason`.

### 4. Validation & Error Matrix

| Input/state | Result |
|---|---|
| Missing session, transaction ID, or database | Reject before a scoped write |
| Non-POST action request | HTTP 405 |
| Missing/invalid transaction ID or empty reason | Redirect with an action error; write no action |
| Cross-user or missing transaction | Scoped lookup fails; write no action |
| Expense or non-pending income is manually deferred | Reject; preserve source and allocations |
| Latest action is `defer` / `undefer` | Hide / restore the pending transaction in the dashboard query |
| Allocation fails validation | Roll back allocation, projections, and deferral clearing together |

### 5. Good/Base/Bad Cases

- Good: a landlord defers a pending income row; the dashboard count and list both
  stop showing it, while `/transactions` still shows it. A later match appends an
  undefer event in the allocation transaction and projects `matched` or `partial`
  from the effective allocations.
- Base: a matched transaction has an old defer event, but it is absent from the
  queue because its projected match status is no longer pending.
- Bad: changing the source to `ignored` for a temporary skip, deleting a defer
  action to restore the item, or hiding only the displayed list while leaving the
  dashboard count query unchanged.

### 6. Tests Required

- On a disposable database, assert defer preserves `payment_transactions` and
  allocation rows while removing the pending row from both queue count and list.
- Assert undefer makes a still-pending transaction eligible again and that a
  later allocation clears deferral in the same transaction.
- Cover partial and full allocation projections, required reasons, cross-user
  IDs, and the `/billing` aliases. Do not infer DB verification from a skipped
  MySQL test group.

### 7. Wrong vs Correct

Wrong:

```go
// Reuses "ignored" and loses the distinction between a temporary skip and a
// confirmed decision that the bank row is not rent.
source.MatchStatus = "ignored"
```

Correct:

```go
// Preserve source status and ledger facts; the action log controls queue
// visibility, and allocation clears a prior defer in its own DB transaction.
writeTransactionAction(txdb, userID, transactionID, transactionActionDefer, reason, "", "", now)
```
