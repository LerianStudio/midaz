# System Architecture Diagram

This ASCII diagram illustrates the high-level architecture of the Midaz system:

```
+-------------------+     +-------------------+     +-------------------+
|                   |     |                   |     |                   |
|    MDZ CLI        |     |   External Apps   |     |  Other Clients    |
|                   |     |                   |     |                   |
+--------+----------+     +---------+---------+     +---------+---------+
         |                          |                         |
         |                          |                         |
         +---------------+----------+-------------------------+
                         |
                         v
+------------------------------------------------------------+
|                                                            |
|                     API Gateway                            |
|                                                            |
+--+------------------------+-----------------------------+--+
   |                        |                             |
   |                        |                             |
   v                        v                             v
+--+---------------+     +--+---------------+     +-------+----------+
|                  |     |                  |     |                  |
| Onboarding API   |     | Transaction API  |     |  Future Services |
|                  |     |                  |     |                  |
+--+---------------+     +--+---------------+     +------------------+
   |                        |
   |                        |
+--v------------------------v--+     +--------------------+
|                              |     |                    |
|      PostgreSQL DB           |<--->|     MongoDB       |
| (Transactional Data)         |     | (Metadata Storage)|
|                              |     |                    |
+------------------------------+     +--------------------+
   |                        |
   |                        |
+--v------------------------v--+     +--------------------+
|                              |     |                    |
|         RabbitMQ             |<--->|      Redis        |
|    (Event Communication)     |     |   (Caching)       |
|                              |     |                    |
+------------------------------+     +--------------------+
            |
            v
+------------------------------+
|                              |
|     Monitoring Stack         |
|     (Grafana, Prometheus)    |
|                              |
+------------------------------+
```

## Architecture Components

- **Client Layer**:
  - MDZ CLI: Command-line interface for interacting with the system
  - External Apps: Third-party applications using the API
  - Other Clients: Web interfaces, mobile apps, etc.

- **API Layer**:
  - Onboarding API: Manages entities like organizations, ledgers, assets, etc.
  - Transaction API: Handles financial transaction processing
  - Future Services: Placeholder for additional services

- **Data Storage**:
  - PostgreSQL: Primary database for transactional data
  - MongoDB: NoSQL database for flexible metadata storage

- **Infrastructure**:
  - RabbitMQ: Message broker for asynchronous communication
  - Redis: In-memory data structure store for caching
  - Monitoring: Observability tools (Grafana, Prometheus)

## Related Documentation
- [System Overview](../architecture/system-overview.md)
- [Component Integration](../architecture/component-integration.md)
- [Event-Driven Design](../architecture/event-driven-design.md)