# Tenant-first receipt finder: interaction and technical design

## Interaction model

Every tenant row opens one `找银行流水` drawer. It anchors the selected tenant and the workspace rent month. If that month is fully paid, the drawer says so and offers inspection of other months/prepayment. The default drawer state is a searchable receipt list. Choosing a receipt replaces the list content with the existing full evidence and allocation review in the same drawer position; it does not stack another drawer. The URL preserves the list filters for returning. The review shows bank evidence, existing allocations, target rent month, and an editable draft amount initially set to the lower of the receipt's unallocated balance and the rent month's unpaid balance. Only `确认分配` writes to the ledger through the existing endpoint.

The user explicitly chose this one-drawer prefill-then-confirm interaction over immediate posting on receipt selection.

### Proposed drawer layout

```text
┌ 找银行流水 · Qiang Li                  [×] ┐
│ 2026年9月租金 · 未付 €1,200               │
│ 到账日期 [近两个月] [近六个月] [全部时间]   │
│ 查看     [全部待处理] [姓名线索]          │
│ 姓名线索 [Qiang Li] [Qiang] [Li]         │
│ 搜索     [全部字段] [付款人] [Description]│
│          [输入关键词________] [搜索] [清除]│
│                                         │
│ 12 笔可核对 · 姓名仅作查找线索             │
│ 9月12日 10:45    €1,200 · 待确认          │
│ 付款人  QIANG LI                         │
│ 描述    September rent ...               │
│ 剩余可分配 €1,200 · 线索：付款人完整姓名    │
│                         [用于本月租金]    │
│ ...                                     │
│ [上一页]  1 / 2  [下一页]                │
│ [用于本月租金] → 同一抽屉位置显示核对页 │
│ ── 已选流水 · 核对并分配 ───────────────  │
│ 完整银行原文 / 已分配 / 剩余可用           │
│ Qiang Li · 2026年9月 · 本次 €1,200 [编辑] │
│ [更多月份或租客]           [确认分配]     │
└─────────────────────────────────────────┘
```

On mobile, each receipt is a stacked card with the amount and action still visible. Long bank descriptions wrap or clamp with an expansion control; the payer and original description remain accessible. Empty results explain the active date/keyword filters and offer `清除搜索` or `查更早流水`.

### Search semantics

- Default: all eligible pending income receipts received from the first day of the previous calendar month through the end of the current calendar month, newest first. Determine month boundaries in the existing bank-local `Europe/Dublin` calendar and convert them to stored transaction timestamps for the query. Show payment date and time in that calendar. The window concerns bank arrival time, while the selected rent month remains the allocation target; they can differ.
- Scope buttons: current plus previous month, recent six months, or all dates. Search operates inside the selected scope. Counts and pagination use the same query.
- `姓名线索` is a user-selected view that ranks matching receipts; it never hides the existence of other pending receipts. Whole official name and alias (when present), plus whitespace/name-token variants, are explicit buttons. Each button searches payer and description for its exact text as a case-insensitive substring. Short/common tokens can produce noisy results; label them as broad clues and rank them below full-name hits.
- Free-text search has field buttons `全部字段`, `付款人`, and `Description`; only the chosen field set is searched. The receipt list labels why a result appeared (`付款人包含…`, `Description 包含…`, or saved payer relation when relevant), but does not call it a confirmed identity.
- Date, mode, token, free text, and pagination are URL state so browser back/refresh and return to results are stable. The list is server-filtered, tenant-owned, income-only, and paginated before rendering.

### Allocation inside the finder

- `用于本月租金` selects one receipt and replaces the list with its allocation review inside the same drawer position. It prefills the current tenant, workspace rent month, and a bounded draft amount; a click does not persist the match. The user can inspect the full bank reference and description, existing allocations, source remaining amount, and monthly obligation evidence before confirming. Opening this area must not create future rent facts merely from a GET; the existing explicit month-selection path remains the point where missing facts may be materialized.
- `更多月份或租客` exposes the existing multi-item draft capability for the selected receipt inside this same area. One selected receipt is processed at a time because `/transactions/confirm-batch` is atomic per source transaction, not across several receipts. Switching sources while a draft exists requires an explicit discard action.
- `返回流水列表` collapses the allocation area and restores finder filters. `确认分配` uses `/transactions/confirm-batch` without a new accounting path. Successful confirmation refreshes the same workspace/finder context, showing the updated tenant balance and pending list; invalid or stale balances keep the selected source and display the error in the drawer.
- When a receipt was already partially allocated, the card displays the unallocated remainder and existing-use cue. A fully allocated or deferred receipt does not appear in the default pending list. The review service still revalidates live balances and may reject a stale result.
- The optional `记住银行付款人对应的租客` choice stays off unless the user explicitly chooses it in the allocation area. Name clues never pre-authorize it.

## Architecture and boundaries

- Add a read-only finder query/service near the existing transaction list query code, reusing the pending-income status vocabulary, user ownership filter, and active-defer rule. Do not reuse `tenant_id` filtering for unassociated receipts because that filter requires an existing association.
- Render a dedicated finder partial from `/rent-dashboard` when a validated tenant ID query key is present; the selected workspace month comes from existing filters. Extract a small shared URL helper for tenant-row entry and selected-source state.
- Reuse the evidence and draft view-model logic from `transactionMatchReviewForMonthWithOrigin` inside the tenant-first drawer, and keep `confirmRentMatchBatchWithPrepayment` as the write boundary. Use a lookup origin when carrying the workspace month so selecting a candidate remains read-only; show that month as a viewed target even when it has no obligation yet. Avoid nesting two `role=dialog` elements or maintaining two independent draft implementations. Preserve the receipt-first drawer behavior.
- Treat text matching as search/ranking only. No new payer relation, allocation, or auto-match decision is made by the finder.

## Compatibility and risks

- `一键平账` currently creates a synthetic income transaction. The user chose side-by-side tenant-row controls for finding a bank receipt and manual balance. Label them `找银行流水` and `人工平账` (with the current manual-balance form retained), and use short help text to distinguish the sources so users do not record the same payment twice.
- The existing workspace template, styles, and review partial already have unrelated uncommitted edits. Integrate without replacing those changes.
- Use one server query for count and rows with bounded page size; the all-dates option needs pagination. Validate any date-scope and search-field values rather than interpolating SQL fields.
- Reuse existing ownership and source-balance validation at the review and confirmation boundaries. A stale candidate may disappear or have a lower remainder between list and confirmation.
