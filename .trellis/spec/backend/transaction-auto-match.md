# Automatic Rent Matching

## Scenario: Reconcile unallocated bank income with rent

### 1. Scope / Trigger

`transactionService.reconcilePendingRentTransactions` runs after bank ingestion and other reconciliation triggers. It processes only unallocated, non-deferred `income` transactions in `unmatched`, `candidate`, or `needs_review`. A confirmed allocation is never recalculated or rewritten.

### 2. Signatures

- `decideStrictRentMatch(tx paymentTransactionInput, payers []tenantPayer, tenants []tenant, obligations []rentObligation) matchDecision`
- `autoRentPeriod(tx paymentTransactionInput) (*time.Time, bool)` returns a month and whether it was inferred from the bank arrival date.
- `RENT_NEXT_MONTH_FROM_DAY` (default `15`) controls the **display suggestion** cutoff.
- `RENT_AUTO_CURRENT_THROUGH_DAY` (default `5`) and `RENT_AUTO_NEXT_MONTH_FROM_DAY` (default `25`) control the automatic date windows. Bounds must straddle the suggestion cutoff; invalid values fall back to the defaults. An incompatible suggestion cutoff disables date-based automatic matching.
- `transactionService.futureRentPlanMatchesTransaction(ctx, userID, tenantID, period, amountCents, currency) (bool, error)` gates future fact creation using `room_rent_plan_members` joined to active `room_rent_plans`.
- `payment_allocations.confirmation_source` records `auto_id`, `auto_name`, or `auto_exact_name` for automatic allocations. `transactionPageRow.MatchMethodLabel` derives the display label from effective rent allocations.

### 3. Contracts

For TrueLayer income, an explicit rent month in `description` wins on every arrival day. Strip `TxnDate` and full dates before parsing; `reference` and legacy persisted parsed months do not provide explicit evidence. Without an explicit month, local `Europe/Dublin` arrival days 1–5 suggest the current month and days 25–31 suggest the following month for automatic matching. Days 6–24 remain pending with the existing display suggestion. Deposit, refund, expense, loan and borrowing descriptions do not qualify for date-based automatic matching.

Identity is unique only when an active `tenant_payers` relation identifies one tenant, or a bank-confirmed payer name equals one tenant's official `tenants.name` after trimming, case folding and collapsing spaces. Multiple or conflicting relations/name matches stay pending. Fuzzy name similarity is presentation-only in the manual review drawer.

Date-inferred matching requires exactly one active obligation for that tenant and month, matching nonempty currencies, positive amount, and a transaction amount equal to the obligation's unpaid balance. Explicit description months retain the existing partial allocation behavior. Reconciliation creates current/past monthly rent facts with `rentFactsIntentRead`. For a future date-inferred month, it may first create monthly facts only when identity is unique and exactly one active rent-plan member has the same amount and currency; it then reloads obligations and rechecks the complete match decision. The allocator revalidates balances and uses an idempotency key before writing.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Unknown/inferred payer name without remembered relation | No automatic tenant identity |
| Payer ID/name relations disagree, or exact official name points elsewhere | `needs_review`; no allocation |
| Arrival day 6–24 without explicit description month | `candidate` when identity known; no allocation |
| Date-derived target has zero or multiple open obligations | Pending review; no allocation |
| Date-derived amount differs from unpaid balance or currency differs | Pending review; no allocation |
| Existing allocation, deferred transaction, or ignored status | Reconciliation skips it |
| Future date-derived month without obligation, but one exact active rent plan | Materialize monthly facts, then re-evaluate the full decision |
| Future month with no plan, multiple plans, or plan amount/currency mismatch | Pending review; no future fact creation |

### 5. Good / Base / Bad Cases

- Good: A bank-confirmed unique official name pays the exact unpaid rent on 3 September; the September obligation is allocated with `auto_exact_name` and the list shows `自动匹配`.
- Base: The same payer pays on 14 September with no explicit rent month; the list suggests September and the row waits for confirmation.
- Bad: `TxnDate: 25Apr2026` in a 31 August bank description is treated as April rent, or a similar-looking tenant name is used for background allocation.

### 6. Tests Required

- `TestStrictRentMatchDateWindowAndExactTenantName`: 1/5/6/14/15/24/25/30 boundaries.
- `TestDateAutoWindowIsConfigurableAndUsesDublinCalendarDay`: configurable windows and UTC-to-Dublin day crossing.
- `TestStrictRentMatchDateWindowHonorsIdentityAndPaymentGuards` and `TestStrictRentMatchDateWindowRejectsMultipleOpenObligations`: identity, amount, currency, non-rent, and obligation ambiguity.
- `TestBankTxnDateOnlyUsesTransferDateForAutoMatching`: `TxnDate` does not select the rent month.
- `TestReconcilePendingRentTransactionsOnMySQL`: on a disposable DB, create only a future plan, rerun reconciliation, assert one generated obligation and allocation, and assert the earlier allocation remains unchanged; this test skips without `RENTOPS_MYSQL_TEST_DSN`.
- `TestManualTenantSuggestionsFindCloseBankNamesOnly` and `TestMatchMethodLabelUsesOnlyEffectiveRentAllocations`: fuzzy suggestions remain manual and badges reflect confirmed effective rent allocations.

### 7. Wrong vs Correct

Wrong:

```go
period := bankTransactionPeriod(tx.Description, tx.TransactionTime).Month
// Every transfer date becomes a month eligible for automatic allocation.
```

Correct:

```go
period, inferred := autoRentPeriod(tx)
// Only explicit description evidence or the configured arrival-date windows
// can reach allocation; inferred dates also require an exact obligation balance.
```
