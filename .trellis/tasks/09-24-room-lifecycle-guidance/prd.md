# Room succession and asset guidance

## Goal

Make room tenant changes and asset lifecycle rules understandable without damaging financial history.

## Requirements and acceptance

- [ ] A room can be set to a new tenant effective September while the former tenant remains recorded through August; previously paid months remain unchanged.
- [ ] The room rent-plan UI explains the month-boundary replacement path and distinguishes it from ending occupancy for a vacancy.
- [ ] Entering zero monthly rent produces a specific message that the monthly amount must be greater than zero, with a path to end occupancy if the room is vacant.
- [ ] The property/room “删除” action performs a soft delete, preserving the asset and all historical rent, payment, and expense links. It hides the asset from daily lists and offers no restore control.
- [ ] An active rent plan must be ended from a chosen month before its room can be soft-deleted; a property with any such room must guide the user through those rooms first. Soft-deleted assets disappear from default daily lists but remain accessible from historical records.
- [ ] Check the live Arslan Arshad room plan and the September successor if the local database contains them, reporting the actual state.

## Constraints

- Preserve financial history and room-plan locked-facts checks. Replace physical deletion guards with a safe soft-delete contract.
- Do not move Arslan Arshad's July/August charges or payments to the new tenant.
- Current deactivation does not stop an active rent plan; the new stop-use flow must not leave new rent bills accruing silently.
