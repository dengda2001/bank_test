# Integration design

The [direction clarity child](../09-24-transaction-direction-clarity/prd.md) changes only the transaction list presentation and filter placement. The [bank expense attribution child](../09-24-bank-expense-attribution/design.md) owns persistent expense linkage, invoice handling, and the expense drawer. Both use the existing `Direction` and `MatchStatus` fields. The expense child follows the direction child so its new action lands in the final desktop and mobile list layout.

Integration review must verify that a linked debit is represented once in `payment_transactions`, once as an expense fact in `manual_expenses`, and once in each relevant financial summary. Income rent matching retains its existing behavior.
