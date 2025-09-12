Integration tests interact with running services and backing stores to validate behavior across boundaries. These tests assume the local Docker stack is running or will be managed by the tests if MIDAZ_TEST_MANAGE_STACK=true.

Focus:
- Onboarding + Transaction APIs working together
- Persistence in PostgreSQL/Mongo
- Basic Redis and RabbitMQ flows where applicable

