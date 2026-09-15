ALTER TABLE cash_receipts
  ADD COLUMN void_operation_id varchar(64) NULL AFTER operation_id;
