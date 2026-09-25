ALTER TABLE properties ADD COLUMN deleted_at timestamp NULL;

ALTER TABLE rooms ADD COLUMN deleted_at timestamp NULL;
