# Workspace interaction and consistency remediation assessment

## Goal

Assess the real code impact, dependencies, delivery order and effort for the 15 requested workspace fixes, then produce an implementation-ready split that can be reviewed before any business code changes.

## User value

- Reduce navigation between property, room, tenant, expense and transaction workflows.
- Make month pickers, selects, pagination, buttons and text inputs use one interaction and visual language.
- Let the landlord identify, match or defer incoming transactions from the dashboard.

## Requirements

1. Creating an expense from a property or room page opens a drawer on the same page and carries the property/room context automatically.
2. A selected receivable-bill filter remains selected and affects the result instead of reverting visually to “未结清”.
3. Paginated lists show clickable page numbers as well as previous and next controls.
4. The dashboard month control uses the shared month-picker style and has previous/next month controls outside it.
5. Visible filter action buttons use the label “搜索”.
6. Every date/month input uses the transaction-matching calendar style and consistent corner radius, including property, room and tenancy pages.
7. Tenant management supports search.
8. Editing from tenant detail stays in tenant detail and opens its drawer there.
9. Existing paginated lists default to 10 rows per page.
10. The transaction list labels column two “付款人” and sources it from the bank transaction, adds a matched tenant-name column, exposes a match action at the right of detail, and uses separate tenant and month selectors.
11. Dashboard manual-review transactions render as up to three standalone rounded cards. Processing reveals payer, time, note/reference and amount plus tenant/month selection, match and defer actions. Match or defer removes the item and refills the queue to three when possible. Deferred transactions remain visible and matchable in the transaction list.
12. All selects use the same light button-like system visual and open anchored to the clicked control.
13. Remove the cash-receipt and expense sidebar tabs. Add “现金补录” and “新增支出” actions to transaction matching and preserve the current drawer contents and style.
14. Increase the clickable area of the clear action shown for non-empty text search fields.
15. Remove the shared top-right search field and adjacent count/action button while keeping page-local search controls.

## Confirmed repository facts

- The bill-filter reset is a template selected-state defect, not a query/service defect.
- The dashboard pending queue currently fetches only two records despite prototype comments and numbering supporting three.
- Transaction detail already has matching logic, but the action is nested inside a disclosure rather than exposed directly.
- Property/room expense actions navigate to `/expenses`; the receiving form currently ignores the supplied property/room IDs for selection.
- Shared calendar CSS/JS exists but is not loaded by all workspace templates.
- Existing paginated defaults are 12 for workspace/bills/tenant history and 50 for transactions.
- Tenant management has no list search state or search UI.
- Tenant detail edit links intentionally route to the list drawer in the current implementation.
- Approximately 42 native select controls exist across the server-rendered pages; meeting the requested popup appearance requires a shared custom select enhancement because native popup styling is controlled by the OS/browser.

## Constraints

- This planning task inspects, estimates and decomposes the work; it does not modify business code.
- Shared components and styles should solve repeated behavior instead of page-by-page copies.
- The estimate must distinguish template/UI work, backend query/state work, data-model impact and browser-verification cost.
- Preserve the unrelated untracked file `收租明细_Rosewood_20260916.xlsx`.
- Keep the existing product contracts that match status, allocation kind and bill payment status are separate, and that “remember payer” defaults to checked but remains optional.

## Acceptance criteria

- [x] Every requested item is mapped to current implementation evidence and likely affected areas.
- [x] Every item has a size, main risk and dependency assessment in `research/codebase-findings.md`.
- [x] Cross-cutting shared-component opportunities are identified.
- [x] Dashboard “暂不处理” uses a distinct persistent `deferred` state/action.
- [x] Recommended phases and independently verifiable task splits are recorded in `design.md` and `implement.md`.
- [x] The user approves implementation after reviewing the planning artifacts.

## Out of scope

- No business-code edits, schema migration or production-data change in this phase.
- Do not enter the Trellis implementation phase without user review.

## Confirmed product decision

“暂不处理” uses a distinct persistent `deferred` transaction state/action. It disappears from the dashboard queue, remains visible and directly matchable in the transaction list, and does not acquire the incorrect “非租金/无需匹配” meaning of the existing `ignored` state. A later successful match replaces `deferred` with the resulting matched or partial state.
