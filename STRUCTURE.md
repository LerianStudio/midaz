# Project Structure Overview

Welcome to the comprehensive guide on the structure of our project, which is designed with a focus on scalability, maintainability, and clear separation of concerns in line with the Command Query Responsibility Segregation (CQRS) pattern. This architecture not only enhances our project's efficiency and performance but also ensures that our codebase is organized in a way that allows developers to navigate and contribute effectively.

#### Directory Layout

The project is structured into several key directories, each serving specific roles:

```
├── common
│   ├── console
│   ├── mlog
│   ├── mmongo
│   ├── mpointers
│   ├── mpostgres
│   ├── mzap
│   ├── net
│   │   └── http
│   └── shell
├── components
│   ├── auth
│   ├── ledger
│   │   ├── api
│   │   ├── internal
│   │   │   ├── adapters
│   │   │   │   └── database
│   │   │   │       ├── mongodb
│   │   │   │       └── postgres
│   │   │   ├── app
│   │   │   │   ├── command
│   │   │   │   └── query
│   │   │   ├── domain
│   │   │   │   ├── metadata
│   │   │   │   ├── onboarding
│   │   │   │   │   ├── ledger
│   │   │   │   │   └── organization
│   │   │   │   └── portfolio
│   │   │   │       ├── account
│   │   │   │       ├── asset
│   │   │   │       ├── portfolio
│   │   │   │       └── cluster
│   │   │   ├── gen
│   │   │   │   └── mock
│   │   │   │       ├── account
│   │   │   │       ├── asset
│   │   │   │       ├── ledger
│   │   │   │       ├── metadata
│   │   │   │       ├── organization
│   │   │   │       ├── portfolio
│   │   │   │       └── cluster
│   │   │   ├── ports
│   │   │   │   └── http
│   │   │   └── service
│   │   ├── migrations
│   │   └── setup
│   └── mdz
│       ├── cmd
│       │   ├── login
│       │   ├── ui
│       │   └── version
│       └── pkg
├── config
│   ├── auth
│   └── identity-schemas
├── image
│   └── README
└── postman

```

#### Common Utilities (`./common`)

* `console`: Description of the console utilities and their usage.
* `mlog`: Overview of the logging framework and configuration details.
* `mmongo`, `mpostgres`: Database utilities, including setup and configuration.
* `mpointers`: Explanation of any custom pointer utilities or enhancements used in the project.
* `mzap`: Details on the structured logger adapted for high-performance scenarios.
* `net/http`: Information on HTTP helpers and network communication utilities.
* `shell`: Guide on shell utilities, including scripting and automation tools.

#### Components (`./components`)

##### Ledger (`./components/ledger`)

###### API (`./ledger/api`)

* **Endpoints** : List and describe all API endpoints, including parameters, request/response formats, and error codes.

###### Internal (`./ledger/internal`)

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
