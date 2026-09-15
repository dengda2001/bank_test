ALTER TABLE payment_allocations
  MODIFY COLUMN rent_obligation_id bigint unsigned NULL,
  MODIFY COLUMN tenant_id bigint unsigned NULL,
  ADD COLUMN allocation_kind varchar(32) NOT NULL DEFAULT 'rent' AFTER amount_cents,
  ADD COLUMN operation_id varchar(64) NULL AFTER allocation_kind,
  ADD COLUMN idempotency_key varchar(191) NULL AFTER operation_id,
  ADD COLUMN voided_at timestamp NULL AFTER confirmed_at,
  ADD COLUMN voided_by_user_id bigint unsigned NULL AFTER voided_at,
  ADD COLUMN void_reason varchar(512) NULL AFTER voided_by_user_id,
  DROP INDEX idx_payment_allocations_tx_obligation,
  ADD UNIQUE KEY idx_payment_allocations_user_idempotency (user_id, idempotency_key),
  ADD KEY idx_payment_allocations_user_tx_status (user_id, payment_transaction_id, status),
  ADD CONSTRAINT fk_payment_allocations_voided_by_user FOREIGN KEY (voided_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE rent_obligations
  ADD COLUMN record_status varchar(32) NOT NULL DEFAULT 'active' AFTER status,
  ADD COLUMN voided_at timestamp NULL AFTER record_status,
  ADD COLUMN voided_by_user_id bigint unsigned NULL AFTER voided_at,
  ADD COLUMN void_reason varchar(512) NULL AFTER voided_by_user_id,
  ADD CONSTRAINT fk_rent_obligations_voided_by_user FOREIGN KEY (voided_by_user_id) REFERENCES users(id) ON DELETE SET NULL;
