# Design: manual transaction matching and limited match editing

## Boundaries

- `/billing` becomes a read-only presentation of calculated matching
  suggestions. It must not call `reconcileTransactions`, persist match status,
  mutate parsed-period facts, or create rent allocations.
- `/rent-dashboard` and legacy import likewise stop invoking reconciliation.
  Dashboard obligation generation is unchanged; it is not a transaction-match
  decision.
- The only transaction mutation paths are explicit POST requests from billing:
  a new manual one-click match and a limited rematch action. Bank transaction
  source fields remain immutable.

## Billing page model and interaction

For each income row with no effective allocation, the service calculates a
strict payer/tenant/month decision in memory from existing payer relations and
rent obligations.

- If a complete valid target is found, the row shows the suggested tenant and
  rent month plus a `一键匹配` POST. The request contains the transaction and
  target identifiers; the server re-validates both.
- If the suggestion is incomplete or needs review, the row continues to offer
  the explicit rent allocation controls rather than changing a stored match
  state. The user selects the tenant and rent month as part of that existing
  manual workflow.
- A row with exactly one effective `rent` allocation shows `修改匹配`. Its
  form uses independent tenant and rent-month selectors; it does not render a
  pre-expanded tenant × month option list. The only editable facts are the
  matched tenant and rent month. It neither renders nor accepts inputs for bank
  date, payer, description, reference, amount, currency, or identifiers.
- Rows with multiple effective allocations, or any effective non-rent
  allocation, show the existing revoke/reclassify route and no direct rematch
  form.

## Service contracts and atomicity

The explicit-match service resolves a user-scoped rent obligation, validates
the income source, currency, tenant ownership, and the outstanding obligation
balance under row locks, then writes a confirmed rent allocation through the
existing ledger allocation mechanism. It may record the payer relation only
after the allocation succeeds.

The rematch service accepts only a transaction ID, target tenant ID, and target
`YYYY-MM` rent month. Inside one database transaction it resolves the
user-scoped rent obligation and then:

1. locks and reads the source transaction and its allocations;
2. rejects anything other than exactly one effective rent allocation;
3. voids that allocation, re-projects the old obligation and writes the
   existing revoke audit action;
4. creates an equivalent rent allocation for the selected target and reprojects
   the new obligation; and
5. updates the source match projection from persisted allocations.

The old allocation is retained as `voided`; no historical bank data or
allocation row is rewritten. The replacement uses a distinct manual-rematch
confirmation source, so audit and later review can distinguish it from the
initial manual one-click match.

## Compatibility and error handling

- Existing `/billing/confirm` behavior remains a POST-only manual matching
  path, but gains the target validation needed by the one-click form.
- Add a dedicated POST rematch endpoint and redirect failures to the billing
  error state. It rejects invalid, foreign, paid/over-capacity, ignored, or
  split/mixed sources without changing either obligation.
- Revoke remains the rollback/correction path for split and mixed rows.
- No schema migration is needed: allocation status, operation IDs, void data,
  confirmation source, and transaction actions already provide the required
  audit trail.

## Verification shape

Add renderer tests for the new buttons/forms and MySQL ledger integration tests
for no-write browsing/import, explicit matching, atomic rematch, audit
retention, capacity/currency/tenant isolation, and rejection of split/mixed
allocations. Run the full Go test and vet suites; the MySQL tests may skip when
their configured DSN is absent.
