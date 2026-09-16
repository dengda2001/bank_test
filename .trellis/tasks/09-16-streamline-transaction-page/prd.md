# Streamline transaction page

## Goal

Make the `/billing` transaction page and `/rent-dashboard` easier to scan by
removing technical identifiers, showing the single operator-confirmed rent
month, and hiding non-essential bank-income summary cards.

## Confirmed Facts

- This is a front-end-only change to the server-rendered transaction page in
  `cmd/truelayer-demo/billing_page.go`.
- The API response and backend transaction fields must remain unchanged.
- The confirmed rent month (`FinalPeriodDisplay`) is the only rent-month value
  shown in the transaction row.
- The Dashboard currently renders `待分配金额` and `其他收入` as a separate,
  secondary bank-income summary section.

## Requirements

- Do not render payer ID, internal ID, account ID, provider transaction ID, or
  reference number on `/billing` rows.
- Replace the two rent-month lines (parsed and confirmed) with one `租金月`
  line using only the confirmed rent month. Use `未确认` when it is absent.
- Rename the period filter label from `到账月` to `流水到账月` without changing
  the filter parameter or behavior.
- Do not render the Dashboard's `待分配金额` and `其他收入` cards. Retain their
  calculations and all other dashboard metrics.

## Acceptance Criteria

- [x] A rendered `/billing` transaction row has none of the listed technical
  identifier labels or values.
- [x] A rendered row shows exactly one `租金月` value sourced from the confirmed
  rent month, and never renders `解析租金月`.
- [x] The period filter is labelled `流水到账月` and continues to submit the
  existing `period` value.
- [x] The Dashboard renders neither the `待分配金额` nor `其他收入` cards, while
  retaining the primary rent summary metrics.
- [x] The existing Go test suite passes.

## Out of Scope

- Removing fields from API responses, data models, or backend queries.
- Changing identifiers on other pages or the inputs needed by transaction
  actions.

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
