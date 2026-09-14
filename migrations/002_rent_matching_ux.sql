ALTER TABLE payment_transactions
  ADD COLUMN matched_tenant_id bigint unsigned NULL AFTER payer_name_kind,
  ADD KEY idx_payment_transactions_user_tenant (user_id, matched_tenant_id),
  ADD CONSTRAINT fk_payment_transactions_tenant FOREIGN KEY (matched_tenant_id) REFERENCES tenants(id) ON DELETE SET NULL;
