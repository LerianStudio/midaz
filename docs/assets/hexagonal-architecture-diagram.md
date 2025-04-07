# Hexagonal Architecture Diagram

This ASCII diagram illustrates the hexagonal architecture pattern implemented in Midaz:

```
                    +----------------------------------+
                    |                                  |
                    |        External Systems          |
                    |                                  |
                    +--------------+-+-----------------+
                                   | |
                                   | |
                    +--------------v-v-----------------+
                    |                                  |
                    |        Primary Adapters          |
                    |    (HTTP, CLI, Event Consumers)  |
                    |                                  |
+-------------------|                                  |-------------------+
|                   +--^---------------------------^---+                   |
|                      |                           |                       |
|                      |                           |                       |
|    +------------------v---+           +----------v-------------+        |
|    |                      |           |                        |        |
|    |    Primary Ports     |           |     Secondary Ports    |        |
|    |    (API Interfaces)  |           |  (Repository/Service   |        |
|    |                      |           |       Interfaces)      |        |
|    +----------+-----------+           +-----------+------------+        |
|               |                                   |                     |
|               |                                   |                     |
|    +----------v---------------------------------v-+                     |
|    |                                              |                     |
|    |               Domain Layer                   |                     |
|    |        (Business Logic & Models)             |                     |
|    |                                              |                     |
|    +----------------------------------------------+                     |
|                                                                         |
|                    Application Core                                     |
+-------------------------------------------------------------------------+
                                   |
                                   |
                    +-------------v------------------+
                    |                                |
                    |       Secondary Adapters       |
                    | (Database, MessageQueue, etc.) |
                    |                                |
                    +--------------------------------+
                                   |
                                   |
                    +-------------v------------------+
                    |                                |
                    |       External Resources       |
                    |  (PostgreSQL, MongoDB, etc.)   |
                    |                                |
                    +--------------------------------+
```

## Architecture Explanation

### Layers

1. **Domain Layer (Core)**
   - Contains business logic and domain models
   - Pure Go structs (Organization, Ledger, Asset, Account, etc.)
   - Business rules and validations
   - No dependencies on external frameworks or resources

2. **Ports (Interfaces)**
   - **Primary/Driving Ports**: Define how external actors use the application
   - **Secondary/Driven Ports**: Define how the application uses external resources
   - Implemented as Go interfaces

3. **Adapters (Implementations)**
   - **Primary/Driving Adapters**: Implementation of primary ports
     - HTTP Controllers
     - CLI Commands
     - Event Consumers
   - **Secondary/Driven Adapters**: Implementation of secondary ports
     - Database Repositories (PostgreSQL, MongoDB)
     - Message Queue Clients (RabbitMQ)
     - Cache Clients (Redis)

### Flow Directions

- **Inbound Flow**: External requests → Primary Adapters → Primary Ports → Domain Layer
- **Outbound Flow**: Domain Layer → Secondary Ports → Secondary Adapters → External Resources

## Practical Example

In Midaz, a request to create an organization follows this path:

1. HTTP request received by HTTP Controller (Primary Adapter)
2. Controller calls CreateOrganization use case (Primary Port)
3. Use case implements business logic, validations (Domain Layer)
4. Use case calls repository interfaces (Secondary Ports)
5. PostgreSQL repository implementation handles persistence (Secondary Adapter)
6. Data is stored in the database (External Resource)

## Related Documentation
- [Hexagonal Architecture](../architecture/hexagonal-architecture.md)
- [Component Integration](../architecture/component-integration.md)
- [System Overview](../architecture/system-overview.md)