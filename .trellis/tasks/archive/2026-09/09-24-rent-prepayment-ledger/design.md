# Design: tenant prepayment ledger

## Decision and boundaries

An overpayment is an explicit third allocation use, `prepayment`, owned by one tenant and backed by one incoming bank transaction. It consumes source budget but no monthly obligation. Existing rent obligations continue to count only `rent` allocations. The user has chosen manual future use and no refund feature in this task.

## Storage and contracts

- Add a `tenant_prepayments` table with owner, tenant, original income transaction, original amount, currency, created timestamp, and immutable operation identity. Add nullable `prepayment_id` to `payment_allocations` for prepayment and later rent rows funded by that credit.
- Add `allocationKindPrepayment` to ledger validation/effective-kind projection. A prepayment requires an owner-scoped tenant, positive EUR amount, income source, and no rent obligation.
- Extend batch confirmation with optional `{prepayment_tenant_id, prepayment_amount}`. The reviewer must explicitly opt in, and the prepayment amount must equal the source amount left after existing effective allocations and this batch's rent rows. This prevents an arbitrary hidden remainder. Save rent rows, credit row, payer relation, and source projection in one locked transaction with the existing request key.
- Derive a credit's live available balance from effective `prepayment` allocations linked to its ID. Do not treat it as rent paid. Display the original and available amounts with source reference on the tenant and transaction surfaces.
- A manual “apply to rent” command locks the credit, source, and target obligation; validates same owner/tenant/currency and target unpaid balance. It voids the currently effective prepayment allocation, writes a `rent` allocation for the applied amount linked to the credit, and writes a replacement prepayment allocation for any residue, all in one transaction. Each replacement gets a fresh idempotency key and shares an operation ID. Effective source allocations still sum to no more than the bank receipt. Historical voided rows remain for audit.
- Full-source revoke and exact-share revoke must account for linked credit rows. A full-source revoke voids its effective credit and rent shares; a direct revoke of a credit-funded rent share restores the amount to the same tenant credit in the same transaction. No orphaned positive credit is allowed after the source is revoked.

## Cross-layer behavior

The transaction summary and source status include effective prepayment allocations. Monthly paid rent includes only effective rent allocations. The detail page names the prepayment separately from rent/deposit/other income. Applying credit to rent shows the original bank source as payment provenance and the credit operation in audit history. Existing automatic matching never creates a prepayment.

## Rollout, compatibility, rollback

The additive migration preserves all old allocations. New code must tolerate rows without `prepayment_id`; legacy rent/deposit/other rows are unchanged. Do not enable the overpayment UI before the migration and service tests pass. Rolling back the UI leaves stored prepayments readable only by the new code, so application rollback requires retaining the new ledger reader or disabling new writes first.

## Verification

Test €620 → €600 rent + €20 prepayment; apply €12 later and leave €8; insufficient credit/obligation, cross-user, duplicate request, changed retry, full-source revoke, exact-share revoke, and concurrent application. Run the MySQL tests on a disposable DB when available; skip only under the repository's existing DSN convention.
