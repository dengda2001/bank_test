# Existing tenant and receipt matching flow

## Surfaces and routes

- `web/templates/pages/rent-workspace.html` renders the tenant view in desktop table and mobile cards. A row has tenant and room identity, monthly expected/paid/outstanding amounts, status, `一键平账`, and tenant detail.
- `rent_workspace_page.go` renders the workspace and opens the existing review drawer when `match=<transaction ID>` is present. It already accepts `match_tenant=<tenant ID>` and `match_month=<YYYY-MM>`; `match_origin=lookup` avoids explicit monthly fact materialization during a lookup.
- `web/templates/partials/transaction-match-review-drawer.html` and `web/static/js/transaction-match-review.js` own the receipt-first review, draft basket, evidence, detail overlay, and final confirmation.
- `transaction_match_review.go` builds the review data. Its workspace URL setup currently closes to the base workspace URL, so a new finder needs an explicit back-to-results path if review is entered from it.
- `transaction_match_batch_handlers.go` submits to `/transactions/confirm-batch`; `transaction-match-batch.md` requires the existing atomic, idempotent allocation contract to remain the only write path.

## Search and status facts

- `pendingMatchStatuses` in `transactions.go` includes `candidate`, `needs_review`, `unmatched`, and `partial`; `partial` can still have an unallocated remainder.
- `rentWorkspacePendingTransactions` already filters income and pending statuses by transaction time and excludes actively deferred receipts. A tenant finder may reuse this base query pattern but must span two calendar months and support explicit earlier ranges.
- `applyTransactionFilters` searches payer name, payer ID, and description with a single `LIKE`; it searches tenant names only when a transaction is already associated with the tenant. An unassociated receipt needs a new tenant-name-derived payer/description query.
- `manualTenantSuggestions` and `tenantNameTokens` provide edit-distance and token hints for the receipt-first direction. The project contract says fuzzy similarity is presentation-only, never automatic payer identity or allocation.

## Accounting distinction

- `collection-settle-form.html` labels a tenant-row action `一键平账`. `dashboard_manual_balance.go` shows it creates a synthetic income transaction and allocates it to the obligation. It does not search for or attach an existing bank receipt.
- A finder should explain that suggested receipts are only candidates and require bank evidence review; otherwise users may confuse the finder with artificial settlement.

## Local constraints

- The workspace has pre-existing uncommitted changes in its template/CSS and the receipt review partial. Implementation must preserve and integrate with those changes.
- Frontend is Go `html/template` plus embedded CSS/JS. Relevant guidance: `.trellis/spec/frontend/index.md`, `responsive-conventions.md`, `rent-workspace-navigation.md`; backend: `.trellis/spec/backend/transaction-match-batch.md`, `transaction-auto-match.md`.
