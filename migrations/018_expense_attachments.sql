CREATE TABLE IF NOT EXISTS expense_attachments (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  expense_id bigint unsigned NOT NULL,
  file_name varchar(255) NOT NULL,
  content_type varchar(127) NOT NULL,
  size_bytes bigint NOT NULL,
  sha256 char(64) NOT NULL,
  file_data longblob NOT NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  removed_at timestamp NULL,
  KEY idx_expense_attachments_owner_expense (user_id, expense_id, removed_at, id),
  CONSTRAINT fk_expense_attachments_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_expense_attachments_expense FOREIGN KEY (expense_id) REFERENCES manual_expenses(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
