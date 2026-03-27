ALTER TABLE operation ADD COLUMN balance_version_before BIGINT NOT NULL DEFAULT 0;

ALTER TABLE operation ADD COLUMN balance_version_after BIGINT NOT NULL DEFAULT 0;