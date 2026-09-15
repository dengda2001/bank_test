ALTER TABLE tenants
  ADD COLUMN display_alias varchar(191) NULL AFTER name,
  ADD COLUMN email varchar(191) NULL AFTER display_alias,
  DROP INDEX idx_tenants_user_payer_id,
  ADD KEY idx_tenants_user_payer_id (user_id, payer_id),
  ADD KEY idx_tenants_user_alias (user_id, display_alias);

CREATE TABLE IF NOT EXISTS tenant_payers (
  id bigint unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id bigint unsigned NOT NULL,
  tenant_id bigint unsigned NOT NULL,
  payer_id varchar(191) NULL,
  payer_name_original varchar(191) NOT NULL,
  payer_name_normalized varchar(191) NOT NULL,
  source varchar(32) NOT NULL DEFAULT 'manual',
  last_matched_at timestamp NULL,
  removed_at timestamp NULL,
  removed_by_user_id bigint unsigned NULL,
  created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_tenant_payers_user_tenant_active (user_id, tenant_id, removed_at),
  KEY idx_tenant_payers_user_payer_id (user_id, payer_id),
  KEY idx_tenant_payers_user_payer_name (user_id, payer_name_normalized),
  CONSTRAINT fk_tenant_payers_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_tenant_payers_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CONSTRAINT fk_tenant_payers_removed_by FOREIGN KEY (removed_by_user_id) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO tenant_payers (
  user_id, tenant_id, payer_id, payer_name_original, payer_name_normalized, source
)
SELECT t.user_id,
       t.id,
       NULLIF(TRIM(t.payer_id), ''),
       COALESCE(NULLIF(TRIM(t.payer_name_hint), ''), t.name),
       LOWER(TRIM(COALESCE(NULLIF(t.payer_name_hint, ''), t.name))),
       'legacy_tenant'
FROM tenants AS t
WHERE NOT EXISTS (
  SELECT 1
  FROM tenant_payers AS tp
  WHERE tp.user_id = t.user_id
    AND tp.tenant_id = t.id
    AND tp.removed_at IS NULL
    AND (
      (t.payer_id IS NOT NULL AND t.payer_id <> '' AND tp.payer_id = t.payer_id)
      OR (t.payer_name_hint IS NOT NULL AND t.payer_name_hint <> '' AND tp.payer_name_normalized = LOWER(TRIM(t.payer_name_hint)))
    )
);
