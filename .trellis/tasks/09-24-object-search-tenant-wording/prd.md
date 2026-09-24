# Object search and tenant wording

## Goal

Make object searches find the relevant tenant and make user-facing rent copy describe tenants and amounts plainly.

## Requirements

- Room management search matches room, property, and tenants associated with the selected month, including each tenant's real name and display alias. Existing status/property/collection filters continue to apply.
- Property management search matches property name/address, rooms, and tenants associated with the selected month, including real names and aliases. Existing property/collection filters continue to apply.
- Both list search fields submit by Enter; a visible search action is available on desktop and mobile. The existing mobile advanced-filter toggle stays distinct from search submission.
- Audit visible workspace wording containing “责任”. Replace jargon where a tenant, rent share, expected amount, month, or payment recipient is clearer. Preserve exact financial meaning and keep domain model/API names unchanged.

## Acceptance Criteria

- [ ] Entering a tenant's real name or alias on room or property management, then pressing Enter, returns the owner-scoped room/property for the displayed month.
- [ ] Room and property name searches still work; month/status/collection filters and sort links remain stable.
- [ ] Desktop/mobile search controls visibly distinguish “搜索” submission from “筛选” expansion.
- [ ] High-visibility labels such as “当前责任人”, “责任记录”, “个人责任”, and “责任月份” read as tenant/rent/amount terms without changing calculations.
- [ ] No other user's tenant or room appears through search.

## Dependency

None. Wording in transaction detail should be reconciled with `09-24-transaction-review-search-ux` during parent integration.
