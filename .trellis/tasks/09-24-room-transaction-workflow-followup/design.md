# Design: room and transaction workflow follow-up

## Boundaries and task map

This parent task owns integration and acceptance. The room setup child is independent. The allocation-share child establishes the server contract used by the transaction review/search child; the latter may build its independent color, search, detail, and deferral UI before that contract lands. Integrate and validate all three before completion.

| Child task | Deliverable | Dependency |
| --- | --- | --- |
| `09-24-room-tenant-setup-corrections` | Room-context tenant drawer; selected-only occupancy warning | None |
| `09-24-allocation-share-revocation` | Exact-share revoke service, route, audit, projection | None |
| `09-24-shared-danger-confirmation-dialogs` | Global red second-confirmation component and migration of native confirms | None |
| `09-24-login-visual-alignment` | Login palette and responsive form alignment | None |
| `09-24-object-search-tenant-wording` | Room/property tenant search and user-facing terminology | None; coordinate transaction detail copy with transaction UI child |
| `09-24-transaction-review-search-ux` | Review/detail/list UI, evidence confirmation, deferral confirmation | Evidence revoke calls the allocation-share route; dialogs use the shared confirmation component |

## Room flow

Reuse the existing `tenant-form-drawer` on the room detail page. Opening “新增租客” uses a room URL with a `tenant_add=1` query; the room handler loads the same tenant assignment options as the tenant page and validates that the room belongs to the signed-in user and the property. Only the tenant drawer renders while this query is active; the rent-plan editor can be restored on cancel/success. Room creation’s “保存并设置租客” redirects to the newly created room and opens that drawer. The form POST still uses `/tenants`; its return target is a validated room URL so both success and validation errors return to the room context. Existing tenant-list entry points continue to work.

The occupancy-availability script continues disabling conflicting options but builds the warning from the selected option in each active selector. The room-create form checks its “已有租客” radio before showing any warning. Remove the server-rendered list of all conflicting candidates. The service retains its occupancy conflict validation to protect direct POSTs.

## Transaction review and detail flow

One review drawer owns rent allocation on the dashboard, transaction list, and transaction detail. The detail view exposes an obvious “处理分配” action that opens the same drawer and an explicit “撤销匹配” action for the existing full-source revoke preview. Remove the detail page's competing single-target rent confirmation/rematch controls and right-side quick-match card. Keep deposit and other-income classification separate, but make its label and help text clearly about non-rent uses. In the list, replace the rent-only “修改匹配” form with a link to the same review drawer where applicable; the full-source revoke preview remains available.

The detail page begins with one full-width “流水分配” panel. It reuses the review drawer's progress language and colors: confirmed amount, pending amount (zero outside a draft review), and remaining amount, with total source amount alongside. Immediately beneath, “租客与入账分配” lists each effective share by tenant, month, use, amount, and status; voided entries can remain in a secondary history view. No “责任” label appears in this panel. The visible “处理分配” action sits in the panel header so it remains reachable on mobile even where page-head actions are hidden. Bank-original facts and action history remain in their own sections below; remove the empty right rail after deleting the quick-match card.

The review service must render already fully allocated income sources in a read-only correction state, with existing shares and month evidence visible. Its add/confirm controls stay disabled until a share is revoked and funds are available. Partial and unmatched sources keep the existing batch allocation behavior. The detail handler parses and validates the same review query as the list handler, then renders the shared review partial with detail-aware close/return URLs. Existing `confirm-batch` remains the only rent allocation write path. After successful save, return to the originating surface with visible success or actionable failure; preserve drafts after failed saves and ordinary lookups.

Each evidence row represents an effective `payment_allocation`, so it carries the allocation ID as well as source transaction ID, tenant, month, amount, payer, date, and description. “撤销本份分配” opens a confirmation dialog showing those facts. The dialog POST sends only the allocation ID, optional expected source ID, a request key, and a safe return URL. The server derives tenant and obligation from its own rows. On success, close the dialog by reloading the current review URL, refresh the affected month and source balance, and keep unsaved draft items. If the revoked allocation belongs to another source, that source’s newly freed balance becomes pending independently of the current review source.

The “暂不处理” action moves next to “确认分配” in the drawer footer and opens a separate confirmation dialog. The message explicitly says the source leaves the home queue and can be found in the transaction list's pending filter. Cancel has no effect; confirm uses the existing `/transactions/defer` action. The option appears only for eligible pending sources. A source that was deferred and later allocated already has its deferral cleared by the existing allocation path.

## Shared strong confirmation

Replace the shared shell's native `window.confirm` handler and the duplicate bills/dunning page handlers with one global workspace confirmation component. A native HTML `<dialog>` styled by the workspace design tokens gives a red danger heading, target-specific impact text, neutral cancel button, and explicit red confirm button. The dialog is an in-app element, not a browser prompt. The component supports both declarative `data-confirm` form submitters and an imperative call for the transaction review's dynamic evidence/deferral details. It uses `textContent` for untrusted labels, returns focus to the initiating control on cancel, supports Escape and mobile sizing, and never relies on color alone.

The delegated submit handler checks `event.submitter` rather than any matching button within the form. After confirmation, it calls `requestSubmit(originalSubmitter)` with a one-shot bypass so validation, `formaction`, name/value, and external `form=` buttons are preserved without a double-submit. Cancel does not mutate or navigate. Replace existing browser confirmation on property/room/tenant delete, debt settlement, and bulk reminder send. Add the same dialog for payer-relation removal and end-occupancy submission, which currently mutate committed data without a second step. Unsaved row removal remains immediate. Cash-receipt void and full-source revoke retain their existing dedicated detail/confirmation pages, without an extra third confirmation; their visible destructive styling should remain consistent. Search the workspace for remaining `confirm()`/`alert()` calls before completion.

## Login, object search, and wording

The unauthenticated login template currently embeds a blue palette independent of the green workspace tokens. Reuse the workspace color/type/control language in a compact sign-in layout, keep the POST route and error behavior, and remove configuration-variable text from the product screen. Static styles are reachable before login.

Room list filtering already scans displayed tenant names but can miss a real name when a display alias is used. Property list filtering scans only property identity/address. Build owner-scoped search text from active room-plan membership at the selected month, including room label and tenant real name/alias, and feed both filters without adding one query per result. Add a real search submit button to each object list form and rename the mobile advanced-filter toggle “筛选”; Enter submits the same GET form with existing filters. No cross-account search data is exposed.

The copy audit changes visible labels and messages, not backend types or stored values. Examples: “当前责任人” → “入住租客”, “责任记录” → “租客租金记录”, “个人责任” → “个人月租” or “本月应付”, “责任月份” → “租金月份”. Match each phrase to its specific amount and recipient so replacing jargon does not erase who paid versus whose rent was covered. Reconcile the transaction detail wording with its dedicated UI child.

Payer and tenant names in desktop rows and mobile cards become accessible search controls. Activation sets the existing `payer` query parameter, resets pagination, runs the ordinary GET search, and displays that keyword in the top search field. The mobile layout must reveal that field when searching so the result is understandable and editable. Existing filters should remain where compatible.

## Exact-share revoke contract

Add a dedicated POST action, separate from the existing full-source `/transactions/revoke`, for one confirmed rent allocation. The service locks the owner-scoped bank source and target allocation, rejects an allocation owned by another user, already voided, or of another kind, locks the associated rent obligation/room with the existing helper, and conditionally voids exactly that row. It recomputes the affected obligation’s paid amount and status, writes an append-only transaction action linked by operation ID and idempotency key, and reprojects the bank source’s matched/partial/unmatched state using all remaining effective allocations. It must check affected-row count so a race cannot report a false success. Source amount and bank record remain untouched. Other effective allocations and obligations remain untouched. Existing full-source revoke semantics remain unchanged.

The dashboard queue already selects income with pending match status and excludes current deferrals. A fully allocated source whose single share is revoked becomes unmatched; a split source becomes partial. Both re-enter the selected month’s home manual queue when otherwise eligible. Source status and obligation totals are recalculated in the same DB transaction, so the queue never sees a half-updated state.

## Visual semantics and compatibility

Use the shared green accent for the prominent “智能建议” badge. Roommate and suggested-month panels use neutral borders/backgrounds, with green reserved for selected/confirmed meaning; red remains for validation or destructive confirmation. The smart marker must disappear when the chosen tenant differs from the suggestion, including after lookup/navigation. Preserve keyboard focus, accessible dialog labels, and mobile layout. No schema migration is expected; new action kind uses the existing action table.

`manualTenantSuggestions` already requires a confirmed bank payer name and ranks close full-name matches for display only. Keep that first pass. Only when it finds none, tokenize the payer name by whitespace/name punctuation, ignore short or generic tokens, then rank tenants by exact distinctive-token match before prefix/substring match against normalized tenant name and display alias. Deduplicate and show all matching tenants, in deterministic rank/name order, as one name-only button each in the intelligent-suggestion area. No result cap and no “查看月份” button suffix. Context beside the buttons may explain that a payer-name word produced the suggestions. This fallback never changes automatic matching, selected tenant, payer relationships, or allocation; selecting a suggestion only navigates to that tenant's review. The “智能建议” marker is tied to the still-active suggestion selection so changing to an unrelated tenant clears it after server lookup as well as immediately in the client.

The current roommate link builder sets `match_month=group.Period`; `transactionMatchReviewForMonth` interprets any `match_month` as an explicit month lookup and marks that card `Viewed`, rendered as “正在查看”. Keep the roommate month as a data-loading context because requested months trigger monthly-fact generation, but distinguish its origin from an explicit “查看月份” form submission. Only the explicit lookup sets `Viewed`; a roommate tenant switch loads the relevant month without the badge. Clear any previous explicit-lookup marker when switching tenants, and preserve unsaved allocations.

Each `transactionReviewRoommateGroup` already derives from an owner-scoped active room plan and includes a room ID in the query result. Expose a server-built `/rooms/{id}?period={group.Period}` URL on the group and render a “查看房间” link beside its property/room/month summary. The existing room detail handler checks ownership again. The link is ordinary same-tab navigation; source-keyed review drafts remain in session storage if the user returns.

## Operational notes

Do not commit pre-existing unrelated worktree edits. Review the diff before commit and stage only this task’s files or hunks. If integration reveals a conflict in files already changed by another in-progress task, reconcile against current worktree content instead of restoring it. The share revoke is audit-preserving and cannot be rolled back by deleting history; correction is a subsequent allocation through the review drawer.
