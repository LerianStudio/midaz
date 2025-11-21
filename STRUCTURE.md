# Project Structure Overview

Welcome to the comprehensive guide on the structure of our project, which is designed with a focus on scalability, maintainability, and clear separation of concerns in line with the Command Query Responsibility Segregation (CQRS) pattern. This architecture not only enhances our project's efficiency and performance but also ensures that our codebase is organized in a way that allows developers to navigate and contribute effectively.

#### Directory Layout

The project is structured into several key directories, each serving specific roles:

```
MIDAZ
 |   bin
 |   components
 |---   infra
 |---   |   artifacts
 |---   |   grafana
 |---   |   postgres
 |---   |   rabbitmq
 |---   |---   etc
 |---   onboarding
 |---   |   api
 |---   |   artifacts
 |---   |   cmd
 |---   |---   app
 |---   |   internal
 |---   |---   adapters
 |---   |---   |   http
 |---   |---   |---   in
 |---   |---   |---   out
 |---   |---   |   mongodb
 |---   |---   |   postgres
 |---   |---   |---   account
 |---   |---   |---   asset
 |---   |---   |---   ledger
 |---   |---   |---   organization
 |---   |---   |---   portfolio
 |---   |---   |---   segment
 |---   |---   |   rabbitmq
 |---   |---   |   redis
 |---   |---   bootstrap
 |---   |---   services
 |---   |---   |   command
 |---   |---   |   query
 |---   |   migrations
 |---   transaction
 |---   |   api
 |---   |   artifacts
 |---   |   cmd
 |---   |---   app
 |---   |   internal
 |---   |---   adapters
 |---   |---   |   http
 |---   |---   |---   in
 |---   |---   |---   out
 |---   |---   |   mongodb
 |---   |---   |   postgres
 |---   |---   |---   assetrate
 |---   |---   |---   balance
 |---   |---   |---   operation
 |---   |---   |---   transaction
 |---   |---   |   rabbitmq
 |---   |---   |   redis
 |---   |---   bootstrap
 |---   |---   services
 |---   |---   |   command
 |---   |---   |   query
 |---   |   migrations
 |   image
 |---   README
 |   pkg
 |---   constant
 |---   errors
 |---   gold
 |---   |   parser
 |---   |   transaction
 |---   mmodel
 |---   mgrpc
 |---   net
 |---   |   http
 |---   pointers
 |---   server
 |---   shell
 |---   utils
 |   postman
 |   scripts
 |   tests
```

#### Common Utilities (`./pkg`)

* `console`: Description of the console utilities and their usage.
* `libLog`: Overview of the logging framework and configuration details.md
* `libMongo`, `mpostgres`: Database utilities, including setup and configuration.
* `libZap`: Details on the structured logger adapted for high-performance scenarios.
* `shell`: Guide on shell utilities, including scripting and automation tools.
* `constant`: Project constants for headers, errors, metadata, and configuration values.
* `errors`: Custom error types and error handling utilities.
* `gold`: Transaction parser using ANTLR grammar for .gold file format.
* `mgrpc`: gRPC utilities for connections, protobuf definitions, and error mapping.
* `mmodel`: Domain models including Account, Balance, Organization, Portfolio, and related structures.
* `net`: HTTP network utilities for handlers, responses, and request processing.
* `pointers`: Helper functions to create pointers from primitive types.
* `server`: Server utilities for graceful shutdown and gRPC server management.
* `utils`: General utilities for caching, jitter, metrics, time operations, and string manipulation.

#### Components (`./components`)

##### Ledger (`./components/onboarding`)

###### API (`./onboarding/api`)

* **Endpoints** : List and describe all API endpoints, including parameters, request/response formats, and error codes.

###### Internal (`./onboarding/internal`)

* **Adapters** (`./adapters`):
  * **Database** : Connection and operation guides for MongoDB and PostgreSQL.
* **Application Logic** (`./app`):
  * **Command** : Documentation of command handlers, including how commands are processed.
  * **Query** : Details on query handlers, how queries are executed, and their return structures.
* **Domain** (`./domain`):
  * Description of domain models such as Onboarding, Portfolio, Transaction, etc., and their relationships.
* **Services** (`./service`):
  * Detailed information on business logic services, their roles, and interactions in the application.

### Configuration (`./config`)

* **Identity Schemas** (`./identity-schemas`): Guide on setting up and modifying identity schemas.

### Miscellaneous

#### Images (`./image`)

* **README** : Purpose of images stored and how to use them in the project.

#### Postman Collections (`./postman`)

* **Usage** : How to import and use the provided Postman collections for API testing.
