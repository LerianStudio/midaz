CREATE USER replicator WITH REPLICATION LOGIN ENCRYPTED PASSWORD 'replicator_password';

SELECT pg_create_physical_replication_slot('replication_slot');
SELECT * FROM pg_create_logical_replication_slot('logical_slot', 'pgoutput');

CREATE DATABASE onboarding;
CREATE DATABASE transaction;