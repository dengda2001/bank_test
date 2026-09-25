# Match review interaction and allocation feedback

## Goal

Keep the landlord in one review context while finding a tenant, inspecting evidence, and assigning rent from a bank receipt.

## Requirements and acceptance

- [ ] Opening source or historical transaction details from review shows an accessible overlay and preserves unsaved allocation drafts when closed.
- [ ] The defer action removes an eligible source from the home queue. A failure displays the specific server result in a prominent, keyboard-readable floating notice; the action can be retried.
- [ ] Add/validation errors for drafts use the same prominent notice. Suggestions use blue styling.
- [ ] A property selector narrows room choices; a room selector shows the occupants relevant to the transaction or selected rent month. Choosing an occupant only changes the evidence view.
- [ ] Existing unambiguous payer relations preselect the tenant. If a draft includes that tenant, its “remember payer” selector defaults to that tenant. A unique exact official tenant-name match may also default the selector; shared/conflicting/different names leave it blank.
- [ ] Highlighted month copy states whether the month came from an explicit rent reference in the bank description or from the arrival date.
- [ ] A missing monthly bill explains the checked tenant and month, and points to the room occupancy/rent plan. No bill is fabricated by a view-only action.
- [ ] Rent allocations remain bounded by each obligation and the source. The user may explicitly match an overpayment to a tenant: the selected month's rent receives at most its unpaid balance, and the excess becomes a traceable pending prepayment belonging to that tenant.
- [ ] Verify IE26090426926842 and IE26083165993723 against live data if available, recording the conclusion without altering unrelated historical allocations.

## Constraints

- Preserve atomic, idempotent batch confirmation and the existing allocation audit trail.
- The prepayment write contract is owned by `09-24-rent-prepayment-ledger`; the review UI consumes that contract after it is validated.
- Never establish a payer identity from fuzzy or shared names.
- Do not classify excess money automatically; creating a prepayment requires an explicit user choice.
- Preserve keyboard focus, mobile drawer reachability, and existing drafts during lookups.

## Open decision

- The user has selected tenant prepayment for overpaid matching. Future rent application requires an explicit action; refund functionality is deferred.
