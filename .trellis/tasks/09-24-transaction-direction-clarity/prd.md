# Transaction direction clarity

## Goal

Show at a glance whether each transaction is money received or money spent.

## Requirements

- Put the existing income/expense filter in the always-visible filter row on desktop and mobile, with All, Income, and Expense choices.
- Keep the direction choice while changing other filters, sorting, or paging.
- Show an explicit income/expense label for every transaction in the desktop table and mobile card.
- Give income and expense rows distinct, subtle background colors. The text label must remain usable without color.
- Preserve the existing rent allocation controls and status meanings.

## Acceptance criteria

- [x] Exactly one visible direction selector is outside the advanced filter panel and selects the existing `direction` query parameter.
- [x] Income and expense transactions have different row/card backgrounds and visible direction text at desktop and mobile widths.
- [x] Direction filtering and its active choice survive other filter submissions and pagination.
- [x] No new horizontal overflow or loss of mobile touch targets.
