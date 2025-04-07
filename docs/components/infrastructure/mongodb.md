# MongoDB Configuration

**Navigation:** [Home](../../) > [Components](../) > [Infrastructure](./) > MongoDB

This document describes the MongoDB configuration and usage in the Midaz system.

## Overview

MongoDB is used in the Midaz system as a flexible document database primarily for storing metadata associated with entities maintained in PostgreSQL. This follows a polyglot persistence pattern where structured, relational data is stored in PostgreSQL while flexible, schema-less metadata is stored in MongoDB.

## Database Structure

MongoDB in Midaz follows these structural patterns:

- **Single database** with multiple collections based on entity types
- **Collections naming** follows the entity types (e.g., "organization", "ledger")
- **Document structure** is consistent with a standard metadata schema
- **Replica set configuration** for high availability

### Collections

Collections are created dynamically based on entity types, including:
- `organization`
- `ledger`
- `asset`
- `segment`
- `portfolio`
- `account`
- `transaction`

### Document Structure

Each metadata document follows this structure:

```json
{
  "_id": ObjectId("..."),
  "entity_id": "uuid-reference-to-postgres-entity",
  "entity_name": "EntityTypeName",
  "metadata": {
    // Flexible, entity-specific JSON data here
  },
  "created_at": ISODate("2023-01-01T00:00:00.000Z"),
  "updated_at": ISODate("2023-01-01T00:00:00.000Z")
}
```

## Database Configuration

### Initial Setup

MongoDB is set up as a replica set with authentication enabled:

1. **Replica Set Configuration**: 
   - Named "rs0"
   - Initially configured with a single node (can be expanded in production)
   - Uses keyfile-based authentication between nodes

2. **Authentication**:
   - Creates an admin user with root access
   - Uses SCRAM-SHA-256 authentication mechanism
   - Secured with a dedicated keyfile for inter-node authentication

### Initialization Script

The system uses a script (`mongo.sh`) to initialize MongoDB:

```bash
# Initialize the replica set
mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" <<EOF
    rs.initiate({
      _id: "rs0",
      members: [{ _id: 0, host: "${MONGO_HOST}:${MONGO_PORT}" }]
    });
EOF

# Create the admin user
mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" <<EOF
    use admin;
    db.createUser({
      user: "${MONGO_USER}",
      pwd: "${MONGO_PASSWORD}",
      roles: [
        { role: "root", db: "admin" },
        { role: "userAdminAnyDatabase", db: "admin" },
        { role: "dbAdminAnyDatabase", db: "admin" },
        { role: "readWriteAnyDatabase", db: "admin" }
      ]
    });
EOF
```

### Connection Configuration

MongoDB connections use URI-style connection strings:

```
mongodb://${MONGO_USER}:${MONGO_PASSWORD}@${MONGO_HOST}:${MONGO_PORT}/${MONGO_DATABASE}?replicaSet=rs0&authSource=admin
```

Key environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| MONGO_HOST | MongoDB host | localhost |
| MONGO_PORT | MongoDB port | 27017 |
| MONGO_USER | MongoDB username | admin |
| MONGO_PASSWORD | MongoDB password | adminpassword |
| MONGO_DATABASE | Database name | midaz |
| MONGO_MAX_POOL_SIZE | Maximum connection pool size | 100 |

## Connection Management

### Connection Pooling

The system implements connection pooling with configurable limits:
- `MaxPoolSize`: Controls maximum concurrent connections (default: 100)
- Connections are shared through a centralized `MongoConnection` instance

### Connection Lifecycle

The application follows these connection patterns:
1. MongoDB connection is established during bootstrap
2. Connection pool is maintained throughout application lifecycle
3. Each repository operation obtains a connection from the pool
4. Connections are automatically returned to the pool after use

## Data Access Patterns

### Repository Pattern

MongoDB access follows the repository pattern:
- Interface defines repository contract (`Repository`)
- MongoDB-specific implementation (`MetadataMongoDBRepository`) provides database access
- Clear separation between domain models and MongoDB models

### CRUD Operations

Standard operations on metadata:
- **Create**: Insert new metadata documents
- **FindList**: Retrieve multiple metadata documents with optional filtering
- **FindByEntity**: Find metadata for a specific entity by ID
- **Update**: Modify existing metadata (with upsert capability)
- **Delete**: Remove metadata documents

### Pagination and Filtering

Supports:
- Skip/limit pagination
- Filtering by metadata fields
- Flexible query criteria

## Monitoring & Telemetry

MongoDB operations are instrumented with:

1. **OpenTelemetry Integration**:
   - Each database operation creates spans for performance tracking
   - Error details are captured in spans using `HandleSpanError`
   - Child spans created for specific operations (find, insert, update, delete)

2. **Structured Logging**:
   - Context-aware logging
   - Consistent error logging
   - Operation result logging (e.g., record counts)

## Running Locally

To run MongoDB locally using Docker:

```bash
cd components/infra
docker-compose up -d mongo
```

## Best Practices

1. **Use the Repository Pattern** for consistent data access
2. **Implement Connection Pooling** to efficiently manage connections
3. **Set up proper authentication** for security
4. **Configure replica sets** for high availability
5. **Use OpenTelemetry instrumentation** for observability
6. **Keep flexible schema in MongoDB** while structured data stays in PostgreSQL
7. **Maintain cross-database referential integrity** via entity_id references

## Common Issues and Troubleshooting

1. **Connection Issues**:
   - Check that the replica set is properly initialized
   - Verify authentication credentials are correct
   - Ensure the mongo-keyfile has proper permissions (600)

2. **Performance Issues**:
   - Add appropriate indexes for frequently queried fields
   - Monitor connection pool utilization
   - Use projection to limit returned fields when appropriate

3. **Replica Set Problems**:
   - Check replica set status with `rs.status()`
   - Verify network connectivity between nodes
   - Ensure each node has a unique identifier

## Related Documentation

- [Entity Lifecycle](../../architecture/data-flow/entity-lifecycle.md)
- [Metadata Approach](../../domain-models/metadata-approach.md)
- [PostgreSQL Configuration](postgresql.md)
