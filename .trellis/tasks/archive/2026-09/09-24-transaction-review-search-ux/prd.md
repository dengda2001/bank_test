# Transaction review and search UX

## Goal

Make matching, correction, and deferral follow one understandable flow from list, dashboard, and detail.

## Requirements

- Give “智能建议” a clear green treatment; roommate and date hints use neutral styling, while actual errors retain red. The smart marker clears after selecting another tenant.
- If a confirmed payer name has no close full-name match, split it into words and suggest tenants matching a distinctive word. In the example `Eider Esneir Larios Ospino`, searching `Eider` should surface a tenant named `Eider`. Show all matches as individual name-only buttons under “智能建议”, without a three-result limit or “查看月份” suffix. Suggestions require manual review and never create a payer relation or allocation.
- Switching to a roommate changes the tenant being inspected but does not imply the user manually looked up a month. “正在查看” appears only after an explicit “查看月份” action for the current tenant.
- Each same-room tenant group offers “查看房间”, opening that room's detail page for the group's displayed month.
- Make payer and tenant names in desktop/mobile list rows clickable searches that fill the top keyword field and run the existing query.
- Replace detail's four amount/status cards with one full-width allocation progress panel matching the review drawer's confirmed/pending/remaining language and colors. Directly under the bar, show “租客与入账分配” rows by tenant, month, use, amount, and status, without “责任” wording in that section.
- Detail exposes one visible “处理分配” entry into the shared review drawer and an explicit full-source “撤销匹配” control. Remove the redundant right-side quick-match card and competing single-target rent forms; keep non-rent categorization labeled separately. Keep actions accessible on mobile.
- Fully matched rent sources can open the review drawer to inspect existing shares and revoke one before reallocating. Do not permit an over-allocation.
- Each paid evidence row has a per-share revoke confirmation dialog showing actual source and share details; success closes it and refreshes the drawer without losing ordinary drafts.
- “暂不处理” sits beside “确认分配” and uses a confirmation dialog explaining its home-queue effect and later retrieval through the transaction list pending filter.
- Success and failure feedback is visible after actions.

## Acceptance Criteria

- [ ] The three surfaces use the same rent review interaction for new, partial, and corrected allocations.
- [ ] The detail page has one combined progress and tenant allocation panel; no right-side quick-match card or “关联责任与对象” wording remains.
- [ ] The evidence revoke dialog cancels harmlessly, confirms only the chosen share, then refreshes the month and source balance.
- [ ] Deferral cancel has no effect; confirm removes the source from the home queue and transaction search still finds it.
- [ ] Clicking a payer or tenant name yields a visible populated keyword field and matching list results at desktop and mobile widths.
- [ ] Color and focus behavior match semantic meaning at desktop and mobile widths.
- [ ] Full-name candidates retain priority; word-level fallback finds `Eider`, displays every distinct candidate as a name-only button, does not offer noise from short/common words, and produces no suggestions for inferred or unknown payer names.
- [ ] Roommate selection clears a prior manual month-lookup state while still loading the roommate's relevant month without a misleading “正在查看” marker; a later explicit month lookup sets it correctly.
- [ ] “查看房间” in a same-room group opens the right room and month, on desktop and mobile.

## Dependency

The evidence revoke button depends on `09-24-allocation-share-revocation` for a safe exact-share POST route and allocation ID. Both evidence revoke and deferral confirmation depend on `09-24-shared-danger-confirmation-dialogs` for the global in-app confirmation UI. Other UI changes are independent.
