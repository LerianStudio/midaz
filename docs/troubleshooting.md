# Troubleshooting Guide

**Navigation:** [Home](./) > Troubleshooting

This guide helps you diagnose and resolve common issues when working with the Midaz platform. It covers infrastructure problems, service errors, development challenges, and more.

## Table of Contents

- [Authentication Issues](#authentication-issues)
- [Database Issues](#database-issues)
- [Message Queue Issues](#message-queue-issues)
- [Transaction Processing Issues](#transaction-processing-issues)
- [Entity Management Issues](#entity-management-issues)
- [Infrastructure Setup Problems](#infrastructure-setup-problems)
- [API Integration Issues](#api-integration-issues)
- [CLI Issues](#cli-issues)
- [Development Environment Issues](#development-environment-issues)
- [Debugging and Diagnostics](#debugging-and-diagnostics)

## Authentication Issues

### Login Failures

**Symptoms:**
- Unable to authenticate via MDZ CLI
- "Authentication failed" errors
- Browser login page not loading

**Possible Causes:**
- Empty or incorrect username/password
- Authentication service unavailability
- Token expiration
- Network connectivity issues

**Diagnosis:**
1. Check for error messages like "username must not be empty" or "invalid credentials"
2. Verify network connectivity to authentication servers
3. Check if token exists in settings file (`~/.config/mdz/mdz.toml`)

**Solutions:**
1. Run `mdz login` with valid credentials
2. If browser authentication fails, try terminal method with `mdz login --username your@email.com`
3. Check configuration with `mdz configure --show` and update if necessary
4. Verify network connectivity to the authentication service URL

### Token Validation Failures

**Symptoms:**
- Operations fail after previously working
- "Invalid token" or "Token expired" errors
- Error codes: `ErrTokenMissing` (0041), `ErrInvalidToken` (0042)

**Possible Causes:**
- Token expiration
- JWK service unavailability
- Invalid token format

**Solutions:**
1. Re-authenticate using `mdz login`
2. Verify JWK address configuration in environment variables
3. Check Casdoor JWK service availability

## Database Issues

### PostgreSQL Connection Problems

**Symptoms:**
- Services fail to start
- "Unable to connect to database" errors
- Timeout errors during database operations

**Possible Causes:**
- Database service not running
- Incorrect connection parameters
- Network connectivity issues
- Insufficient privileges

**Diagnosis:**
1. Check PostgreSQL logs for connection attempts
   ```
   docker-compose logs midaz-postgres-primary
   ```
2. Verify database container health status
   ```
   docker-compose ps
   ```
3. Confirm environment variables match the database configuration

**Solutions:**
1. Restart PostgreSQL services
   ```
   docker-compose restart midaz-postgres-primary
   ```
2. Verify database environment variables:
   - `DB_HOST`
   - `DB_PORT`
   - `DB_USER`
   - `DB_PASSWORD`
3. Check database user permissions and connection limits

### MongoDB Connection Problems

**Symptoms:**
- Metadata operations failing
- "Cannot connect to MongoDB" errors
- Replica set initialization failures

**Possible Causes:**
- MongoDB service not running
- Replica set initialization failure
- Authentication issues
- Network connectivity problems

**Diagnosis:**
1. Check MongoDB logs for connection errors
   ```
   docker-compose logs midaz-mongodb
   ```
2. Verify replica set status
   ```
   docker-compose exec midaz-mongodb mongosh --eval "rs.status()"
   ```

**Solutions:**
1. Restart MongoDB
   ```
   docker-compose restart midaz-mongodb
   ```
2. Re-initialize replica set
3. Verify MongoDB credentials in environment variables

### Data Integrity Issues

**Symptoms:**
- Foreign key constraint errors
- Entity relationship violations
- "Entity not found" when trying to create dependent entities

**Possible Causes:**
- Entity relationships violated
- Missing referenced entities
- Race conditions in entity creation

**Diagnosis:**
- Error codes: `ErrOrganizationIDNotFound` (0038), `ErrLedgerIDNotFound` (0037)
- PostgreSQL constraint violation errors

**Solutions:**
1. Ensure parent entities exist before creating dependent entities
2. Verify entity IDs match across related objects
3. Check operation sequence to maintain referential integrity

## Message Queue Issues

### RabbitMQ Connection Problems

**Symptoms:**
- Services failing to start or process messages
- "Failed to connect to RabbitMQ" errors
- Event-driven operations not completing

**Possible Causes:**
- RabbitMQ service not running
- Authentication issues
- Incorrect connection parameters
- Network connectivity issues

**Diagnosis:**
1. Check RabbitMQ management interface (accessible on port 3004)
2. Verify connection logs
   ```
   docker-compose logs midaz-rabbitmq
   ```
3. Test connectivity to RabbitMQ ports

**Solutions:**
1. Restart RabbitMQ
   ```
   docker-compose restart midaz-rabbitmq
   ```
2. Verify RabbitMQ environment variables
3. Check network connectivity to RabbitMQ ports

### Queue Message Processing Failures

**Symptoms:**
- Operations hanging or never completing
- Inconsistent state between services
- Messages accumulating in queues

**Possible Causes:**
- Consumer service not running
- Queue bindings misconfigured
- Message format issues
- Queue overflow

**Diagnosis:**
1. Check queue depth in RabbitMQ management console
2. Verify exchange and queue bindings
3. Monitor consumer logs for processing errors

**Solutions:**
1. Verify exchange and queue definitions in RabbitMQ config
2. Restart consumer services
3. Check for error logs in consumer applications

## Transaction Processing Issues

### Insufficient Funds

**Symptoms:**
- Transactions failing
- "Insufficient funds" errors
- Error codes: `ErrInsufficientFunds` (0018), `ErrInsufficientAccountBalance` (0025)

**Possible Causes:**
- Account balance too low
- Balance calculation errors
- Concurrency issues

**Diagnosis:**
1. Check account balances before transaction
2. Verify transaction amount doesn't exceed available funds
3. Look for balance calculation errors in logs

**Solutions:**
1. Ensure account has sufficient funds before transaction
2. Fix concurrent transaction issues with proper locking
3. Use account describe command to verify current balance
   ```
   mdz account describe --organization-id ORG_ID --ledger-id LEDGER_ID --portfolio-id PORTFOLIO_ID --account-id ACCOUNT_ID
   ```

### Transaction Validation Failures

**Symptoms:**
- Transactions rejected
- Validation error messages
- Error codes: `ErrMismatchedAssetCode` (0030), `ErrInvalidTransactionType` (0072)

**Possible Causes:**
- Invalid transaction format
- Missing required fields
- Business rule violations
- Asset mismatches

**Diagnosis:**
1. Review error messages for specific validation failures
2. Check transaction data against API requirements
3. Verify business rules compliance

**Solutions:**
1. Ensure transaction format follows API specifications
2. Provide all required fields
3. Verify asset codes match between source and destination accounts
4. Check that transaction follows business validation rules

### Idempotency Key Issues

**Symptoms:**
- Duplicate transaction processing
- "Idempotency key already exists" errors
- Error code: `ErrIdempotencyKey` (0084)

**Possible Causes:**
- Missing idempotency keys
- Redis connectivity issues
- Key collision

**Diagnosis:**
1. Check if idempotency keys are being properly generated
2. Verify Redis connection and availability
3. Look for duplicate key errors in logs

**Solutions:**
1. Ensure idempotency keys are unique for each transaction
2. Verify Redis connectivity
3. Implement retry logic with the same idempotency key

## Entity Management Issues

### Entity Creation Failures

**Symptoms:**
- Failures when creating organizations, ledgers, accounts, etc.
- Database constraint violation errors
- Error codes: `ErrDuplicateLedger` (0001), `ErrAssetNameOrCodeDuplicate` (0003)

**Possible Causes:**
- Missing required fields
- Duplicated unique fields
- Foreign key constraints
- Permission issues

**Diagnosis:**
1. Check error message for specific constraint violations
2. Verify all required fields are provided
3. Ensure unique constraints aren't violated

**Solutions:**
1. Provide all required fields when creating entities
2. Ensure unique fields don't conflict with existing entities
3. Verify parent entities exist before creating dependent entities
4. Check permissions for entity creation

### Entity Not Found

**Symptoms:**
- "Entity not found" errors during operations
- Error code: `ErrEntityNotFound` (0007)
- Specific errors: `ErrAccountIDNotFound` (0052), `ErrAssetIDNotFound` (0055)

**Possible Causes:**
- Entity doesn't exist
- Entity was deleted
- Incorrect ID format
- Permission issues

**Diagnosis:**
1. Check if entity exists using list or describe commands
2. Verify UUID format is correct
3. Check for typos in entity IDs

**Solutions:**
1. Verify entity exists before attempting operations
   ```
   mdz <entity-type> list --organization-id ORG_ID
   ```
2. Ensure you're using the correct ID
3. Check for permissions to access the entity

## Infrastructure Setup Problems

### Docker Compose Issues

**Symptoms:**
- Services fail to start
- "Port already in use" errors
- Container exit errors

**Possible Causes:**
- Environment variable misconfiguration
- Port conflicts
- Volume mount issues
- Network problems

**Diagnosis:**
1. Check docker-compose logs
   ```
   docker-compose logs
   ```
2. Verify container health status
   ```
   docker-compose ps
   ```
3. Inspect network connectivity between containers

**Solutions:**
1. Ensure all required environment variables are set in `.env` file
2. Check for port conflicts with `netstat -tuln`
3. Verify volume mounts and permissions
4. Recreate the network
   ```
   docker-compose down
   docker-compose up -d
   ```

### Environment Variable Issues

**Symptoms:**
- Applications unable to start
- "Missing required environment variable" errors
- Configuration-related panics

**Possible Causes:**
- Missing required environment variables
- Incorrect variable formats
- Environment file not loaded

**Diagnosis:**
1. Check application startup logs for configuration errors
2. Verify environment variable presence

**Solutions:**
1. Ensure `.env` file exists and contains all required variables
2. Verify environment variable formats
3. Use the environment validation script
   ```
   ./scripts/check-envs.sh
   ```

## API Integration Issues

### API Request Failures

**Symptoms:**
- HTTP error responses (400, 401, 403, 500)
- Error codes: `ErrBadRequest` (0047), `ErrInvalidRequestBody` (0094)
- Timeout or connection errors

**Possible Causes:**
- Malformed requests
- Missing required fields
- Invalid request body
- Authentication issues

**Diagnosis:**
1. Check HTTP status code and error message
2. Verify request format against API documentation
3. Inspect request and response payloads

**Solutions:**
1. Format request according to API documentation
2. Provide all required fields
3. Verify authentication token is valid
4. Check network connectivity to API servers

### API Rate Limiting

**Symptoms:**
- HTTP 429 "Too Many Requests" responses
- Increasing response times
- Intermittent failures

**Possible Causes:**
- Exceeding rate limits
- API server overload

**Solutions:**
1. Implement request throttling
2. Batch requests when possible
3. Optimize API usage patterns
4. Implement exponential backoff for retries

## CLI Issues

### Command Execution Failures

**Symptoms:**
- MDZ CLI commands failing
- Error responses from server
- Connection timeouts

**Possible Causes:**
- Missing authentication
- Command syntax errors
- Network connectivity issues
- Server-side errors

**Diagnosis:**
1. Check CLI error output
2. Try running with verbose flag if available
3. Verify connectivity to API servers

**Solutions:**
1. Check command syntax with `mdz <command> --help`
2. Ensure authentication with `mdz login`
3. Verify network connectivity to API servers
4. Check server logs for related errors

### Configuration Issues

**Symptoms:**
- "Configuration not found" errors
- Settings not being applied
- Connection failures

**Possible Causes:**
- Corrupted configuration file
- Permission issues
- Missing settings

**Diagnosis:**
1. Verify configuration file exists
   ```
   cat ~/.config/mdz/mdz.toml
   ```
2. Check file permissions
3. Inspect configuration contents

**Solutions:**
1. Reset configuration
   ```
   mdz configure
   ```
2. Fix configuration file permissions
3. Manually edit configuration file if necessary

## Development Environment Issues

### Dependencies and Installation

**Symptoms:**
- Build errors
- Missing dependencies
- Version compatibility issues

**Possible Causes:**
- Missing prerequisites
- Incompatible versions
- Environment setup issues

**Diagnosis:**
1. Check for build errors
2. Verify installed dependency versions
3. Compare with required dependencies

**Solutions:**
1. Follow installation guide steps precisely
2. Verify Go version compatibility (use Go 1.20+)
3. Install required system dependencies
4. Use dependency management tools correctly

### Testing Issues

**Symptoms:**
- Test failures
- Inconsistent test results
- Integration test errors

**Possible Causes:**
- Environment misconfiguration
- Test data issues
- Race conditions

**Diagnosis:**
1. Run specific failing tests with verbose flag
2. Check test environment configuration
3. Look for timing or race condition issues

**Solutions:**
1. Set up isolated test environment
2. Reset test data between runs
3. Use testing scripts in the scripts directory
   ```
   ./scripts/check-tests.sh
   ```
4. Fix race conditions with proper synchronization

## Debugging and Diagnostics

### Log Analysis

**Techniques:**
1. Check component logs
   ```
   docker-compose logs <service-name>
   ```
2. Filter logs by severity
   ```
   docker-compose logs | grep "ERROR"
   ```
3. Track transaction flow across services by ID
4. Look for error codes and stack traces

### Performance Monitoring

**Tools and Techniques:**
1. Grafana dashboards for monitoring (accessible on port 3000)
2. Database query performance analysis
3. Message queue depth monitoring
   ```
   # Check RabbitMQ management interface on port 3004
   ```
4. API response time tracking

### Diagnostic Commands

**Useful Commands:**
1. Check system health
   ```
   docker-compose ps
   ```
2. Database inspection
   ```
   docker-compose exec midaz-postgres-primary psql -U midaz
   ```
3. RabbitMQ queue inspection
   ```
   # Access management interface at port 3004
   ```
4. MongoDB status
   ```
   docker-compose exec midaz-mongodb mongosh --eval "rs.status()"
   ```
5. Check service logs
   ```
   docker-compose logs --tail=100 <service-name>
   ```