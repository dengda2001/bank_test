CREATE TABLE IF NOT EXISTS tenant_prepayments (
  id bigint unsigned NOT NULL AUTO_INCREMENT,
  user_id bigint unsigned NOT NULL,
  tenant_id bigint unsigned NOT NULL,
  payment_transaction_id bigint unsigned NOT NULL,
  original_amount_cents bigint NOT NULL,
  currency varchar(3) NOT NULL,
  operation_id varchar(64) NOT NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY idx_tenant_prepayments_user_id (user_id, id),
  UNIQUE KEY idx_tenant_prepayments_user_operation (user_id, operation_id),
  KEY idx_tenant_prepayments_user_tenant (user_id, tenant_id),
  KEY idx_tenant_prepayments_user_source (user_id, payment_transaction_id),
  CONSTRAINT fk_tenant_prepayments_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_tenant_prepayments_tenant FOREIGN KEY (user_id, tenant_id) REFERENCES tenants(user_id, id) ON DELETE RESTRICT,
  CONSTRAINT fk_tenant_prepayments_source FOREIGN KEY (payment_transaction_id) REFERENCES payment_transactions(id) ON DELETE RESTRICT,
  CONSTRAINT chk_tenant_prepayments_amount CHECK (original_amount_cents > 0)
);

ALTER TABLE payment_allocations
  ADD COLUMN prepayment_id bigint unsigned NULL AFTER rent_obligation_id,
  ADD KEY idx_payment_allocations_user_prepayment (user_id, prepayment_id, status),
  ADD CONSTRAINT fk_payment_allocations_prepayment FOREIGN KEY (user_id, prepayment_id) REFERENCES tenant_prepayments(user_id, id) ON DELETE RESTRICT;
