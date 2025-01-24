create user replicator with replication encrypted password 'replicator_password';
select pg_create_physical_replication_slot('replication_slot');

CREATE DATABASE "midaz-00000000-0000-0000-0000-000000000000";