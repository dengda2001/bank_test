UPDATE manual_expenses AS expense
JOIN payment_transactions AS transaction
  ON transaction.user_id = expense.user_id
 AND transaction.source = 'manual_expense'
 AND transaction.stable_transaction_key = CONCAT('manual_expense:', expense.id)
SET expense.payment_transaction_id = transaction.id
WHERE expense.payment_transaction_id IS NULL;

UPDATE payment_transactions AS transaction
JOIN manual_expenses AS expense
  ON expense.user_id = transaction.user_id
 AND expense.payment_transaction_id = transaction.id
SET transaction.match_status = 'matched'
WHERE transaction.direction = 'expense'
  AND transaction.source = 'manual_expense'
  AND transaction.match_status = 'unmatched';
