# Transaction list and dashboard clarity

## Goal

Make transaction direction and the pending queue legible and numerically consistent.

## Requirements and acceptance

- [ ] Desktop transaction rows no longer use direction-colored backgrounds. A separate column contains a green income icon or red expense icon with an accessible label.
- [ ] Mobile cards likewise avoid direction-colored backgrounds and retain a visible and accessible direction marker.
- [ ] Home pending count and the destination list represent the same eligible income set for the selected period. A deferred transaction leaves the home queue but remains findable in transaction history with clear labeling.
- [ ] The paid-rent metric card contains an in-card collection progress bar and a percentage beside it at desktop and mobile widths.
- [ ] Existing filters, sorting, pagination, and table readability remain intact.

## Constraints

- Direction and pending status are display/filter concepts; do not rewrite bank transactions or allocations.
- The list may still expose unmatched expenses through its general filters, but they must not inflate a home income queue count.
