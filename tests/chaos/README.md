Chaos tests inject failures into containers (stop/pause/restart/network disruptions) and assert that the system recovers without data loss.

Targets (from compose):
- midaz-postgres-primary, midaz-postgres-replica
- midaz-mongodb, midaz-valkey
- midaz-rabbitmq
- midaz-onboarding, midaz-transaction

