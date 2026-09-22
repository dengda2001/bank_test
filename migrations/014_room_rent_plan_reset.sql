ALTER TABLE tenancy_agreements
  DROP FOREIGN KEY fk_tenancy_agreements_user,
  DROP FOREIGN KEY fk_tenancy_agreements_room,
  DROP INDEX idx_tenancy_agreements_user_room_dates;
ALTER TABLE tenancy_agreements
  RENAME TO room_rent_plans,
  CHANGE COLUMN start_date effective_from_month date NOT NULL,
  CHANGE COLUMN end_date effective_to_month date NULL,
  DROP COLUMN contract_date,
  DROP COLUMN move_in_date,
  DROP COLUMN status,
  ADD UNIQUE KEY uq_room_rent_plans_user_room_from (user_id, room_id, effective_from_month),
  ADD UNIQUE KEY uq_room_rent_plans_user_id (user_id, id),
  ADD UNIQUE KEY uq_room_rent_plans_user_room_id (user_id, room_id, id),
  ADD KEY idx_room_rent_plans_user_room_interval (user_id, room_id, effective_from_month, effective_to_month),
  ADD CONSTRAINT fk_room_rent_plans_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  ADD CONSTRAINT chk_room_rent_plans_month_interval CHECK ((effective_to_month IS NULL OR effective_to_month >= effective_from_month) AND DAYOFMONTH(effective_from_month) = 1 AND (effective_to_month IS NULL OR DAYOFMONTH(effective_to_month) = 1)),
  ADD CONSTRAINT chk_room_rent_plans_monthly_rent CHECK (monthly_rent_cents > 0),
  ADD CONSTRAINT chk_room_rent_plans_due_day CHECK (due_day BETWEEN 1 AND 31);

ALTER TABLE properties
  DROP FOREIGN KEY fk_properties_user,
  DROP INDEX idx_properties_user_status,
  DROP COLUMN inactive_from;
ALTER TABLE properties
  ADD UNIQUE KEY uq_properties_user_id (user_id, id),
  ADD KEY idx_properties_user_status (user_id, status),
  ADD CONSTRAINT fk_properties_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE rooms
  DROP FOREIGN KEY fk_rooms_user,
  DROP FOREIGN KEY fk_rooms_property,
  DROP INDEX idx_rooms_user_property_label,
  DROP INDEX idx_rooms_user_property_status,
  DROP COLUMN active_from,
  DROP COLUMN inactive_from,
  DROP COLUMN monthly_rent_cents,
  DROP COLUMN due_day;
ALTER TABLE rooms
  ADD COLUMN rent_plan_version bigint unsigned NOT NULL DEFAULT 0 AFTER status,
  ADD UNIQUE KEY uq_rooms_user_id (user_id, id),
  ADD UNIQUE KEY uq_rooms_user_id_property (user_id, id, property_id),
  ADD UNIQUE KEY idx_rooms_user_property_label (user_id, property_id, room_label),
  ADD KEY idx_rooms_user_property_status (user_id, property_id, status),
  ADD CONSTRAINT fk_rooms_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_rooms_property FOREIGN KEY (user_id, property_id) REFERENCES properties(user_id, id) ON DELETE RESTRICT;

ALTER TABLE room_rent_plans
  ADD CONSTRAINT fk_room_rent_plans_room FOREIGN KEY (user_id, room_id) REFERENCES rooms(user_id, id) ON DELETE RESTRICT;

ALTER TABLE tenants
  DROP FOREIGN KEY fk_tenants_user,
  DROP INDEX idx_tenants_user_status,
  DROP INDEX idx_tenants_user_payer_id,
  DROP INDEX idx_tenants_user_alias,
  DROP COLUMN payer_id,
  DROP COLUMN payer_name_hint,
  DROP COLUMN monthly_rent_cents,
  DROP COLUMN currency,
  DROP COLUMN interval_unit,
  DROP COLUMN interval_count,
  DROP COLUMN billing_start_date,
  DROP COLUMN due_day,
  DROP COLUMN rent_start_date,
  DROP COLUMN rent_end_date,
  DROP COLUMN room_label,
  DROP COLUMN room_address,
  DROP COLUMN property_hint;
ALTER TABLE tenants
  ADD UNIQUE KEY uq_tenants_user_id (user_id, id),
  ADD KEY idx_tenants_user_status (user_id, status),
  ADD KEY idx_tenants_user_alias (user_id, display_alias),
  ADD CONSTRAINT fk_tenants_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE tenant_payers
  DROP FOREIGN KEY fk_tenant_payers_tenant;
ALTER TABLE tenant_payers
  ADD CONSTRAINT fk_tenant_payers_tenant FOREIGN KEY (user_id, tenant_id) REFERENCES tenants(user_id, id) ON DELETE CASCADE;

ALTER TABLE agreement_parties
  DROP FOREIGN KEY fk_agreement_parties_user,
  DROP FOREIGN KEY fk_agreement_parties_agreement,
  DROP FOREIGN KEY fk_agreement_parties_tenant,
  DROP INDEX idx_agreement_parties_user_agreement_tenant,
  DROP INDEX idx_agreement_parties_user_agreement_status;
ALTER TABLE agreement_parties
  RENAME TO room_rent_plan_members,
  CHANGE COLUMN agreement_id room_rent_plan_id bigint unsigned NOT NULL,
  DROP COLUMN joined_at,
  DROP COLUMN left_at,
  DROP COLUMN status,
  ADD UNIQUE KEY uq_room_rent_plan_members_user_plan_tenant (user_id, room_rent_plan_id, tenant_id),
  ADD UNIQUE KEY uq_room_rent_plan_members_user_id_plan_tenant (user_id, id, room_rent_plan_id, tenant_id),
  ADD KEY idx_room_rent_plan_members_user_tenant (user_id, tenant_id),
  ADD CONSTRAINT fk_room_rent_plan_members_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_room_rent_plan_members_plan FOREIGN KEY (user_id, room_rent_plan_id) REFERENCES room_rent_plans(user_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_room_rent_plan_members_tenant FOREIGN KEY (user_id, tenant_id) REFERENCES tenants(user_id, id) ON DELETE RESTRICT;

ALTER TABLE rent_charges
  DROP FOREIGN KEY fk_rent_charges_user,
  DROP FOREIGN KEY fk_rent_charges_property,
  DROP FOREIGN KEY fk_rent_charges_room,
  DROP FOREIGN KEY fk_rent_charges_agreement,
  DROP INDEX idx_rent_charges_user_agreement_period;
ALTER TABLE rent_charges
  CHANGE COLUMN tenancy_agreement_id room_rent_plan_id bigint unsigned NOT NULL,
  ADD UNIQUE KEY uq_rent_charges_user_id_plan_month (user_id, id, room_rent_plan_id, period_month),
  ADD KEY idx_rent_charges_user_plan_period (user_id, room_rent_plan_id, period_month),
  ADD CONSTRAINT fk_rent_charges_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_rent_charges_property FOREIGN KEY (user_id, property_id) REFERENCES properties(user_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_rent_charges_room_property FOREIGN KEY (user_id, room_id, property_id) REFERENCES rooms(user_id, id, property_id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_rent_charges_plan_room FOREIGN KEY (user_id, room_id, room_rent_plan_id) REFERENCES room_rent_plans(user_id, room_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT chk_rent_charges_expected_amount CHECK (expected_amount_cents > 0);

ALTER TABLE rent_obligations
  DROP FOREIGN KEY fk_rent_obligations_tenant,
  DROP FOREIGN KEY fk_rent_obligations_charge,
  DROP INDEX idx_rent_obligations_user_charge_tenant,
  DROP INDEX idx_rent_obligations_user_charge,
  DROP INDEX idx_rent_obligations_lazy_tenant_period,
  DROP COLUMN lazy_period_month,
  DROP COLUMN generated_by;
ALTER TABLE rent_obligations
  MODIFY COLUMN rent_charge_id bigint unsigned NOT NULL,
  ADD COLUMN room_rent_plan_id bigint unsigned NOT NULL AFTER rent_charge_id,
  ADD COLUMN room_rent_plan_member_id bigint unsigned NOT NULL AFTER room_rent_plan_id,
  ADD UNIQUE KEY uq_rent_obligations_user_charge_tenant (user_id, rent_charge_id, tenant_id),
  ADD UNIQUE KEY uq_rent_obligations_user_charge_member (user_id, rent_charge_id, room_rent_plan_member_id),
  ADD UNIQUE KEY uq_rent_obligations_user_id (user_id, id),
  ADD UNIQUE KEY uq_rent_obligations_user_id_tenant (user_id, id, tenant_id),
  ADD KEY idx_rent_obligations_user_tenant_period (user_id, tenant_id, period_month),
  ADD CONSTRAINT fk_rent_obligations_tenant FOREIGN KEY (user_id, tenant_id) REFERENCES tenants(user_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_rent_obligations_charge_plan_period FOREIGN KEY (user_id, rent_charge_id, room_rent_plan_id, period_month) REFERENCES rent_charges(user_id, id, room_rent_plan_id, period_month) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_rent_obligations_plan_member FOREIGN KEY (user_id, room_rent_plan_member_id, room_rent_plan_id, tenant_id) REFERENCES room_rent_plan_members(user_id, id, room_rent_plan_id, tenant_id) ON DELETE RESTRICT,
  ADD CONSTRAINT chk_rent_obligations_expected_amount CHECK (expected_amount_cents > 0),
  ADD CONSTRAINT chk_rent_obligations_paid_amount CHECK (paid_amount_cents >= 0),
  ADD CONSTRAINT chk_rent_obligations_period_month CHECK (DAYOFMONTH(period_month) = 1);

ALTER TABLE payment_allocations
  DROP FOREIGN KEY fk_payment_allocations_obligation,
  DROP INDEX fk_payment_allocations_obligation;
ALTER TABLE payment_allocations
  ADD CONSTRAINT fk_payment_allocations_obligation FOREIGN KEY (user_id, rent_obligation_id) REFERENCES rent_obligations(user_id, id) ON DELETE RESTRICT;

ALTER TABLE cash_receipts
  DROP FOREIGN KEY fk_cash_receipts_tenant,
  DROP FOREIGN KEY fk_cash_receipts_obligation,
  DROP INDEX idx_cash_receipts_user_tenant_received,
  DROP INDEX fk_cash_receipts_obligation;
ALTER TABLE cash_receipts
  CHANGE COLUMN tenant_id payer_tenant_id bigint unsigned NULL,
  ADD COLUMN payer_name_snapshot varchar(191) NULL AFTER payer_tenant_id,
  ADD KEY idx_cash_receipts_user_payer_received (user_id, payer_tenant_id, received_at),
  ADD CONSTRAINT fk_cash_receipts_payer FOREIGN KEY (user_id, payer_tenant_id) REFERENCES tenants(user_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_cash_receipts_obligation FOREIGN KEY (user_id, rent_obligation_id) REFERENCES rent_obligations(user_id, id) ON DELETE RESTRICT;

ALTER TABLE dunning_send_attempts
  DROP FOREIGN KEY fk_dunning_attempts_tenant,
  DROP FOREIGN KEY fk_dunning_attempts_obligation,
  DROP INDEX fk_dunning_attempts_obligation;
ALTER TABLE dunning_send_attempts
  ADD CONSTRAINT fk_dunning_attempts_obligation FOREIGN KEY (user_id, rent_obligation_id, tenant_id) REFERENCES rent_obligations(user_id, id, tenant_id) ON DELETE RESTRICT;
