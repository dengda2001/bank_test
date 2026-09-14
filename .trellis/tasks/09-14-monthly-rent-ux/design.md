# Design draft — monthly rent UX

Status: planning; month-allocation decision confirmed, pending final review.

## Layout

- Default landing: monthly rent dashboard.
- Header: 本月收租, selected month, previous/next month and 回到本月.
- Primary row: 本月应收 / 已收租金 / 剩余未收; one collection progress bar and paid-household count.
- Action area: unsettled tenants and pending incoming transactions; show all tenants on demand and retain expandable payment details.
- Secondary area: bank income and expenses by transaction month, with explicit distinction from rent allocation month.
- Shared control styles and interaction helpers for month, date and tenant/status selection across server-rendered Go templates. Keep rounded dark visual direction; select concrete control implementation after pre-development guidelines and browser review.

## Data and boundaries

- Keep rent obligations and confirmed allocations as the rent-summary source. Count billed tenants for the selected period. Pending-transaction counts must query transactions rather than reuse obligation review counts.
- Store explicitly confirmed payer relationships scoped to the authenticated user. Support multiple payer names per tenant without overwriting existing hints. Exact normalized names only; no fuzzy automatic matching.
- Resolve identity separately from allocation eligibility. Model a tenant association for transactions awaiting month/amount review so that identity need not be selected repeatedly.
- Prefer a unique known payer ID; name rules cannot silently override conflicting known identity. Unconfirmed tenant-name hints remain candidates.
- Shared historical/future reconciliation path: resolve tenant, resolve month, validate currency and remaining balance, then allocate or mark pending review with a reason.
- Confirmed decision: remove nearest-open-month fallback from automatic allocation. Explicit reference month takes precedence; otherwise use transaction month only when eligible. Unclear cases require choosing a month while preserving the tenant association. Show expected, paid and remaining values per month and preview the allocation before confirmation. Validate user/tenant ownership, currency and remaining balance again at submission; maintain idempotency.
- Process batches against fresh obligation balances; use database transactions and uniqueness/locking to prevent duplicate allocation or concurrent oversubscription. Preserve source and batch result counts for user feedback.

## Compatibility and migration

- Use an additive migration for payer rules and transaction association as required; do not rewrite confirmed allocations or assume legacy hints are trusted rules.
- Keep internal statuses and routes stable where possible; translate display labels centrally, retaining external bank data verbatim.
- Preserve currency separation; never sum distinct currencies into a single currency-labelled number.
- Provide removal/correction of remembered relationships without silently rewriting confirmed historical payments; finalize interaction in implementation design review.

## Validation and rollback

- Test identity propagation without payer IDs, sequential partial payments, duplicates, conflicts, user isolation, invalid currency and cross-month allocation.
- Verify totals and pending links against fixtures. Exercise all control types in a real browser at desktop/mobile sizes.
- Do not test using live bank refresh or mutate real financial records. Use isolated fixtures/database.
- Roll back application changes without deleting financial history; any newly confirmed allocations require explicit audited correction, not migration reversal.
