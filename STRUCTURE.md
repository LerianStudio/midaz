# Project Structure Overview

Welcome to the comprehensive guide on the structure of our project, which is designed with a focus on scalability, maintainability, and clear separation of concerns in line with the Command Query Responsibility Segregation (CQRS) pattern. This architecture not only enhances our project's efficiency and performance but also ensures that our codebase is organized in a way that allows developers to navigate and contribute effectively.

#### Directory Layout

The project is structured into several key directories, each serving specific roles:

```
MIDAZ
 |   bin
 |   chocolatey
 |---   tools
 |   components
 |---   infra
 |---   |   artifacts
 |---   |   grafana
 |---   |   postgres
 |---   |   rabbitmq
 |---   |---   etc
 |---   mdz
 |---   |   internal
 |---   |---   domain
 |---   |---   |   repository
 |---   |---   model
 |---   |---   rest
 |---   |   pkg
 |---   |---   cmd
 |---   |---   |   account
 |---   |---   |---   testdata
 |---   |---   |   asset
 |---   |---   |---   testdata
 |---   |---   |   configure
 |---   |---   |---   testdata
 |---   |---   |---   testdata 2
 |---   |---   |   ledger
 |---   |---   |---   testdata
 |---   |---   |   login
 |---   |---   |   organization
 |---   |---   |---   testdata
 |---   |---   |   portfolio
 |---   |---   |---   testdata
 |---   |---   |   root
 |---   |---   |   segment
 |---   |---   |---   testdata
 |---   |---   |   utils
 |---   |---   |   version
 |---   |---   environment
 |---   |---   factory
 |---   |---   iostreams
 |---   |---   mockutil
 |---   |---   output
 |---   |---   ptr
 |---   |---   setting
 |---   |---   tui
 |---   |   test
 |---   |---   integration
 |---   |---   |   testdata
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
 |---   gold
 |---   |   parser
 |---   |   transaction
 |---   mmodel
 |---   net
 |---   |   http
 |---   shell
 |   postman
 |   scripts
```

#### Common Utilities (`./pkg`)

* `console`: Description of the console utilities and their usage.
* `libLog`: Overview of the logging framework and configuration details.
* `libMongo`, `mpostgres`: Database utilities, including setup and configuration.
* `libPointers`: Explanation of any custom pointer utilities or enhancements used in the project.
* `libZap`: Details on the structured logger adapted for high-performance scenarios.
* `libHTTP`: Information on HTTP helpers and network communication utilities.
* `shell`: Guide on shell utilities, including scripting and automation tools.

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

##### MDZ (`./components/mdz`)

* **Command Line Tools** (`./cmd`): Guides on how to use various command-line tools included in the MDZ component.
* **Packages** (`./pkg`): Information on additional packages provided within the MDZ component.

### Configuration (`./config`)

* **Identity Schemas** (`./identity-schemas`): Guide on setting up and modifying identity schemas.

### Miscellaneous

#### Images (`./image`)

* **README** : Purpose of images stored and how to use them in the project.

#### Postman Collections (`./postman`)

* **Usage** : How to import and use the provided Postman collections for API testing.
