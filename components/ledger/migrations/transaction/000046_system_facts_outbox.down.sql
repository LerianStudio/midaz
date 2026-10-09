-- Exposed as outbox.DropSchemaSQL(), for the producer's down migration. Drops
-- the kit table only: outbox_event_status is shared with a producer's own
-- lib-commons outbox, so it is never dropped here.
DROP TABLE IF EXISTS system_facts_outbox;
