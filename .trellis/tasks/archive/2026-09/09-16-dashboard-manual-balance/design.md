# Dashboard Manual Balance Design

## Boundary

This feature adds an authenticated, state-changing Dashboard action. It creates
a `payment_transactions` income record with `source=manual_balance` and
`description=手动平账`, then allocates the record to one rent obligation in the
same database transaction. No API response shape, database schema, or existing
bank transaction is changed.

## Data flow

```text
Dashboard row with balance
  -> POST /rent-dashboard/settle (obligation_id + return filters)
  -> settleRentObligation(userID, obligationID)
  -> lock obligation; compute current remaining balance
  -> create manual_balance income transaction for that exact balance
  -> create confirmed rent allocation; project obligation as paid
  -> redirect back to the same Dashboard filters
```

## UI contract

- The status cell renders a compact `一键平账` POST form only when
  `ExpectedCents > PaidCents`.
- The form uses a native `confirm()` prompt before it submits. The prompt is
  static, so tenant-controlled content is not interpolated into JavaScript.
- The form includes the obligation ID only as a hidden transport field. The
  server scopes the lookup to the authenticated user and never trusts any
  client-supplied amount, tenant, or currency.
- Success redirects to the filtered `/rent-dashboard` view with a success
  notice; a concurrently settled/stale row redirects with an informational
  “no balance remains” notice.

## Ledger contract

- The service locks the user-scoped obligation, recomputes its paid amount from
  effective bank allocations and cash receipts, and derives the amount from
  that locked state.
- A non-zero balance produces exactly one EUR income transaction, marked
  `manual_balance`, with a current timestamp, `手动平账` description, and the
  selected obligation month as its parsed period.
- The transaction and confirmed `rent` allocation are inserted atomically. The
  shared allocation validation and projection logic remains the authority for
  currency, ownership, and overpayment checks.
- A retry or concurrent second request waits on the obligation lock, sees no
  remaining balance, and creates no additional transaction.
- `manual_balance` is excluded from background reconciliation. If its
  allocation is later revoked using the existing audit path, the Dashboard can
  offer a new settlement rather than auto-reapplying the old record.

## Error and rollback behavior

| Condition | Result |
| --- | --- |
| Missing auth, non-POST | Existing auth/method response behavior |
| Invalid or cross-account obligation ID | Dashboard error redirect; no record |
| Void or already-paid obligation | Informational Dashboard redirect; no record |
| Currency or projection validation failure | Dashboard error redirect; transaction rolls back |
| Double click / concurrent request | At most one record; later request is a no-op |
| Incorrect manual balance | Use the existing allocation-revoke flow, which retains audit history |

## Compatibility

- Existing `payment_transactions.source` values are unconstrained strings, so
  `manual_balance` needs no migration.
- Existing `/billing` filters, allocation projections, tenant history, and
  Dashboard totals consume the new record through their normal transaction and
  allocation queries.
