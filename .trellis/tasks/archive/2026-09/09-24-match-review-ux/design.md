# Design: match review interaction and feedback

## Boundaries and dependencies

The existing drawer remains the single rent-allocation surface for dashboard, list, and transaction detail. This child consumes the explicit prepayment contract from `09-24-rent-prepayment-ledger` after its service/migration gate passes. It does not change automatic identity or month matching rules.

## Detail overlay and feedback

- A review-detail overlay opens from source, evidence, or same-payer history links. Load an owner-scoped detail fragment or reuse the existing detail read model; never navigate away or replace the review form. Escape/backdrop closes the top overlay and returns focus to its trigger. Fetch failure shows the shared review notice.
- Add one floating notice within the review backdrop, large enough to read and announced with `role=alert`. All add/validation/defer/revoke/network errors use it. Keep field-level validity and a concise inline anchor where useful, but the floating notice is the primary feedback.
- The defer handler returns a distinguishable error code or safe response body. The client reads the actual result instead of replacing every failure with one generic sentence; it re-enables the button. A successful defer navigates to the same month dashboard with the queue refreshed. Add a server test for eligible, already-deferred, and stale/non-pending sources.
- Smart suggestion surfaces use explicit blue tokens local to the review CSS, while destructive/errors retain the existing danger color.

## Property/room/tenant lookup

Load owner-scoped property, room, and occupant options for the review month. The property selector narrows room options; the room selector narrows tenant options without automatically selecting one. Month changes refresh the occupant context. A tenant selected through this path uses the existing evidence lookup URL. Occupants can include the relevant historical month, not only today's active tenants. The backend still validates every chosen tenant and bill.

## Identity and month evidence

- Keep `decideStrictRentMatch` as the source of unambiguous saved payer identity. Also allow a unique exact official tenant-name match to preselect the remembered payer when that tenant is included in the draft. Fuzzy suggestions and shared/conflicting names never preselect or save a payer relation.
- Rebuild `remember_tenant_id` after draft changes using a server-provided eligible candidate ID, preserving an explicit user selection. Do not override a user-cleared value on a later render.
- Carry `transactionPeriodEvidence.Explicit` into each month row. Render “描述明确提及租金月份” only for explicit rent references, and “根据转账日期推测” for date-based suggestions. An explicit manual month lookup keeps its “正在查看” label.
- Missing-bill copy names the tenant/month and distinguishes no active room plan from an unmaterialized bill where the service can determine that difference. Offer room-plan navigation only when its owner-scoped room is known; otherwise point to tenant/room selection.

## Overpayment interaction

The draft total remains capped by bill balances. After a valid rent draft, if bank cash remains, show an explicit “将剩余 €X 记为该租客预收款” choice and require a specific tenant. The review shows the split before confirmation. The prepayment service rechecks source balance and tenant ownership under lock. A completed split clears the source remainder, while later credit application remains a separate tenant/obligation action.

## Verification and risks

Existing session drafts must survive tenant/property/room lookup, history pagination, detail overlay, and failed saves. Browser-check focus, 390px viewport, and source/history detail interactions. The largest risks are nested overlay focus handling, stale lookup responses, and accidentally treating a fuzzy name as identity; add targeted tests for each.
