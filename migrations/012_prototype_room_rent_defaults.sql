ALTER TABLE rooms
  ADD COLUMN monthly_rent_cents bigint NOT NULL DEFAULT 0 AFTER capacity,
  ADD COLUMN due_day int NOT NULL DEFAULT 1 AFTER monthly_rent_cents;
