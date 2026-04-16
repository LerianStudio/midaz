-- Stage 5: cleanup. Drop the legacy (pre-partition) tables after the
-- partitioned tables have been serving production traffic for long enough
-- that no rollback is required. Scheduled to run days/weeks after 000022/000023,
-- NOT in the same deploy as the swap.
--
-- RESTRICT is used (not CASCADE) so the migration fails loudly if any
-- lingering object still depends on the legacy tables. Operators must
-- investigate rather than silently truncating dependencies.

DROP TABLE IF EXISTS operation_legacy RESTRICT;
DROP TABLE IF EXISTS balance_legacy RESTRICT;
