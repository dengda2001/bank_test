CREATE TABLE IF NOT EXISTS properties (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  name varchar(191) NOT NULL,
  address text NULL,
  status varchar(32) NOT NULL DEFAULT 'active',
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_properties_user_status (user_id, status),
  CONSTRAINT fk_properties_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS rooms (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  property_id bigint unsigned NOT NULL,
  room_label varchar(191) NOT NULL,
  status varchar(32) NOT NULL DEFAULT 'active',
  active_from date NOT NULL,
  inactive_from date NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY idx_rooms_user_property_label (user_id, property_id, room_label),
  KEY idx_rooms_user_property_status (user_id, property_id, status),
  CONSTRAINT fk_rooms_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_rooms_property FOREIGN KEY (property_id) REFERENCES properties(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tenancy_agreements (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  room_id bigint unsigned NOT NULL,
  start_date date NOT NULL,
  end_date date NULL,
  monthly_rent_cents bigint NOT NULL,
  currency char(3) NOT NULL DEFAULT 'EUR',
  due_day int NOT NULL,
  status varchar(32) NOT NULL DEFAULT 'active',
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_tenancy_agreements_user_room_dates (user_id, room_id, start_date, end_date),
  CONSTRAINT fk_tenancy_agreements_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_tenancy_agreements_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS agreement_parties (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  agreement_id bigint unsigned NOT NULL,
  tenant_id bigint unsigned NOT NULL,
  responsibility_cents bigint NOT NULL,
  joined_at date NULL,
  left_at date NULL,
  status varchar(32) NOT NULL DEFAULT 'active',
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY idx_agreement_parties_user_agreement_tenant (user_id, agreement_id, tenant_id),
  KEY idx_agreement_parties_user_agreement_status (user_id, agreement_id, status),
  CONSTRAINT fk_agreement_parties_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_agreement_parties_agreement FOREIGN KEY (agreement_id) REFERENCES tenancy_agreements(id) ON DELETE RESTRICT,
  CONSTRAINT fk_agreement_parties_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS rent_charges (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  property_id bigint unsigned NOT NULL,
  room_id bigint unsigned NOT NULL,
  tenancy_agreement_id bigint unsigned NOT NULL,
  period_month date NOT NULL,
  due_date date NOT NULL,
  expected_amount_cents bigint NOT NULL,
  currency char(3) NOT NULL DEFAULT 'EUR',
  record_status varchar(32) NOT NULL DEFAULT 'active',
  property_name_snapshot varchar(191) NULL,
  room_label_snapshot varchar(191) NULL,
  room_address_snapshot text NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY idx_rent_charges_user_room_period (user_id, room_id, period_month),
  KEY idx_rent_charges_user_property_period_status (user_id, property_id, period_month, record_status),
  KEY idx_rent_charges_user_agreement_period (user_id, tenancy_agreement_id, period_month),
  CONSTRAINT fk_rent_charges_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_rent_charges_property FOREIGN KEY (property_id) REFERENCES properties(id) ON DELETE RESTRICT,
  CONSTRAINT fk_rent_charges_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE RESTRICT,
  CONSTRAINT fk_rent_charges_agreement FOREIGN KEY (tenancy_agreement_id) REFERENCES tenancy_agreements(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE rent_obligations
  ADD COLUMN rent_charge_id bigint unsigned NULL AFTER user_id,
  ADD COLUMN tenant_name_snapshot varchar(191) NULL AFTER tenant_id,
  DROP INDEX idx_rent_obligations_user_tenant_period,
  ADD UNIQUE KEY idx_rent_obligations_user_charge_tenant (user_id, rent_charge_id, tenant_id),
  ADD KEY idx_rent_obligations_user_charge (user_id, rent_charge_id),
  ADD CONSTRAINT fk_rent_obligations_charge FOREIGN KEY (rent_charge_id) REFERENCES rent_charges(id) ON DELETE RESTRICT;

ALTER TABLE manual_expenses
  ADD COLUMN property_id bigint unsigned NULL AFTER user_id,
  ADD COLUMN room_id bigint unsigned NULL AFTER property_id,
  ADD COLUMN record_status varchar(32) NOT NULL DEFAULT 'active' AFTER payment_method,
  ADD COLUMN voided_at timestamp NULL AFTER record_status,
  ADD COLUMN voided_by_user_id bigint unsigned NULL AFTER voided_at,
  ADD COLUMN void_reason varchar(512) NULL AFTER voided_by_user_id,
  ADD KEY idx_manual_expenses_user_property_date (user_id, property_id, expense_date),
  ADD KEY idx_manual_expenses_user_room_date (user_id, room_id, expense_date),
  ADD CONSTRAINT fk_manual_expenses_property FOREIGN KEY (property_id) REFERENCES properties(id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_manual_expenses_room FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_manual_expenses_voided_by_user FOREIGN KEY (voided_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE cash_receipts
  ADD COLUMN payment_transaction_id bigint unsigned NULL AFTER user_id,
  ADD UNIQUE KEY idx_cash_receipts_user_payment_transaction (user_id, payment_transaction_id),
  ADD CONSTRAINT fk_cash_receipts_payment_transaction FOREIGN KEY (payment_transaction_id) REFERENCES payment_transactions(id) ON DELETE RESTRICT;
