// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package shared

import "time"

// Timeouts
const (
	DefaultPollTimeout = 120 * time.Second
	PollInterval       = 2 * time.Second
	PDFExtraTimeout    = 90 * time.Second
	HTTPClientTimeout  = 30 * time.Second
)

// Report statuses (capitalized - matches pkg/constant/report-status.go)
const (
	StatusProcessing = "Processing"
	StatusFinished   = "Finished"
	StatusError      = "Error"
)

// Output formats
const (
	FormatHTML = "html"
	FormatCSV  = "csv"
	FormatXML  = "xml"
	FormatPDF  = "pdf"
	FormatTXT  = "txt"
)

// Core infrastructure credentials
const (
	CoreInfraUsername = "plugin"
	CoreInfraPassword = "Lerian@123"

	// PluginCRMPassword avoids @ in password for MongoDB URI compatibility.
	// Reporter's datasource-config.go doesn't URL-encode passwords in MongoDB connection strings.
	PluginCRMPassword = "testpass123"
)

// DataSource IDs (match DATASOURCE_* env convention)
const (
	DSMidazOnboarding  = "midaz_onboarding"
	DSMidazTransaction = "midaz_transaction"
	DSPluginCRM        = "plugin_crm"
)

// PostgreSQL table names (midaz_onboarding schema)
const (
	TableOrganization = "organization"
	TableLedger       = "ledger"
	TableAccount      = "account"

	// Qualified table names returned by the API (schema.table format via QualifiedName()).
	QualifiedTableOrganization = "public.organization"
	QualifiedTableLedger       = "public.ledger"
	QualifiedTableAccount      = "public.account"
)

// PostgreSQL table names (midaz_transaction schema — shares PG instance with midaz_onboarding)
const (
	TableOperationRoute = "operation_route"
	TableOperation      = "operation"

	// Qualified table names returned by the API (schema.table format via QualifiedName()).
	QualifiedTableOperationRoute = "public.operation_route"
	QualifiedTableOperation      = "public.operation"
)

// MongoDB collection names (plugin_crm)
const (
	CollectionHolders = "holders"
	CollectionAliases = "aliases"

	// PluginCRMMidazOrgID is the Midaz organization ID used to construct
	// plugin_crm collection names (e.g., "holders_<orgID>").
	// The Worker code appends this to the base collection name for plugin_crm queries.
	PluginCRMMidazOrgID = "test-org-001"
)

// RabbitMQ topology
const (
	RabbitExchange      = "reporter.generate-report.exchange"
	RabbitQueue         = "reporter.generate-report.queue"
	RabbitRoutingKey    = "reporter.generate-report.key"
	RabbitDLX           = "reporter.dlx"
	RabbitDLQ           = "reporter.dlq"
	RabbitDLQRoutingKey = "reporter.dlq.key"
)

// Crypto keys for plugin_crm data encryption.
// The Worker decrypts plugin_crm fields at report-generation time, so E2E seeds must encrypt.
// These are distinct keys: one for HMAC hashing, one for AES-256-GCM encryption.
const (
	// TestCryptoHashKey is 32 bytes (64 hex chars) used for HMAC-SHA256 hashing of searchable fields.
	TestCryptoHashKey = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2"

	// TestCryptoEncryptKey is 32 bytes (64 hex chars) used for AES-256-GCM encryption of PII fields.
	TestCryptoEncryptKey = "f1e2d3c4b5a6f7e8d9c0b1a2f3e4d5c6b7a8f9e0d1c2b3a4f5e6d7c8b9a0e1f2"
)

// Deadline types
const (
	DeadlineTypeRegulatory = "regulatory"
	DeadlineTypeCustom     = "custom"
)

// Deadline frequencies
const (
	FrequencyOnce       = "once"
	FrequencyDaily      = "daily"
	FrequencyWeekly     = "weekly"
	FrequencyMonthly    = "monthly"
	FrequencySemiannual = "semiannual"
	FrequencyAnnual     = "annual"
)

// Deadline statuses (computed, not stored)
const (
	DeadlineStatusPending   = "pending"
	DeadlineStatusOverdue   = "overdue"
	DeadlineStatusDelivered = "delivered"
)

// Application ports
const (
	ManagerAPIPort   = 4005
	WorkerHealthPort = 4006
)

// Template fixture paths (relative to testdata/)
const (
	FixtureValidHTML             = "templates/valid_html.tpl"
	FixtureValidCSV              = "templates/valid_csv.tpl"
	FixtureValidXML              = "templates/valid_xml.tpl"
	FixtureValidPDF              = "templates/valid_pdf.tpl"
	FixtureValidTXT              = "templates/valid_txt.tpl"
	FixtureMultiSource           = "templates/multi-source_html.tpl"
	FixtureSchemaQualified       = "templates/schema-qualified_html.tpl"
	FixtureScriptInjection       = "templates/script-injection_html.tpl"
	FixtureEventHandlerInjection = "templates/event-handler-injection_html.tpl"
	FixtureIframeInjection       = "templates/iframe-injection_html.tpl"
	FixtureInvalidField          = "templates/invalid-field_html.tpl"
	FixtureInvalidDatabase       = "templates/invalid-database_html.tpl"
	FixtureInvalidTable          = "templates/invalid-table_html.tpl"
	FixtureCSVContentAsHTML      = "templates/csv-content-as-html.tpl"
	FixtureLFIAttackVectors      = "templates/lfi-attack-vectors_pdf.tpl"
	FixtureEmpty                 = "templates/empty.tpl"
	FixtureNotTPL                = "templates/not-tpl.txt"

	// Template validation test fixtures
	FixtureAccountPDF         = "templates/account_pdf.tpl"
	FixtureACCS005            = "templates/ACCS005.tpl"
	FixtureCadoc4111          = "templates/cadoc-4111.tpl"
	FixtureListAccounts       = "templates/list-accounts_html.tpl"
	FixtureEngineFeatShowcase = "templates/engine-features-showcase_html.tpl"
)
