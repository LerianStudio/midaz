# Redis/Valkey Configuration

**Navigation:** [Home](../../) > [Components](../) > [Infrastructure](./) > Redis Configuration

## Overview

Redis (or Valkey) is used as an in-memory data store within the Midaz platform, providing high-speed caching, session management, and temporary data storage capabilities. Its low-latency characteristics make it ideal for performance-critical operations that require quick access to data.

## Features

- **In-memory data storage**: Provides microsecond response times for data access
- **Key-value storage model**: Simple yet powerful data structure support
- **Data persistence**: Optional persistence to disk for durability
- **Pub/Sub messaging**: Used for real-time messaging between components
- **Lua scripting**: Supports atomic operations through Lua scripts
- **Data structures**: Includes strings, lists, sets, sorted sets, hashes, bitmaps, and hyperloglogs

## Configuration

Redis is configured through the `/components/infra/docker-compose.yml` file and customized environment variables.

### Docker Configuration

```yaml
redis:
  image: redis:7.2-alpine
  container_name: midaz-redis
  command: redis-server --appendonly yes --requirepass ${REDIS_PASSWORD}
  ports:
    - "6379:6379"
  volumes:
    - redis-data:/data
  networks:
    - infra-network
  healthcheck:
    test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"]
    interval: 5s
    timeout: 5s
    retries: 5
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `REDIS_PASSWORD` | Authentication password for Redis access | *[from .env file]* |
| `REDIS_HOST` | Hostname for Redis connections | `redis` |
| `REDIS_PORT` | Port for Redis connections | `6379` |

## Usage in Midaz

Redis serves several functions within the Midaz platform:

### Caching

- API response caching
- Database query result caching
- Object caching for high-frequency access patterns

### Session Management

- User session storage
- Authentication token caching
- Rate limiting implementation

### Temporary Data Storage

- Processing queue management
- Transaction processing state management
- Distributed locking mechanisms

### Pub/Sub Messaging

- Real-time notifications between services
- Synchronization of distributed operations
- Event broadcasting

## Data Persistence

Redis is configured with appendonly mode enabled to ensure data durability. This means:

- All write operations are logged to an append-only file
- Data can be recovered after a restart
- Periodic background saving of datasets occurs

## Security

Redis security is implemented through:

1. **Password authentication**: Required for all connections
2. **Network isolation**: Redis is accessible only within the Docker network
3. **No external exposure**: Only exposed through application services
4. **Command restrictions**: Disables dangerous commands in production environments

## Monitoring

Redis instances can be monitored through:

1. **Grafana dashboards**: Pre-configured dashboards track key metrics
2. **INFO command**: Provides detailed statistics about the Redis server
3. **Redis CLI**: Interactive command-line access for debugging
4. **Logs**: Container logs capture operational information

Example monitoring command:
```bash
docker exec midaz-redis redis-cli -a $REDIS_PASSWORD info
```

## Scaling Considerations

For production environments with high throughput requirements, consider:

- **Redis Cluster**: For horizontal scaling across multiple nodes
- **Redis Sentinel**: For high availability and automatic failover
- **Read replicas**: For distributing read loads

## Backup Strategy

Redis data is backed up through:

1. **RDB snapshots**: Point-in-time snapshots of the dataset
2. **AOF files**: Continuous append-only files for transaction logs
3. **Volume backups**: Docker volume backups for complete data preservation

## Maintenance Tasks

Regular maintenance includes:

1. **Memory management**: Monitoring memory usage and implementing key eviction policies
2. **Key expiration**: Setting appropriate TTLs for cached data
3. **Configuration updates**: Adjusting performance parameters as needed
4. **Version upgrades**: Following a tested upgrade path for new Redis versions

## Troubleshooting

Common issues and solutions:

| Issue | Solution |
|-------|----------|
| Connection refused | Check container status, network configuration, and firewall rules |
| Authentication failure | Verify password in environment variables and connection strings |
| Memory limit reached | Increase container memory allocation or implement key eviction policies |
| Slow performance | Check for large keys, optimize data structures, or consider Redis Cluster |

## Related Documentation

- [Official Redis Documentation](https://redis.io/documentation)
- [Valkey Documentation](https://valkey.io/documentation) (Redis compatible)
- [Docker Hub Redis](https://hub.docker.com/_/redis)