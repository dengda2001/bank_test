ALTER TABLE manual_expenses
  ADD COLUMN payment_transaction_id bigint unsigned NULL AFTER user_id,
  ADD UNIQUE KEY idx_manual_expenses_user_payment_transaction (user_id, payment_transaction_id),
  ADD CONSTRAINT fk_manual_expenses_payment_transaction FOREIGN KEY (payment_transaction_id) REFERENCES payment_transactions(id) ON DELETE RESTRICT;
