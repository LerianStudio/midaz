# Project Structure Overview

Welcome to the comprehensive guide on the structure of our project, which is designed with a focus on scalability, maintainability, and clear separation of concerns in line with the Command Query Responsibility Segregation (CQRS) pattern. This architecture not only enhances our project's efficiency and performance but also ensures that our codebase is organized in a way that allows developers to navigate and contribute effectively.

#### Directory Layout

The project is structured into several key directories, each serving specific roles:

```
MIDAZ
.
|-- CHANGELOG.md
|-- CODE_OF_CONDUCT.md
|-- CONTRIBUTING.md
|-- GOVERNANCE.md
|-- LICENSE
|-- Makefile
|-- README.md
|-- SECURITY.md
|-- STRUCTURE.md
|-- SUPPORT.md
|-- bin
|   `-- golangci-lint
|-- chocolatey
|   |-- mdz.nuspec
|   `-- tools
|       |-- LICENSE.txt
|       |-- VERIFICATION.txt
|       |-- chocolateyinstall.ps1
|       `-- chocolateyuninstall.ps1
|-- components
|   |-- audit
|   |   |-- Dockerfile
|   |   |-- Makefile
|   |   |-- api
|   |   |   |-- docs.go
|   |   |   |-- openapi.yaml
|   |   |   |-- swagger.json
|   |   |   `-- swagger.yaml
|   |   |-- artifacts
|   |   |-- cmd
|   |   |   `-- app
|   |   |       `-- main.go
|   |   |-- db
|   |   |   `-- init.sql
|   |   |-- docker-compose.yml
|   |   `-- internal
|   |       |-- adapters
|   |       |   |-- grpc
|   |       |   |   |-- in
|   |       |   |   `-- out
|   |       |   |       |-- trillian.grpc.go
|   |       |   |       `-- trillian.mock.go
|   |       |   |-- http
|   |       |   |   |-- in
|   |       |   |   |   |-- response.go
|   |       |   |   |   |-- routes.go
|   |       |   |   |   |-- swagger.go
|   |       |   |   |   `-- trillian.go
|   |       |   |   `-- out
|   |       |   |-- mongodb
|   |       |   |   `-- audit
|   |       |   |       |-- audit.go
|   |       |   |       |-- audit.mock.go
|   |       |   |       `-- audit.mongodb.go
|   |       |   `-- rabbitmq
|   |       |       |-- consumer.mock.go
|   |       |       `-- consumer.rabbitmq.go
|   |       |-- bootstrap
|   |       |   |-- config.go
|   |       |   |-- consumer.go
|   |       |   |-- server.go
|   |       |   `-- service.go
|   |       `-- services
|   |           |-- create-log.go
|   |           |-- create-log_test.go
|   |           |-- get-audit-info.go
|   |           |-- get-audit-info_test.go
|   |           |-- get-log-by-hash.go
|   |           |-- get-log-by-hash_test.go
|   |           |-- usecase.go
|   |           |-- validate-log-hash.go
|   |           `-- validate-log-hash_test.go
|   |-- auth
|   |   |-- Makefile
|   |   |-- artifacts
|   |   |-- docker-compose.yml
|   |   `-- setup
|   |       |-- 00_init.sql
|   |       `-- init_data.json
|   |-- infra
|   |   |-- Makefile
|   |   |-- artifacts
|   |   |-- docker-compose.yml
|   |   |-- grafana
|   |   |   `-- run-grafana.sh
|   |   |-- postgres
|   |   |   `-- init.sql
|   |   `-- rabbitmq
|   |       `-- etc
|   |           |-- definitions.json
|   |           `-- rabbitmq.conf
|   |-- mdz
|   |   |-- Makefile
|   |   |-- bin
|   |   |   `-- mdz
|   |   |-- internal
|   |   |   |-- domain
|   |   |   |   `-- repository
|   |   |   |       |-- account.go
|   |   |   |       |-- account_mock.go
|   |   |   |       |-- asset.go
|   |   |   |       |-- asset_mock.go
|   |   |   |       |-- auth.go
|   |   |   |       |-- auth_mock.go
|   |   |   |       |-- ledger.go
|   |   |   |       |-- ledger_mock.go
|   |   |   |       |-- organization.go
|   |   |   |       |-- organization_mock.go
|   |   |   |       |-- portfolio.go
|   |   |   |       |-- portfolio_mock.go
|   |   |   |       |-- segment.go
|   |   |   |       `-- segment_mock.go
|   |   |   |-- model
|   |   |   |   |-- auth.go
|   |   |   |   `-- error.go
|   |   |   `-- rest
|   |   |       |-- account.go
|   |   |       |-- account_test.go
|   |   |       |-- asset.go
|   |   |       |-- asset_test.go
|   |   |       |-- auth.go
|   |   |       |-- auth_test.go
|   |   |       |-- ledger.go
|   |   |       |-- ledger_test.go
|   |   |       |-- organization.go
|   |   |       |-- organization_test.go
|   |   |       |-- portfolio.go
|   |   |       |-- portfolio_test.go
|   |   |       |-- segment.go
|   |   |       |-- segment_test.go
|   |   |       `-- utils.go
|   |   |-- main.go
|   |   |-- pkg
|   |   |   |-- cmd
|   |   |   |   |-- account
|   |   |   |   |   |-- account.go
|   |   |   |   |   |-- create.go
|   |   |   |   |   |-- create_test.go
|   |   |   |   |   |-- delete.go
|   |   |   |   |   |-- delete_test.go
|   |   |   |   |   |-- describe.go
|   |   |   |   |   |-- describe_test.go
|   |   |   |   |   |-- list.go
|   |   |   |   |   |-- list_test.go
|   |   |   |   |   |-- testdata
|   |   |   |   |   |   |-- output_describe.golden
|   |   |   |   |   |   `-- output_list.golden
|   |   |   |   |   |-- update.go
|   |   |   |   |   `-- update_test.go
|   |   |   |   |-- asset
|   |   |   |   |   |-- asset.go
|   |   |   |   |   |-- create.go
|   |   |   |   |   |-- create_test.go
|   |   |   |   |   |-- delete.go
|   |   |   |   |   |-- delete_test.go
|   |   |   |   |   |-- describe.go
|   |   |   |   |   |-- describe_test.go
|   |   |   |   |   |-- list.go
|   |   |   |   |   |-- list_test.go
|   |   |   |   |   |-- testdata
|   |   |   |   |   |   |-- output_describe.golden
|   |   |   |   |   |   `-- output_list.golden
|   |   |   |   |   |-- update.go
|   |   |   |   |   `-- update_test.go
|   |   |   |   |-- configure
|   |   |   |   |   |-- configure.go
|   |   |   |   |   |-- configure_test.go
|   |   |   |   |   `-- testdata
|   |   |   |   |       `-- output_configure.golden
|   |   |   |   |-- ledger
|   |   |   |   |   |-- create.go
|   |   |   |   |   |-- create_test.go
|   |   |   |   |   |-- delete.go
|   |   |   |   |   |-- delete_test.go
|   |   |   |   |   |-- describe.go
|   |   |   |   |   |-- describe_test.go
|   |   |   |   |   |-- ledger.go
|   |   |   |   |   |-- list.go
|   |   |   |   |   |-- list_test.go
|   |   |   |   |   |-- testdata
|   |   |   |   |   |   |-- output_describe.golden
|   |   |   |   |   |   `-- output_list.golden
|   |   |   |   |   |-- update.go
|   |   |   |   |   `-- update_test.go
|   |   |   |   |-- login
|   |   |   |   |   |-- browser.go
|   |   |   |   |   |-- login.go
|   |   |   |   |   |-- login_test.go
|   |   |   |   |   `-- term.go
|   |   |   |   |-- organization
|   |   |   |   |   |-- create.go
|   |   |   |   |   |-- create_test.go
|   |   |   |   |   |-- delete.go
|   |   |   |   |   |-- delete_test.go
|   |   |   |   |   |-- describe.go
|   |   |   |   |   |-- describe_test.go
|   |   |   |   |   |-- list.go
|   |   |   |   |   |-- list_test.go
|   |   |   |   |   |-- organization.go
|   |   |   |   |   |-- testdata
|   |   |   |   |   |   |-- output_describe.golden
|   |   |   |   |   |   `-- output_list.golden
|   |   |   |   |   |-- update.go
|   |   |   |   |   `-- update_test.go
|   |   |   |   |-- portfolio
|   |   |   |   |   |-- create.go
|   |   |   |   |   |-- create_test.go
|   |   |   |   |   |-- delete.go
|   |   |   |   |   |-- delete_test.go
|   |   |   |   |   |-- describe.go
|   |   |   |   |   |-- describe_test.go
|   |   |   |   |   |-- list.go
|   |   |   |   |   |-- list_test.go
|   |   |   |   |   |-- portfolio.go
|   |   |   |   |   |-- testdata
|   |   |   |   |   |   |-- output_describe.golden
|   |   |   |   |   |   `-- output_list.golden
|   |   |   |   |   |-- update.go
|   |   |   |   |   `-- update_test.go
|   |   |   |   |-- root
|   |   |   |   |   |-- help.go
|   |   |   |   |   |-- root.go
|   |   |   |   |   `-- root_test.go
|   |   |   |   |-- segment
|   |   |   |   |   |-- create.go
|   |   |   |   |   |-- create_test.go
|   |   |   |   |   |-- delete.go
|   |   |   |   |   |-- delete_test.go
|   |   |   |   |   |-- describe.go
|   |   |   |   |   |-- describe_test.go
|   |   |   |   |   |-- list.go
|   |   |   |   |   |-- list_test.go
|   |   |   |   |   |-- segment.go
|   |   |   |   |   |-- testdata
|   |   |   |   |   |   |-- output_describe.golden
|   |   |   |   |   |   `-- output_list.golden
|   |   |   |   |   |-- update.go
|   |   |   |   |   `-- update_test.go
|   |   |   |   |-- utils
|   |   |   |   |   |-- utils.go
|   |   |   |   |   `-- utils_test.go
|   |   |   |   `-- version
|   |   |   |       |-- version.go
|   |   |   |       `-- version_test.go
|   |   |   |-- environment
|   |   |   |   `-- environment.go
|   |   |   |-- factory
|   |   |   |   `-- factory.go
|   |   |   |-- iostreams
|   |   |   |   `-- iostreams.go
|   |   |   |-- mockutil
|   |   |   |   `-- mockutil.go
|   |   |   |-- output
|   |   |   |   `-- output.go
|   |   |   |-- ptr
|   |   |   |   `-- ptr.go
|   |   |   |-- setting
|   |   |   |   `-- setting.go
|   |   |   `-- tui
|   |   |       |-- input.go
|   |   |       |-- password.go
|   |   |       `-- select.go
|   |   `-- test
|   |       `-- integration
|   |           |-- login_test.go
|   |           |-- mdz.sh
|   |           |-- mdz_test.go
|   |           |-- testdata
|   |           |   `-- out_login_flags.golden
|   |           `-- testutils.go
|   |-- onboarding
|   |   |-- Dockerfile
|   |   |-- Makefile
|   |   |-- api
|   |   |   |-- docs.go
|   |   |   |-- openapi.yaml
|   |   |   |-- swagger.json
|   |   |   `-- swagger.yaml
|   |   |-- artifacts
|   |   |-- cmd
|   |   |   `-- app
|   |   |       `-- main.go
|   |   |-- docker-compose.yml
|   |   |-- internal
|   |   |   |-- adapters
|   |   |   |   |-- http
|   |   |   |   |   |-- in
|   |   |   |   |   |   |-- account.go
|   |   |   |   |   |   |-- asset.go
|   |   |   |   |   |   |-- ledger.go
|   |   |   |   |   |   |-- organization.go
|   |   |   |   |   |   |-- portfolio.go
|   |   |   |   |   |   |-- routes.go
|   |   |   |   |   |   |-- segment.go
|   |   |   |   |   |   `-- swagger.go
|   |   |   |   |   `-- out
|   |   |   |   |-- mongodb
|   |   |   |   |   |-- metadata.go
|   |   |   |   |   |-- metadata.mock.go
|   |   |   |   |   `-- metadata.mongodb.go
|   |   |   |   |-- postgres
|   |   |   |   |   |-- account
|   |   |   |   |   |   |-- account.go
|   |   |   |   |   |   |-- account.mock.go
|   |   |   |   |   |   `-- account.postgresql.go
|   |   |   |   |   |-- asset
|   |   |   |   |   |   |-- asset.go
|   |   |   |   |   |   |-- asset.mock.go
|   |   |   |   |   |   `-- asset.postgresql.go
|   |   |   |   |   |-- ledger
|   |   |   |   |   |   |-- ledger.go
|   |   |   |   |   |   |-- ledger.mock.go
|   |   |   |   |   |   `-- ledger.postgresql.go
|   |   |   |   |   |-- organization
|   |   |   |   |   |   |-- organization.go
|   |   |   |   |   |   |-- organization.mock.go
|   |   |   |   |   |   `-- organization.postgresql.go
|   |   |   |   |   |-- portfolio
|   |   |   |   |   |   |-- portfolio.go
|   |   |   |   |   |   |-- portfolio.mock.go
|   |   |   |   |   |   `-- portfolio.postgresql.go
|   |   |   |   |   `-- segment
|   |   |   |   |       |-- segment.go
|   |   |   |   |       |-- segment.mock.go
|   |   |   |   |       `-- segment.postgresql.go
|   |   |   |   |-- rabbitmq
|   |   |   |   |   |-- producer.mock.go
|   |   |   |   |   `-- producer.rabbitmq.go
|   |   |   |   `-- redis
|   |   |   |       |-- consumer.redis.go
|   |   |   |       `-- redis.mock.go
|   |   |   |-- bootstrap
|   |   |   |   |-- config.go
|   |   |   |   |-- server.go
|   |   |   |   `-- service.go
|   |   |   `-- services
|   |   |       |-- command
|   |   |       |   |-- command.go
|   |   |       |   |-- create-account.go
|   |   |       |   |-- create-account_test.go
|   |   |       |   |-- create-asset.go
|   |   |       |   |-- create-asset_test.go
|   |   |       |   |-- create-ledger.go
|   |   |       |   |-- create-ledger_test.go
|   |   |       |   |-- create-metadata.go
|   |   |       |   |-- create-metadata_test.go
|   |   |       |   |-- create-organization.go
|   |   |       |   |-- create-organization_test.go
|   |   |       |   |-- create-portfolio.go
|   |   |       |   |-- create-portfolio_test.go
|   |   |       |   |-- create-segment.go
|   |   |       |   |-- create-segment_test.go
|   |   |       |   |-- delete-account.go
|   |   |       |   |-- delete-account_test.go
|   |   |       |   |-- delete-asset.go
|   |   |       |   |-- delete-asset_test.go
|   |   |       |   |-- delete-ledger.go
|   |   |       |   |-- delete-ledger_test.go
|   |   |       |   |-- delete-organization.go
|   |   |       |   |-- delete-organization_test.go
|   |   |       |   |-- delete-portfolio.go
|   |   |       |   |-- delete-portfolio_test.go
|   |   |       |   |-- delete-segment.go
|   |   |       |   |-- delete-segment_test.go
|   |   |       |   |-- send-account-queue-transaction.go
|   |   |       |   |-- update-account.go
|   |   |       |   |-- update-account_test.go
|   |   |       |   |-- update-asset.go
|   |   |       |   |-- update-asset_test.go
|   |   |       |   |-- update-ledger.go
|   |   |       |   |-- update-ledger_test.go
|   |   |       |   |-- update-metadata.go
|   |   |       |   |-- update-metadata_test.go
|   |   |       |   |-- update-organization.go
|   |   |       |   |-- update-organization_test.go
|   |   |       |   |-- update-portfolio.go
|   |   |       |   |-- update-portfolio_test.go
|   |   |       |   |-- update-segment.go
|   |   |       |   `-- update-segment_test.go
|   |   |       |-- errors.go
|   |   |       `-- query
|   |   |           |-- get-alias-account.go
|   |   |           |-- get-alias-account_test.go
|   |   |           |-- get-alias-accounts.go
|   |   |           |-- get-alias-accounts_test.go
|   |   |           |-- get-all-accounts.go
|   |   |           |-- get-all-accounts_test.go
|   |   |           |-- get-all-asset.go
|   |   |           |-- get-all-asset_test.go
|   |   |           |-- get-all-ledgers.go
|   |   |           |-- get-all-ledgers_test.go
|   |   |           |-- get-all-metadata-accounts.go
|   |   |           |-- get-all-metadata-accounts_test.go
|   |   |           |-- get-all-metadata-asset.go
|   |   |           |-- get-all-metadata-asset_test.go
|   |   |           |-- get-all-metadata-ledgers.go
|   |   |           |-- get-all-metadata-ledgers_test.go
|   |   |           |-- get-all-metadata-organizations.go
|   |   |           |-- get-all-metadata-organizations_test.go
|   |   |           |-- get-all-metadata-portfolios.go
|   |   |           |-- get-all-metadata-portfolios_test.go
|   |   |           |-- get-all-metadata-segment.go
|   |   |           |-- get-all-metadata-segment_test.go
|   |   |           |-- get-all-organizations.go
|   |   |           |-- get-all-organizations_test.go
|   |   |           |-- get-all-portfolios.go
|   |   |           |-- get-all-portfolios_test.go
|   |   |           |-- get-all-segment.go
|   |   |           |-- get-all-segment_test.go
|   |   |           |-- get-id-account-with-deleted.go
|   |   |           |-- get-id-account-with-deleted_test.go
|   |   |           |-- get-id-account.go
|   |   |           |-- get-id-account_test.go
|   |   |           |-- get-id-asset.go
|   |   |           |-- get-id-asset_test.go
|   |   |           |-- get-id-ledger.go
|   |   |           |-- get-id-ledger_test.go
|   |   |           |-- get-id-organization.go
|   |   |           |-- get-id-organization_test.go
|   |   |           |-- get-id-portfolio.go
|   |   |           |-- get-id-portfolio_test.go
|   |   |           |-- get-id-segment.go
|   |   |           |-- get-id-segment_test.go
|   |   |           |-- get-ids-accounts.go
|   |   |           |-- get-ids-accounts_test.go
|   |   |           `-- query.go
|   |   `-- migrations
|   |       |-- 000000_create_organization_table.down.sql
|   |       |-- 000000_create_organization_table.up.sql
|   |       |-- 000001_create_ledger_table.down.sql
|   |       |-- 000001_create_ledger_table.up.sql
|   |       |-- 000002_create_asset_table.down.sql
|   |       |-- 000002_create_asset_table.up.sql
|   |       |-- 000003_create_segment_table.down.sql
|   |       |-- 000003_create_segment_table.up.sql
|   |       |-- 000004_create_portfolio_table.down.sql
|   |       |-- 000004_create_portfolio_table.up.sql
|   |       |-- 000005_create_account_table.down.sql
|   |       `-- 000005_create_account_table.up.sql
|   `-- transaction
|       |-- Dockerfile
|       |-- Makefile
|       |-- api
|       |   |-- docs.go
|       |   |-- openapi.yaml
|       |   |-- swagger.json
|       |   `-- swagger.yaml
|       |-- artifacts
|       |-- cmd
|       |   `-- app
|       |       `-- main.go
|       |-- docker-compose.yml
|       |-- internal
|       |   |-- adapters
|       |   |   |-- http
|       |   |   |   |-- in
|       |   |   |   |   |-- assetrate.go
|       |   |   |   |   |-- operation.go
|       |   |   |   |   |-- routes.go
|       |   |   |   |   |-- swagger.go
|       |   |   |   |   `-- transaction.go
|       |   |   |   `-- out
|       |   |   |-- mongodb
|       |   |   |   |-- metadata.go
|       |   |   |   |-- metadata.mock.go
|       |   |   |   `-- metadata.mongodb.go
|       |   |   |-- postgres
|       |   |   |   |-- assetrate
|       |   |   |   |   |-- assetrate.go
|       |   |   |   |   |-- assetrate.mock.go
|       |   |   |   |   `-- assetrate.postgresql.go
|       |   |   |   |-- balance
|       |   |   |   |   |-- balance.go
|       |   |   |   |   |-- balance.mock.go
|       |   |   |   |   `-- balance.postgresql.go
|       |   |   |   |-- operation
|       |   |   |   |   |-- operation.go
|       |   |   |   |   |-- operation.mock.go
|       |   |   |   |   `-- operation.postgresql.go
|       |   |   |   `-- transaction
|       |   |   |       |-- transaction.go
|       |   |   |       |-- transaction.mock.go
|       |   |   |       `-- transaction.postgresql.go
|       |   |   |-- rabbitmq
|       |   |   |   |-- consumer.mock.go
|       |   |   |   |-- consumer.rabbitmq.go
|       |   |   |   |-- producer.mock.go
|       |   |   |   `-- producer.rabbitmq.go
|       |   |   `-- redis
|       |   |       |-- consumer.redis.go
|       |   |       `-- redis.mock.go
|       |   |-- bootstrap
|       |   |   |-- config.go
|       |   |   |-- consumer.go
|       |   |   |-- server.go
|       |   |   `-- service.go
|       |   `-- services
|       |       |-- command
|       |       |   |-- command.go
|       |       |   |-- create-assetrate.go
|       |       |   |-- create-assetrate_test.go
|       |       |   |-- create-balance.go
|       |       |   |-- create-idempotency-key.go
|       |       |   |-- create-operation.go
|       |       |   |-- create-operation_test.go
|       |       |   |-- create-transaction.go
|       |       |   |-- create-transaction_test.go
|       |       |   |-- send-log-transaction-audit-queue.go
|       |       |   |-- update-balance.go
|       |       |   |-- update-operation.go
|       |       |   |-- update-operation_test.go
|       |       |   |-- update-transaction.go
|       |       |   `-- update-transaction_test.go
|       |       |-- errors.go
|       |       `-- query
|       |           |-- get-all-assetrates-assetcode.go
|       |           |-- get-all-assetrates-assetcode_test.go
|       |           |-- get-all-metadata-transactions.go
|       |           |-- get-all-metadata-transactions_test.go
|       |           |-- get-all-operations-account.go
|       |           |-- get-all-operations-account_test.go
|       |           |-- get-all-operations.go
|       |           |-- get-all-operations_test.go
|       |           |-- get-all-transactions.go
|       |           |-- get-all-transactions_test.go
|       |           |-- get-balances.go
|       |           |-- get-external-id-assetrate.go
|       |           |-- get-external-id-assetrate_test.go
|       |           |-- get-id-operation-account.go
|       |           |-- get-id-operation-account_test.go
|       |           |-- get-id-operation.go
|       |           |-- get-id-operation_test.go
|       |           |-- get-id-transaction.go
|       |           |-- get-id-transaction_test.go
|       |           |-- get-parent-id-transaction.go
|       |           |-- get-parent-id-transaction_test.go
|       |           `-- query.go
|       `-- migrations
|           |-- 000000_create_transaction_table.down.sql
|           |-- 000000_create_transaction_table.up.sql
|           |-- 000001_create_operation_table.down.sql
|           |-- 000001_create_operation_table.up.sql
|           |-- 000002_create_asset_rate_table.down.sql
|           |-- 000002_create_asset_rate_table.up.sql
|           |-- 000003_create_balance_table.down.sql
|           `-- 000003_create_balance_table.up.sql
|-- go.mod
|-- go.sum
|-- image
|   `-- README
|       `-- midaz-banner.png
|-- make.sh
|-- pkg
|   |-- app.go
|   |-- app_mock.go
|   |-- app_test.go
|   |-- constant
|   |   |-- account.go
|   |   |-- asset_transaction.go
|   |   |-- errors.go
|   |   |-- http.go
|   |   |-- metadata.go
|   |   |-- operation.go
|   |   |-- pagination.go
|   |   `-- transaction.go
|   |-- context.go
|   |-- context_test.go
|   |-- errors.go
|   |-- errors_test.go
|   |-- gold
|   |   |-- Transaction.g4
|   |   |-- parser
|   |   |   |-- Transaction.interp
|   |   |   |-- Transaction.tokens
|   |   |   |-- TransactionLexer.interp
|   |   |   |-- TransactionLexer.tokens
|   |   |   |-- transaction_base_listener.go
|   |   |   |-- transaction_base_visitor.go
|   |   |   |-- transaction_lexer.go
|   |   |   |-- transaction_listener.go
|   |   |   |-- transaction_parser.go
|   |   |   `-- transaction_visitor.go
|   |   `-- transaction
|   |       |-- error.go
|   |       |-- error_test.go
|   |       |-- model
|   |       |   |-- transaction.go
|   |       |   `-- validations.go
|   |       |-- parse.go
|   |       |-- parse_test.go
|   |       `-- validate.go
|   |-- mcasdoor
|   |   |-- casdoor.go
|   |   |-- casdoor_test.go
|   |   `-- certificates
|   |       `-- token_jwt_key.pem
|   |-- mlog
|   |   |-- log.go
|   |   |-- log_mock.go
|   |   `-- nil.go
|   |-- mmodel
|   |   |-- account.go
|   |   |-- account_test.go
|   |   |-- asset.go
|   |   |-- balance.go
|   |   |-- ledger.go
|   |   |-- organization.go
|   |   |-- portfolio.go
|   |   |-- queue.go
|   |   |-- segment.go
|   |   |-- status.go
|   |   `-- status_test.go
|   |-- mmongo
|   |   `-- mongo.go
|   |-- mopentelemetry
|   |   `-- otel.go
|   |-- mpointers
|   |   |-- pointers.go
|   |   `-- pointers_test.go
|   |-- mpostgres
|   |   |-- pagination.go
|   |   `-- postgres.go
|   |-- mrabbitmq
|   |   `-- rabbitmq.go
|   |-- mredis
|   |   `-- redis.go
|   |-- mtrillian
|   |   `-- trillian.go
|   |-- mzap
|   |   |-- injector.go
|   |   |-- injector_test.go
|   |   |-- zap.go
|   |   `-- zap_test.go
|   |-- net
|   |   `-- http
|   |       |-- cursor.go
|   |       |-- cursor_test.go
|   |       |-- errors.go
|   |       |-- handler.go
|   |       |-- headers.go
|   |       |-- httputils.go
|   |       |-- proxy.go
|   |       |-- response.go
|   |       |-- withBasicAuth.go
|   |       |-- withBody.go
|   |       |-- withBody_test.go
|   |       |-- withCORS.go
|   |       |-- withJWT.go
|   |       |-- withLogging.go
|   |       `-- withTelemetry.go
|   |-- os.go
|   |-- os_test.go
|   |-- shell
|   |   |-- ascii.sh
|   |   |-- colors.sh
|   |   `-- logo.txt
|   |-- stringUtils.go
|   |-- stringUtils_test.go
|   |-- time.go
|   |-- time_test.go
|   |-- utils.go
|   |-- utils_mock.go
|   `-- utils_test.go
|-- postman
|   `-- MIDAZ.postman_collection.json
|-- revive.toml
`-- scripts
    |-- coverage.sh
    `-- coverage_ignore.txt

```

#### Common Utilities (`./pkg`)

* `console`: Description of the console utilities and their usage.
* `mlog`: Overview of the logging framework and configuration details.
* `mmongo`, `mpostgres`: Database utilities, including setup and configuration.
* `mpointers`: Explanation of any custom pointer utilities or enhancements used in the project.
* `mzap`: Details on the structured logger adapted for high-performance scenarios.
* `net/http`: Information on HTTP helpers and network communication utilities.
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
