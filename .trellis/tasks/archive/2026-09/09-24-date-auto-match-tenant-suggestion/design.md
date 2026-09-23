# Design

## Decision boundary

`decideStrictRentMatch` remains the single background decision path. For TrueLayer income, use description evidence first. If description has no explicit rent month, use the Europe/Dublin arrival date only on days 1–5 (current month) and 25–31 (next month). Days 6–24 remain suggestions in the UI. `RENT_AUTO_CURRENT_THROUGH_DAY` and `RENT_AUTO_NEXT_MONTH_FROM_DAY` adjust these bounds; invalid values fall back to 5/25 and are constrained around the existing suggestion cutoff. Exclude descriptions with clear non-rent markers from date-based allocation. Do not read reference, TxnDate, or persisted legacy parsed month as rent-month evidence.

Identity candidates come from active `tenant_payers` by stable payer ID/name and from a uniquely equal normalized official tenant name when `payer_name_kind=confirmed`. Any disagreement or multiplicity blocks allocation. A date-derived allocation requires one open obligation, same currency, and exactly its unpaid amount; explicit description months keep existing partial-payment behavior. Auto allocations use `confirmation_source` beginning with `auto_`, including `auto_exact_name` for direct name matches.

## Data flow

Ingestion and re-reconciliation call `reconcilePendingRentTransactions`. It skips ignored, deferred, and allocated transactions; materializes current/past monthly facts for eligible evidence; evaluates the decision; applies an allocation or updates the pending projection. For a future month inferred at the end of the previous month, it first requires a uniquely identified tenant and exactly one active rent-plan member with the same amount and currency. Only then may it materialize the next month's obligation and repeat the decision. Existing confirmed allocations are never rewritten.

`enrichTransactionPageRow` derives a display-only method label from effective rent allocations. The list, detail, and review drawer share that field. No schema migration is needed.

The manual review drawer receives up to three ranked tenant-name suggestions from confirmed bank payer names. Unicode-aware normalization and edit/token similarity handle minor spelling differences, extra middle names, prefixes, and abbreviations. These links only select a tenant for review; they do not create an allocation. The complete searchable tenant select remains available.

## Risks and compatibility

Reconciliation rechecks each transaction against current obligation balance, so two same-month payments do not both allocate. An existing pending row may auto-match on a later reconciliation; no live-data replay is part of development. Source-derived badges cannot reconstruct the source of legacy allocations lacking a source field, so those display as manual/previously confirmed.
