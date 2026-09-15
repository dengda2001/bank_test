CREATE TABLE IF NOT EXISTS bank_sync_runs (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  provider varchar(64) NOT NULL,
  environment varchar(32) NOT NULL,
  mode varchar(32) NOT NULL,
  requested_from date NOT NULL,
  requested_to timestamp NOT NULL,
  status varchar(32) NOT NULL DEFAULT 'running',
  started_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at timestamp NULL,
  error_message varchar(512) NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_bank_sync_runs_user_started (user_id, started_at),
  KEY idx_bank_sync_runs_user_mode (user_id, mode),
  CONSTRAINT fk_bank_sync_runs_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS bank_sync_run_accounts (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  bank_sync_run_id bigint unsigned NOT NULL,
  account_id varchar(191) NOT NULL,
  account_name varchar(191) NULL,
  requested_from date NOT NULL,
  requested_to timestamp NOT NULL,
  covered_from date NULL,
  covered_to timestamp NULL,
  status varchar(32) NOT NULL DEFAULT 'running',
  transaction_count int NOT NULL DEFAULT 0,
  error_message varchar(512) NULL,
  started_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at timestamp NULL,
  KEY idx_bank_sync_run_accounts_user_run (user_id, bank_sync_run_id),
  KEY idx_bank_sync_run_accounts_user_account (user_id, account_id),
  CONSTRAINT fk_bank_sync_run_accounts_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_bank_sync_run_accounts_run FOREIGN KEY (bank_sync_run_id) REFERENCES bank_sync_runs(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE payment_transactions
  ADD COLUMN parsed_period_month date NULL AFTER payer_name_kind,
  ADD COLUMN parsed_period_source varchar(64) NULL AFTER parsed_period_month,
  ADD COLUMN parsed_period_note varchar(512) NULL AFTER parsed_period_source,
  ADD COLUMN match_reason varchar(512) NULL AFTER parsed_period_note,
  ADD KEY idx_payment_transactions_user_parsed_period (user_id, parsed_period_month),
  ADD KEY idx_payment_transactions_user_transaction_time (user_id, transaction_time);

ALTER TABLE payment_allocations
  ADD COLUMN note varchar(512) NULL AFTER confirmation_source;

CREATE TABLE IF NOT EXISTS payment_transaction_actions (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  payment_transaction_id bigint unsigned NOT NULL,
  action_kind varchar(32) NOT NULL,
  reason varchar(512) NOT NULL,
  operation_id varchar(64) NOT NULL,
  idempotency_key varchar(191) NULL,
  acted_by_user_id bigint unsigned NOT NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY idx_payment_transaction_actions_user_idempotency (user_id, idempotency_key),
  KEY idx_payment_transaction_actions_user_tx (user_id, payment_transaction_id, created_at),
  CONSTRAINT fk_payment_transaction_actions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_payment_transaction_actions_tx FOREIGN KEY (payment_transaction_id) REFERENCES payment_transactions(id) ON DELETE CASCADE,
  CONSTRAINT fk_payment_transaction_actions_actor FOREIGN KEY (acted_by_user_id) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
