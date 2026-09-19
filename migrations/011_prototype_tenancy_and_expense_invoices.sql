ALTER TABLE tenancy_agreements
  ADD COLUMN contract_date date NULL AFTER room_id,
  ADD COLUMN move_in_date date NULL AFTER contract_date;

ALTER TABLE properties
  ADD COLUMN city_region varchar(191) NULL AFTER name,
  ADD COLUMN timezone varchar(64) NOT NULL DEFAULT 'Europe/Dublin' AFTER address,
  ADD COLUMN notes text NULL AFTER timezone;

ALTER TABLE rooms
  ADD COLUMN room_type varchar(64) NULL AFTER room_label,
  ADD COLUMN capacity int NOT NULL DEFAULT 1 AFTER room_type,
  ADD COLUMN notes text NULL AFTER capacity;

UPDATE tenancy_agreements
SET contract_date = start_date,
    move_in_date = start_date
WHERE contract_date IS NULL OR move_in_date IS NULL;

CREATE TABLE IF NOT EXISTS manual_expense_invoices (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  expense_id bigint unsigned NOT NULL,
  invoice_number varchar(191) NOT NULL,
  vendor varchar(191) NOT NULL,
  invoice_date date NOT NULL,
  amount_cents bigint NOT NULL,
  currency char(3) NOT NULL,
  file_name varchar(255) NOT NULL,
  content_type varchar(64) NOT NULL,
  sha256 char(64) NOT NULL,
  file_data longblob NOT NULL,
  note text NULL,
  is_current tinyint(1) NOT NULL DEFAULT 1,
  replaced_at timestamp NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_expense_invoices_user_expense_current (user_id, expense_id, is_current, id),
  CONSTRAINT fk_expense_invoices_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_expense_invoices_expense FOREIGN KEY (expense_id) REFERENCES manual_expenses(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
