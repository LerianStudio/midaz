-- Single-row control table that gates the dual-write path in the application
-- layer. Phases:
--   legacy_only   Only the non-partitioned table is written/read. Default.
--   dual_write    INSERTs go to both the legacy and partitioned tables. Reads
--                 still come from the legacy table.
--   partitioned   The partitioned table has been RENAMEd over the legacy name
--                 by migration 000022/000023. Legacy table is retained as
--                 <name>_legacy until explicit cleanup (migration 000024).

CREATE TABLE IF NOT EXISTS partition_migration_state (
    id         SMALLINT NOT NULL PRIMARY KEY DEFAULT 1,
    phase      TEXT NOT NULL DEFAULT 'legacy_only',
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    CONSTRAINT partition_migration_state_singleton CHECK (id = 1),
    CONSTRAINT partition_migration_state_phase_check
        CHECK (phase IN ('legacy_only', 'dual_write', 'partitioned'))
);

INSERT INTO partition_migration_state (id, phase) VALUES (1, 'legacy_only')
ON CONFLICT (id) DO NOTHING;
