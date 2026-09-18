ALTER TABLE payment_transactions
  ADD COLUMN manual_adjustment_reason varchar(512) NULL AFTER match_reason;

ALTER TABLE manual_expenses
  ADD COLUMN invoice_url varchar(2048) NULL AFTER tenant_hint;

ALTER TABLE properties
  ADD COLUMN inactive_from date NULL AFTER status;
