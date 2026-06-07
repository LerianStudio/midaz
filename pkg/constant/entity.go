// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// Entity type names used for error reporting, metadata tagging, and audit logging.
// These replace reflect.TypeOf(mmodel.Foo{}).Name() calls scattered across the codebase.
const (
	EntityAccount               = "Account"
	EntityAccountRule           = "AccountRule"
	EntityAccountType           = "AccountType"
	EntityAsset                 = "Asset"
	EntityAssetRate             = "AssetRate"
	EntityAuditEvent            = "AuditEvent"
	EntityBalance               = "Balance"
	EntityBillingPackage        = "BillingPackage"
	EntityDataSource            = "DataSource"
	EntityDeadline              = "Deadline"
	EntityFeeCalculation        = "FeeCalculation"
	EntityHolder                = "Holder"
	EntityInstrument            = "Instrument"
	EntityLedger                = "Ledger"
	EntityLimit                 = "Limit"
	EntityOperation             = "Operation"
	EntityOperationRoute        = "OperationRoute"
	EntityOrganization          = "Organization"
	EntityPackage               = "Package"
	EntityPortfolio             = "Portfolio"
	EntityRelatedParty          = "RelatedParty"
	EntityReport                = "Report"
	EntityReservation           = "Reservation"
	EntityRule                  = "Rule"
	EntitySegment               = "Segment"
	EntityTemplate              = "Template"
	EntityTransaction           = "Transaction"
	EntityTransactionRoute      = "TransactionRoute"
	EntityTransactionValidation = "TransactionValidation"
	EntityUsageCounter          = "UsageCounter"
	EntityValidationRequest     = "ValidationRequest"
)
