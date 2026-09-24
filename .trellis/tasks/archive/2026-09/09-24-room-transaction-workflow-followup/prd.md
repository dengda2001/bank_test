# Room and transaction workflow follow-up

## Goal

Make room setup and bank transaction correction understandable and usable without detours or misleading status cues.

## Task map

- `09-24-room-tenant-setup-corrections`: room-context tenant creation and selected-only occupancy warning.
- `09-24-allocation-share-revocation`: audit-preserving revoke of exactly one tenant-month allocation.
- `09-24-shared-danger-confirmation-dialogs`: one consistent red in-app confirmation dialog for high-impact actions; removes browser-native confirmation.
- `09-24-login-visual-alignment`: password login page uses the workspace's visual language.
- `09-24-object-search-tenant-wording`: owner-scoped room/property tenant search and user-facing tenant/rent terminology.
- `09-24-transaction-review-search-ux`: review/detail/list UI, confirmations, colors, and name search. Its evidence revoke UI depends on the exact-share route; its confirmation dialogs use the shared component.

## Confirmed facts

- The room-create form currently prints every tenant with a room conflict before a tenant is selected; its availability script also collects every conflicting option.
- Creating a tenant from room detail opens `/tenants?add=1`, whose drawer is on the tenant-list page. A room-bound return path already exists for the saved result.
- Transaction detail uses a single-target rematch form, a separate rent-confirmation form, and a separate allocation form. The review drawer supports multi-tenant and multi-month rent allocation.
- Current transaction revoke voids every effective allocation on the source. The existing confirmation UI is a separate page. Month evidence rows have no revoke action.
- The dashboard's manual queue excludes a transaction whose latest action is `defer`; it still appears in transaction search.
- Login currently uses a standalone blue inline palette while the workspace accent is green.
- Room search currently checks displayed tenant names, which can be aliases; property search does not inspect room or tenant names. Both object list forms use a “搜索” button as a filter disclosure rather than a true submit control.
- “责任” appears throughout visible room, property, tenant, bill, and transaction copy; backend obligation terminology is a separate domain contract.

## Requirements

1. Use green for the prominent “智能建议” indicator. Use neutral or positive styling for roommate information and month hints; red remains for genuine errors. If confirmed bank payer name has no exact or close full-name tenant match, split it into name words and suggest tenants matching a distinctive word (for example, `Eider Esneir Larios Ospino` should suggest a tenant named `Eider`). Show every matching tenant in the intelligent-suggestion area, one name-only button per tenant, with no result cap or “查看月份” suffix. These are manual review suggestions only. The same-room tenant group offers a “查看房间” button to open that room's detail at the displayed month. Switching to a roommate must not mark a month “正在查看” unless the user explicitly used the month lookup for that tenant.
2. Start adding a tenant from the room page in a room-context drawer and return to that room after save or cancel, without navigating to the tenant list.
3. In room creation and occupancy selection, display an unbind warning only for the selected tenant when their existing occupancy conflicts with the chosen month and room. Keep unavailable choices protected on the server.
4. In the transaction list, clicking a payer or tenant name fills the existing keyword search and submits it. Preserve the page's existing search behavior and accessible keyboard interaction.
5. Replace transaction detail's four top metric cards with the same allocated/pending/remaining progress view used in the match drawer. Place a tenant-centered allocation list directly beneath that progress view, named “租客与入账分配” and with no “责任” wording in that section. Remove the redundant right-side quick-match card. Make detail offer a visible revoke action and one “处理分配” entry into the same review flow used for new, partial, and corrected rent allocations; keep the action accessible on mobile.
6. Add a revoke action to each effective paid-transaction evidence row in the review drawer. Revoke only that tenant-month allocation share, preserving every other share of the same bank source. Show the source transaction and exact affected share in a confirmation dialog. After confirmation, close the dialog, refresh the affected month's evidence/balance, and return freed source balance to the manual-review queue according to its effective status.
7. Move “暂不处理” beside “确认分配” in the review drawer. Confirm the action with copy that says it removes the item from the home queue and can later be found in transaction search under pending status.
8. Preserve audit records and account-scoped validation for all allocation or deferral changes. Preserve unsaved review drafts across ordinary tenant/month lookup and failed saves.
9. Replace browser-native strong confirmations throughout the workspace with one consistent red in-app second-confirmation dialog. Cover existing confirmed deletion, settlement, and batch-send actions, and the newly requested revoke/deferral actions. Include other committed destructive controls found in the audit, such as removing a payer relation or ending occupancy; do not interrupt removal of unsaved form rows. Preserve existing dedicated review pages that already provide a separate confirmation step.
10. Align the username/password login page with the workspace's green visual language and responsive control styles, preserving authentication behavior and accessible feedback.
11. In both room and property management, search the selected month's related tenants by real name or alias as well as room/property terms, and submit search by Enter or a visible button. Retain existing filters and owner scope.
12. Review user-facing occurrences of “责任” across the workspace and replace jargon with tenant, rent share, expected amount, or month wording where clearer. Keep the underlying financial distinctions and backend domain model intact.

## Acceptance criteria

- [ ] Smart suggestion, roommate block, and month hint use the intended semantic colors on desktop and mobile; suggestion disappears when a different tenant is selected.
- [ ] When full-name matching misses, the review drawer offers every relevant word-level candidate from the payer name as a separate name-only button; clicking a button opens that tenant's month review without auto matching or remembering a payer relation. Inferred/unknown payer text does not create suggestions.
- [ ] Clicking a roommate switches the tenant and loads the relevant month cards without an inherited “正在查看” badge; using “查看月份” for that tenant marks only the month explicitly requested.
- [ ] Each displayed same-room group has a “查看房间” control that navigates to the correct owner-scoped room detail and displayed month.
- [ ] Room-context tenant creation does not show the tenant-list page during the flow and ends on the originating room.
- [ ] A room with three conflicting available tenants shows no warning before selection, and only the chosen conflict appears after selection.
- [ ] Clicking payer or tenant names on desktop and mobile produces the same search result as entering that name manually.
- [ ] Transaction detail exposes a revoke control and routes rent work through the same review interaction used from the list/dashboard.
- [ ] Detail shows a full-width allocation progress panel with the tenant/month/amount list immediately below; the old right-side quick-match card and “关联责任与对象” wording are gone.
- [ ] The evidence revoke dialog lists the actual source and affected rent allocation, requires a second confirmation, and refreshes the open review drawer after success; other shares of the source stay matched.
- [ ] Revocation updates obligation totals and the source's effective status without deleting the bank record or audit history; an eligible freed source appears in the dashboard manual queue.
- [ ] Deferral confirmation states its queue effect; cancel changes nothing, confirm removes the eligible source from the home queue while transaction search can still find it.
- [ ] No workspace strong-confirmation action invokes the browser's `confirm()` or `alert()`. All covered destructive submissions show a red in-app dialog with target/impact, cancel and explicit confirm; cancel submits nothing, confirm submits once with the original form values and button semantics on desktop and mobile.
- [ ] Login visually matches the workspace at desktop/mobile widths and still supports Enter, autofill, invalid-credential feedback, and keyboard focus.
- [ ] Room and property management search find the selected month's related tenant by real name or alias on Enter; property/room name searches, filters, sorting, and owner scope still work.
- [ ] Rendered workspace labels use tenant and rent terms in place of ambiguous “责任” copy where the meaning is preserved.
- [ ] Focused Go/MySQL tests and desktop/mobile browser checks cover the changed cross-layer flows; `go test ./...` and `go vet ./...` pass.
- [ ] Commit and push the completed changes, as authorized earlier in the conversation.

## Confirmed product decision

- An evidence-row revoke removes only the clicked tenant-month share of a source. All other tenant-month shares remain effective.

## Out of scope

- Automatic redistribution of a revoked share to other tenants or months.
- Changing the definition of bank source amount, rent responsibility, or transaction matching status.
