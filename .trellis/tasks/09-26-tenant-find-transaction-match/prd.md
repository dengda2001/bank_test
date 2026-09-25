# Find bank receipts from a tenant row

## Goal

Let a landlord start with a tenant's monthly rent row, find a plausible bank receipt, inspect its original evidence, and allocate it through the existing confirmation flow.

## Confirmed facts

- The entry surface is the tenant view of `/rent-dashboard`; it is a month-specific table with a tenant, room, monthly amount, paid amount, balance, status, and actions.
- The existing receipt-first review drawer opens with `match=<transaction ID>`. It supports selecting a tenant, inspecting monthly evidence, adding one or more draft allocations, optional prepayment and payer association, and a single final confirmation through `/transactions/confirm-batch`.
- Pending income statuses include `candidate`, `needs_review`, `unmatched`, and `partial`. Deferred receipts are separately excluded from the current workspace pending queue.
- Current fuzzy tenant suggestions are presentation hints only. Similar names are not sufficient for automatic accounting or payer identity.
- The existing transaction list can search payer, payer ID, description, and already associated tenant names. It does not provide a tenant-name search across unassociated receipts.
- The tenant row's existing `一键平账` action creates a new synthetic `manual_balance` income transaction and immediately allocates it; it does not select an existing bank receipt.

## Requirements from the request

- Add a tenant-first entry in the tenant view that opens a receipt-finding drawer for that tenant and the visible rent month.
- Initially list pending receipts from the current calendar month and previous calendar month, including payment time, payer, description, amount, and useful status/balance context. Allow the user to extend the date scope to older receipts.
- Provide one-click name-based search suggestions based on the tenant's name, including tokenized searches for names separated by spaces.
- Provide direct search of payer name or description and clear button controls for the available search choices.
- Keep finding, inspecting, drafting, and confirming inside one tenant-first drawer; selecting a receipt must not open a second drawer or full page. Reuse the receipt-first allocation rules and confirmation endpoint.
- Selecting a receipt prefills tenant, rent month, and a safe draft amount inside that drawer. The user then presses `确认分配` to post it; selecting a receipt alone does not write to the ledger. This interaction was explicitly chosen by the user.
- Show the bank-receipt finder and the existing manual-balance action as two side-by-side tenant-row controls; distinguish their meanings clearly in labels/help text.
- Show `找银行流水` on every tenant row. When the selected month is already paid, the finder explains that this month has no remaining balance but may still help inspect other months or prepayments. (Planning assumption pending user review.)

## Acceptance criteria

- [x] Opening the finder from a tenant row retains tenant and rent-month context; closing it returns to the same filtered workspace view.
- [x] The initial list shows only authenticated user's eligible income receipts in the default arrival-date window and exposes payment time, payer, description, amount, remaining balance, and status.
- [x] Search suggestions are explicitly labeled as candidates, allow easy switching/clearing, and do not allocate or remember a payer without confirmation.
- [x] Free-text search checks payer and description; the user can extend the initial date range to older receipts; an empty result gives a useful next action.
- [x] Selecting a receipt shows its full evidence and a prefilled allocation draft for the current tenant/month inside the same drawer position, with a clear final `确认分配` action. There is no second drawer transition.
- [x] A receipt that cannot cover the current month shows the reason instead of submitting an invalid allocation. Partial receipts and multi-month/tenant allocation retain the existing review capabilities.
- [x] The existing receipt-first flow, ledger validation, idempotency, and return behavior remain intact.

## Accepted product decisions

- Show the finder on every tenant row. Show a settled/no-bill notice if the selected month has no unpaid rent.

## Out of scope (proposed)

- Automatic allocation based on fuzzy names.
- A new ledger mutation endpoint or separate allocation rules.
