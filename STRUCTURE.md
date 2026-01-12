# Project Structure Overview

Welcome to the comprehensive guide on the structure of our project, which is designed with a focus on scalability, maintainability, and clear separation of concerns in line with the Command Query Responsibility Segregation (CQRS) pattern. This architecture not only enhances our project's efficiency and performance but also ensures that our codebase is organized in a way that allows developers to navigate and contribute effectively.

#### Directory Layout

The project is structured into several key directories, each serving specific roles:

```
MIDAZ
 |   bin
 |   components
 |   |---   crm
 |   |   |---   api
 |   |   |---   cmd
 |   |   |---   internal
 |   |   |   |---   adapters
 |   |   |   |   |---   http
 |   |   |   |   |   |---   in
 |   |   |   |   |---   mongodb
 |   |   |   |---   services
 |   |   |---   scripts
 |   |---   infra
 |   |   |---   artifacts
 |   |   |---   grafana
 |   |   |---   postgres
 |   |   |---   rabbitmq
 |   |   |   |---   etc
 |   |---   ledger
 |   |   |---   api
 |   |   |---   artifacts
 |   |   |---   cmd
 |   |   |   |---   app
 |   |   |---   internal
 |   |   |   |---   adapters
 |   |   |   |   |---   http
 |   |   |   |   |   |---   in
 |   |   |   |---   bootstrap
 |   |   |---   scripts
 |   |---   onboarding
 |   |   |---   api
 |   |   |---   artifacts
 |   |   |---   cmd
 |   |   |   |---   app
 |   |   |---   internal
 |   |   |   |---   adapters
 |   |   |   |   |---   http
 |   |   |   |   |   |---   in
 |   |   |   |   |   |---   out
 |   |   |   |   |---   mongodb
 |   |   |   |   |---   postgres
 |   |   |   |   |   |---   account
 |   |   |   |   |   |---   asset
 |   |   |   |   |   |---   ledger
 |   |   |   |   |   |---   organization
 |   |   |   |   |   |---   portfolio
 |   |   |   |   |   |---   segment
 |   |   |   |   |---   rabbitmq
 |   |   |   |   |---   redis
 |   |   |   |---   bootstrap
 |   |   |   |---   services
 |   |   |   |   |---   command
 |   |   |   |   |---   query
 |   |   |---   migrations
 |   |---   transaction
 |   |   |---   api
 |   |   |---   artifacts
 |   |   |---   cmd
 |   |   |   |---   app
 |   |   |---   internal
 |   |   |   |---   adapters
 |   |   |   |   |---   http
 |   |   |   |   |   |---   in
 |   |   |   |   |   |---   out
 |   |   |   |   |---   mongodb
 |   |   |   |   |---   postgres
 |   |   |   |   |   |---   assetrate
 |   |   |   |   |   |---   balance
 |   |   |   |   |   |---   operation
 |   |   |   |   |   |---   transaction
 |   |   |   |   |---   rabbitmq
 |   |   |   |   |---   redis
 |   |   |   |---   bootstrap
 |   |   |   |---   services
 |   |   |   |   |---   command
 |   |   |   |   |---   query
 |   |   |---   migrations
 |   image
 |   |---   README
 |   pkg
 |   |---   constant
 |   |---   gold
 |   |   |---   parser
 |   |   |---   transaction
 |   |---   mmodel
 |   |---   mgrpc
 |   |---   net
 |   |   |---   http
 |   |---   shell
 |   |---   transaction
 |   postman
 |   scripts
 |   tests
```

#### Common Utilities (`./pkg`)

* `libLog`: Overview of the logging framework and configuration details.
* `libMongo`, `mpostgres`: Database utilities, including setup and configuration.
* `libPointers`: Explanation of any custom pointer utilities or enhancements used in the project.
* `libZap`: Details on the structured logger adapted for high-performance scenarios.
* `libHTTP`: Information on HTTP helpers and network communication utilities.
* `shell`: Guide on shell utilities, including scripting and automation tools.
* `transaction`: Contains details of transaction models and validations

#### Components (`./components`)

##### CRM (`./components/crm`)

###### API (`./components/crm/api`)

* **Endpoints**: List and describe all CRM API endpoints, including parameters, request/response formats, and error codes.

###### Internal (`./components/crm/internal`)

* **Adapters** (`./components/crm/internal/adapters`):
  * **HTTP**: Inbound HTTP handlers for CRM operations.
  * **MongoDB**: Database connection and operations for CRM data persistence.
* **Services** (`./components/crm/internal/services`):
  * Business logic services for customer relationship management operations.

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
