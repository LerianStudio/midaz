# Project Structure Overview

This guide covers the project structure. The codebase is designed for scalability, maintainability, and clear separation of concerns following the Command Query Responsibility Segregation (CQRS) pattern. This architecture keeps the codebase organized so developers can navigate and contribute effectively.

#### Directory Layout

The project is structured into key directories, each serving specific roles:

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
 |   |   |---   seaweedfs
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
 |   |---   tracer
 |   |   |---   api
 |   |   |---   cmd
 |   |   |   |---   app
 |   |   |---   internal
 |   |   |---   migrations
 |   |---   reporter-manager
 |   |   |---   api
 |   |   |---   cmd
 |   |   |   |---   app
 |   |   |---   internal
 |   |---   reporter-worker
 |   |   |---   cmd
 |   |   |   |---   app
 |   |   |---   internal
 |   image
 |   |---   README
 |   pkg
 |   |---   constant
 |   |---   gold
 |   |---   mbootstrap
 |   |---   mmodel
 |   |---   mongo
 |   |---   mtransaction
 |   |---   net
 |   |---   pagination
 |   |---   reporter
 |   |---   repository
 |   |---   shell
 |   |---   streaming
 |   |---   utils
 |   postman
 |   scripts
 |   tests
 |   |---   chaos
 |   |---   helpers
 |   |---   reporter
 |   |---   utils
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

###### Components (`./components/crm`)

* **Adapters** (`./components/crm/adapters`):
  * **HTTP**: Inbound HTTP handlers for CRM operations.
  * **MongoDB**: Database connection and operations for CRM data persistence.
* **Services** (`./components/crm/services`):
  * Business logic services for customer relationship management operations.

##### Ledger (`./components/ledger`)

The unified ledger deploy unit (`:3002`) folds the former separate `onboarding`
and `transaction` components into one binary.

###### API (`./onboarding/api`)

* **Endpoints:** List and describe all API endpoints, including parameters, request/response formats, and error codes.

###### Internal (`./onboarding/internal`)

* **Adapters** (`./adapters`):
  * **Database:** Connection and operation guides for MongoDB and PostgreSQL.
* **Application Logic** (`./app`):
  * **Command:** Documentation of command handlers, including how commands are processed.
  * **Query:** Details on query handlers, how queries are executed, and their return structures.
* **Domain** (`./domain`):
  * Description of domain models such as Onboarding, Portfolio, Transaction, etc., and their relationships.
* **Services** (`./service`):
  * Detailed information on business logic services, their roles, and interactions in the application.

##### Tracer (`./components/tracer`)

Real-time transaction validation and fraud-prevention API (`:4020`). Hexagonal +
CQRS, CEL rule engine, hash-chained audit log. Ships its own migrations under
`./components/tracer/migrations`.

##### Reporter (`./components/reporter-manager` + `./components/reporter-worker`)

Reporter is **two deploy units**, co-located via the Option C split (shared
library extracted to `pkg/reporter`, shared suites to `tests/reporter`):

* **Reporter Manager** (`./components/reporter-manager`, `:4005`): the REST API
  that accepts report-generation requests and publishes jobs to RabbitMQ
  (`reporter.generate-report.{exchange,queue,key}`). Ships as a distroless image.
* **Reporter Worker** (`./components/reporter-worker`, `:4006`): the async
  consumer that renders PDFs via headless Chromium (chromedp) and writes output
  to S3-compatible object storage. Ships as a fat alpine image with the Chromium
  userland (cannot be distroless — R20).

Both services attach to the shared `infra-network` and use midaz's shared
Mongo / Valkey / RabbitMQ (4.1.3) backing services.

* **Shared library** (`./pkg/reporter`): datasource/fetcher, PDF/pongo rendering,
  template builder, S3 (seaweedfs) and storage adapters, multi-tenant helpers —
  imported by both reporter deploy units.
* **Shared suites** (`./tests/reporter`): `e2e`, `integration`, `property`,
  `fuzzy`, `chaos`, and `utils` test trees for the reporter components.

**Object storage / autoscaling:** the SeaweedFS S3 config is staged at
`./components/infra/seaweedfs/` (`s3.json`, `init-bucket.sh`), but the SeaweedFS
service definition and KEDA autoscaling are reporter-only net-new infra owned by
**Phase 8** — they are not part of the shared infra compose yet.

### Configuration (`./config`)

* **Identity Schemas** (`./identity-schemas`): Guide on setting up and modifying identity schemas.

### Miscellaneous

#### Images (`./image`)

* **README:** Purpose of images stored and how to use them in the project.

#### Postman Collections (`./postman`)

* **Usage:** How to import and use the provided Postman collections for API testing.
