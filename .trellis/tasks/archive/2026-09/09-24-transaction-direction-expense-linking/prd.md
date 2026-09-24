# Transaction direction clarity and bank expense attribution

## Goal

Make income and expense transactions easy to distinguish, and let landlords attribute a synced bank expense to the relevant property or room with its purpose and invoice in the transaction workflow.

## Deliverables

1. [Transaction direction clarity](../09-24-transaction-direction-clarity/prd.md): move the income/expense filter into the always-visible filter row; label and visually distinguish both directions in the list.
2. [Bank expense attribution](../09-24-bank-expense-attribution/prd.md): provide a drawer to attribute, document, and later edit a bank expense.

## Cross-deliverable acceptance

- [x] A landlord can filter to expenses, identify an unattributed bank expense, open its attribution drawer, save it, and see its updated state without leaving the transaction workflow.
- [x] Existing income rent allocation and manually entered expense flows still work.
- [x] The same bank debit does not appear as a duplicate financial transaction after attribution.

## Confirmed product decision (2026-09-24)

- The user confirmed that a property is required for attributed expenses and a room is optional, so property-wide costs remain possible.
