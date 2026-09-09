CREATE TABLE IF NOT EXISTS schema_migrations (
  version varchar(191) NOT NULL PRIMARY KEY,
  applied_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS users (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  username varchar(191) NOT NULL,
  password_hash varchar(255) NOT NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY idx_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bank_connections (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  provider varchar(64) NOT NULL,
  environment varchar(32) NOT NULL,
  refresh_token_ciphertext text NOT NULL,
  refresh_token_nonce varchar(255) NOT NULL,
  token_storage_mode varchar(32) NOT NULL,
  saved_at timestamp NULL,
  last_sync_at timestamp NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY idx_bank_connections_user_provider_env (user_id, provider, environment),
  KEY idx_bank_connections_user_id (user_id),
  CONSTRAINT fk_bank_connections_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tenants (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  name varchar(191) NOT NULL,
  payer_id varchar(191) NULL,
  payer_name_hint varchar(191) NULL,
  monthly_rent_cents bigint NOT NULL,
  currency char(3) NOT NULL DEFAULT 'EUR',
  interval_unit varchar(32) NOT NULL DEFAULT 'month',
  interval_count int NOT NULL DEFAULT 1,
  billing_start_date date NOT NULL,
  due_day int NOT NULL,
  rent_start_date date NOT NULL,
  rent_end_date date NULL,
  status varchar(32) NOT NULL DEFAULT 'active',
  room_label varchar(191) NOT NULL,
  room_address text NOT NULL,
  property_hint varchar(191) NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_tenants_user_status (user_id, status),
  UNIQUE KEY idx_tenants_user_payer_id (user_id, payer_id),
  CONSTRAINT fk_tenants_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS rent_obligations (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  tenant_id bigint unsigned NOT NULL,
  period_month date NOT NULL,
  due_date date NOT NULL,
  expected_amount_cents bigint NOT NULL,
  paid_amount_cents bigint NOT NULL DEFAULT 0,
  currency char(3) NOT NULL DEFAULT 'EUR',
  status varchar(32) NOT NULL DEFAULT 'open',
  generated_by varchar(32) NOT NULL DEFAULT 'lazy',
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY idx_rent_obligations_user_tenant_period (user_id, tenant_id, period_month),
  KEY idx_rent_obligations_user_period (user_id, period_month),
  CONSTRAINT fk_rent_obligations_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_rent_obligations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS payment_transactions (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  source varchar(64) NOT NULL,
  source_batch_id varchar(191) NULL,
  provider_transaction_id varchar(191) NULL,
  stable_transaction_key varchar(255) NOT NULL,
  account_id varchar(191) NULL,
  account_name varchar(191) NULL,
  direction varchar(32) NOT NULL,
  amount_cents bigint NOT NULL,
  currency char(3) NOT NULL DEFAULT 'EUR',
  transaction_time timestamp NULL,
  description text NULL,
  reference text NULL,
  payer_id varchar(191) NULL,
  payer_name varchar(191) NULL,
  payer_name_kind varchar(32) NOT NULL DEFAULT 'unknown',
  match_status varchar(32) NOT NULL DEFAULT 'unmatched',
  raw_payload_json json NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY idx_payment_transactions_user_stable_key (user_id, stable_transaction_key),
  KEY idx_payment_transactions_user_direction_time (user_id, direction, transaction_time),
  KEY idx_payment_transactions_user_match_status (user_id, match_status),
  CONSTRAINT fk_payment_transactions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS payment_allocations (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  payment_transaction_id bigint unsigned NOT NULL,
  rent_obligation_id bigint unsigned NOT NULL,
  tenant_id bigint unsigned NOT NULL,
  amount_cents bigint NOT NULL,
  status varchar(32) NOT NULL DEFAULT 'confirmed',
  confirmed_by_user_id bigint unsigned NOT NULL,
  confirmed_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  confirmation_source varchar(32) NOT NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY idx_payment_allocations_tx_obligation (user_id, payment_transaction_id, rent_obligation_id),
  KEY idx_payment_allocations_user_tenant (user_id, tenant_id),
  CONSTRAINT fk_payment_allocations_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_payment_allocations_tx FOREIGN KEY (payment_transaction_id) REFERENCES payment_transactions(id) ON DELETE CASCADE,
  CONSTRAINT fk_payment_allocations_obligation FOREIGN KEY (rent_obligation_id) REFERENCES rent_obligations(id) ON DELETE CASCADE,
  CONSTRAINT fk_payment_allocations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS manual_expenses (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  description text NOT NULL,
  category varchar(191) NOT NULL,
  amount_cents bigint NOT NULL,
  currency char(3) NOT NULL DEFAULT 'EUR',
  expense_date date NOT NULL,
  payment_method varchar(191) NOT NULL,
  room_hint varchar(191) NULL,
  tenant_hint varchar(191) NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_manual_expenses_user_date (user_id, expense_date),
  CONSTRAINT fk_manual_expenses_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
