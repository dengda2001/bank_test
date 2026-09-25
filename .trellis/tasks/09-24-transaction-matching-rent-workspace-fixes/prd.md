# Transaction matching and rent workspace fixes

## Goal

Make bank transaction matching understandable and reliable for a landlord who may know a property and room but not the payer's tenant name. Keep rent history and accounting meaning intact while improving feedback, invoice attachment, and dashboard consistency.

## Confirmed current behavior

- The match drawer sends transaction detail links to `/transactions?detail=…`, leaving the review context.
- The defer client displays one generic failure message for several outcomes. The precise failure on the user's transaction remains unverified.
- The home pending queue excludes deferred income, while the transaction-list pending filter includes deferred income and may include unmatched expenses.
- The highlighted month badge always says “描述提及”, including months inferred from the bank arrival date.
- Monthly rent must be greater than zero; the current validation copy does not identify that rule.
- Rent allocation is capped at the selected obligation's remaining amount. The transaction detail page can classify remaining cash as deposit or other income.
- Invoice upload exists only after an expense has been saved and currently requires metadata fields. The requested replacement is a simple multi-file attachment manager, not invoice metadata capture.
- Property and room deletion is blocked where dependent financial history exists; deactivation remains available.
- Current deactivation only changes status. It does not end an active room rent plan or prevent later monthly rent generation.
- The fixture tenant Arslan Arshad has an end date of 2026-08-31. The live room and the two referenced IE transactions are not present in repository fixtures and need live-data verification.

## Requirements and acceptance criteria

### Matching review

1. Transaction details opened from the match review must appear in an overlay without losing review drafts, selected tenant, or scroll position.
2. Defer must either persist and remove the source from the home queue or show a readable, specific failure. The action must remain retryable after failure.
3. Intelligent suggestions use a blue treatment distinct from errors.
4. Invalid allocation, failed add, and failed defer feedback appears in a prominent accessible floating notice. The error is also available to assistive technology.
5. The user can locate candidate tenants by property and room, then inspect a tenant's monthly evidence before allocation. This selection does not by itself create or change a payer relationship.
6. A unique verified payer-to-tenant relationship or uniquely matching payer and tenant name can preselect the “remember payer” choice when that tenant is part of the current draft. Ambiguous or different names must leave it empty. No relation is silently saved before confirmation.
7. Existing verified payer relationships preselect the appropriate matching tenant on later transactions where unambiguous.
8. Month evidence distinguishes an explicit rent-month reference in the description from a date-based suggestion. A date-only suggestion must never claim that the description mentioned the month.
9. When no bill exists for a suggested month, the review explains which tenant and month were checked and offers an actionable next step without inventing a bill.
10. The user may explicitly choose an overpayment match for a tenant. A €620 payment against €600 unpaid rent records €600 as that month's rent and €20 as that tenant's pending prepayment. The current month's bill and collection percentage do not exceed their expected amount. The bank source remains fully accounted for. The prepayment stays pending until the user explicitly applies it to a later rent bill; no automatic deduction occurs. Refund functionality is outside this task.

### Transaction list and dashboard

11. Remove income/expense row background tint on desktop and mobile. Add a separate direction marker column on desktop, with a green income icon and red expense icon; preserve an understandable marker on mobile.
12. Home pending count and its link to the matching list use clearly consistent scope. Deferred items remain findable in transaction history, with a distinct explanation of their state.
13. The paid-rent dashboard metric contains a progress bar with the collection percentage beside it.

### Expenses and assets

14. The create-expense flow accepts multiple optional attachment files at creation. An existing expense supports adding, downloading, and removing individual files. No invoice number, vendor, date, amount, or note is required. Existing invoice files remain downloadable and associated with the correct user and expense.
15. Replace the user-facing property/room delete action with soft deletion. The asset and its rent, payment, and expense history remain stored and visible in historical context; the asset disappears from default daily lists. A still-effective occupancy/rent plan must first end from an explicit month, so soft deletion does not allow future rent generation. No restoration UI is provided.
16. A tenant change at a month boundary can be entered on a room so that the old tenant's prior months remain intact and the new tenant owes rent from the selected month. The UI explains how to do this.
17. A zero monthly rent gives a specific explanation that rent must be greater than zero and points to ending occupancy for a vacant room.

### Specific records

18. Diagnose the missing-bill message for IE26090426926842 against live data if available. Record the payer, selected tenant, suggested month, room plan coverage, and resulting fix or reason.
19. Verify the month-evidence label for IE26083165993723 against its actual description and arrival date if available.
20. Verify Arslan Arshad's actual room plan and the September successor against live data if available; no historical months may be reassigned.
21. The matching review initially leaves the tenant selection empty. A suggested or remembered tenant remains available through a clickable button; choosing it or manually choosing a tenant fills the corresponding property and room when the viewed month has a unique room. Remove the separate occupancy-month finder control.
22. Investigate why deferring IE26090367179222 fails. Distinguish its stored-state eligibility from the actual server-side write error, and retain diagnostic detail for future failures without exposing database errors in the UI.

## Constraints

- Preserve confirmed financial allocations and historical room/rent facts.
- Do not infer a payer relationship from a shared or ambiguous name.
- Do not silently classify excess rent as other income, a deposit, or future rent. Prepayment requires an explicit choice.
- Keep changes usable on mobile and with keyboard navigation.

## Product decisions

- Explicit overpayment matching creates a tenant prepayment, capped after the selected month's rent is paid.
- A later month consumes prepayment only through an explicit user action.
- Refund handling is deferred; do not expose a refund action in this task.
- Expense files are generic attachments; multiple files may belong to one expense.
- New expense attachments accept PDF, JPEG/PNG/WebP images, and modern Word/Excel/PowerPoint documents.
- Property and room deletion means soft deletion: preserve asset and financial history, hide from default daily lists, and end active occupancy first.
- Soft deletion has no restoration action in the product UI; historical links remain readable.

## Delivery map

This is the parent requirement set. Independently verifiable child tasks cover the prepayment ledger, matching review, transaction list/dashboard, expense attachments, and room/asset guidance. The prepayment ledger must establish its contract before matching review integrates the overpayment choice. Final integration checks compare child outcomes with all criteria above.
