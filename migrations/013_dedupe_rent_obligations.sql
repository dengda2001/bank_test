DROP TABLE IF EXISTS _mig013_keep;

CREATE TABLE _mig013_keep (
  obligation_id bigint unsigned NOT NULL,
  user_id bigint unsigned NOT NULL,
  tenant_id bigint unsigned NOT NULL,
  period_month date NOT NULL,
  PRIMARY KEY (obligation_id),
  UNIQUE KEY uq_mig013_keep_group (user_id, tenant_id, period_month)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO _mig013_keep (obligation_id, user_id, tenant_id, period_month)
SELECT o.id, o.user_id, o.tenant_id, o.period_month
FROM rent_obligations o
WHERE o.rent_charge_id IS NULL
  AND o.id = (
    SELECT cand.id
    FROM rent_obligations cand
    WHERE cand.user_id = o.user_id
      AND cand.tenant_id = o.tenant_id
      AND cand.period_month = o.period_month
      AND cand.rent_charge_id IS NULL
    ORDER BY
      CASE WHEN EXISTS (SELECT 1 FROM payment_allocations pa WHERE pa.rent_obligation_id = cand.id)
             OR EXISTS (SELECT 1 FROM cash_receipts cr WHERE cr.rent_obligation_id = cand.id)
             OR EXISTS (SELECT 1 FROM dunning_send_attempts da WHERE da.rent_obligation_id = cand.id)
           THEN 0 ELSE 1 END,
      CASE WHEN cand.record_status = 'voided' THEN 1 ELSE 0 END,
      CASE WHEN cand.paid_amount_cents > 0 THEN 0 ELSE 1 END,
      cand.id
    LIMIT 1
  );

DELETE pa FROM payment_allocations pa
JOIN rent_obligations o ON o.id = pa.rent_obligation_id
JOIN _mig013_keep k ON k.user_id = o.user_id AND k.tenant_id = o.tenant_id AND k.period_month = o.period_month
JOIN payment_allocations keep
  ON keep.user_id = pa.user_id
 AND keep.idempotency_key = pa.idempotency_key
 AND keep.rent_obligation_id = k.obligation_id
WHERE o.rent_charge_id IS NULL
  AND o.id <> k.obligation_id
  AND pa.idempotency_key IS NOT NULL;

UPDATE payment_allocations pa
JOIN rent_obligations o ON o.id = pa.rent_obligation_id
JOIN _mig013_keep k ON k.user_id = o.user_id AND k.tenant_id = o.tenant_id AND k.period_month = o.period_month
SET pa.rent_obligation_id = k.obligation_id
WHERE o.rent_charge_id IS NULL
  AND o.id <> k.obligation_id;

DELETE cr FROM cash_receipts cr
JOIN rent_obligations o ON o.id = cr.rent_obligation_id
JOIN _mig013_keep k ON k.user_id = o.user_id AND k.tenant_id = o.tenant_id AND k.period_month = o.period_month
JOIN cash_receipts keep
  ON keep.user_id = cr.user_id
 AND keep.rent_obligation_id = k.obligation_id
 AND (keep.receipt_number = cr.receipt_number
   OR keep.idempotency_key = cr.idempotency_key
   OR (keep.payment_transaction_id IS NOT NULL AND keep.payment_transaction_id = cr.payment_transaction_id))
WHERE o.rent_charge_id IS NULL
  AND o.id <> k.obligation_id;

UPDATE cash_receipts cr
JOIN rent_obligations o ON o.id = cr.rent_obligation_id
JOIN _mig013_keep k ON k.user_id = o.user_id AND k.tenant_id = o.tenant_id AND k.period_month = o.period_month
SET cr.rent_obligation_id = k.obligation_id
WHERE o.rent_charge_id IS NULL
  AND o.id <> k.obligation_id;

DELETE doomed FROM dunning_send_attempts doomed
JOIN rent_obligations o ON o.id = doomed.rent_obligation_id
JOIN _mig013_keep k ON k.user_id = o.user_id AND k.tenant_id = o.tenant_id AND k.period_month = o.period_month
JOIN dunning_send_attempts winner
  ON winner.user_id = doomed.user_id
 AND winner.request_key = doomed.request_key
JOIN rent_obligations ow ON ow.id = winner.rent_obligation_id
WHERE o.rent_charge_id IS NULL
  AND o.id <> k.obligation_id
  AND ow.rent_charge_id IS NULL
  AND ow.user_id = o.user_id
  AND ow.tenant_id = o.tenant_id
  AND ow.period_month = o.period_month
  AND (winner.rent_obligation_id = k.obligation_id OR winner.id < doomed.id);

UPDATE dunning_send_attempts da
JOIN rent_obligations o ON o.id = da.rent_obligation_id
JOIN _mig013_keep k ON k.user_id = o.user_id AND k.tenant_id = o.tenant_id AND k.period_month = o.period_month
SET da.rent_obligation_id = k.obligation_id
WHERE o.rent_charge_id IS NULL
  AND o.id <> k.obligation_id;

DELETE o FROM rent_obligations o
JOIN _mig013_keep k ON k.user_id = o.user_id AND k.tenant_id = o.tenant_id AND k.period_month = o.period_month
WHERE o.rent_charge_id IS NULL
  AND o.id <> k.obligation_id;

SET @mig013_year = YEAR(UTC_TIMESTAMP());

SET @mig013_dst_start = DATE_ADD(DATE_SUB(STR_TO_DATE(CONCAT(@mig013_year, '-03-31'), '%Y-%m-%d'), INTERVAL (DAYOFWEEK(STR_TO_DATE(CONCAT(@mig013_year, '-03-31'), '%Y-%m-%d')) - 1) DAY), INTERVAL 1 HOUR);

SET @mig013_dst_end = DATE_ADD(DATE_SUB(STR_TO_DATE(CONCAT(@mig013_year, '-10-31'), '%Y-%m-%d'), INTERVAL (DAYOFWEEK(STR_TO_DATE(CONCAT(@mig013_year, '-10-31'), '%Y-%m-%d')) - 1) DAY), INTERVAL 1 HOUR);

SET @mig013_today = CASE WHEN UTC_TIMESTAMP() >= @mig013_dst_start AND UTC_TIMESTAMP() < @mig013_dst_end THEN DATE(DATE_ADD(UTC_TIMESTAMP(), INTERVAL 1 HOUR)) ELSE DATE(UTC_TIMESTAMP()) END;

UPDATE rent_obligations o
JOIN (
  SELECT o2.id AS obligation_id,
         o2.user_id AS user_id,
         (SELECT COALESCE(SUM(pa.amount_cents), 0)
            FROM payment_allocations pa
           WHERE pa.rent_obligation_id = o2.id
             AND pa.user_id = o2.user_id
             AND pa.status = 'confirmed'
             AND (pa.allocation_kind = 'rent' OR pa.allocation_kind = ''))
       + (SELECT COALESCE(SUM(cr.amount_cents), 0)
            FROM cash_receipts cr
           WHERE cr.rent_obligation_id = o2.id
             AND cr.user_id = o2.user_id
             AND cr.tenant_id = o2.tenant_id
             AND cr.status = 'confirmed'
             AND cr.amount_cents > 0
             AND UPPER(TRIM(cr.currency)) = 'EUR'
             AND UPPER(TRIM(cr.currency)) = UPPER(TRIM(o2.currency))) AS paid_cents
  FROM rent_obligations o2
  WHERE o2.rent_charge_id IS NULL
) p ON p.obligation_id = o.id AND p.user_id = o.user_id
SET o.paid_amount_cents = p.paid_cents,
    o.status = CASE
      WHEN o.status = 'needs_review' THEN 'needs_review'
      WHEN o.record_status = 'voided' THEN 'voided'
      WHEN p.paid_cents >= o.expected_amount_cents THEN 'paid'
      WHEN p.paid_cents > 0 THEN 'partial'
      WHEN o.due_date < @mig013_today THEN 'overdue'
      ELSE 'open'
    END;

ALTER TABLE rent_obligations
  ADD COLUMN lazy_period_month date
    GENERATED ALWAYS AS (CASE WHEN rent_charge_id IS NULL THEN period_month ELSE NULL END) STORED,
  ADD UNIQUE KEY idx_rent_obligations_lazy_tenant_period (user_id, tenant_id, lazy_period_month);

DROP TABLE IF EXISTS _mig013_keep;
