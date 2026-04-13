// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// Entity type names used for error reporting, metadata tagging, and audit logging.
// These replace reflect.TypeOf(mmodel.Foo{}).Name() calls scattered across the codebase.
const (
	EntityAccount          = "Account"
	EntityAccountRule      = "AccountRule"
	EntityAccountType      = "AccountType"
	EntityAsset            = "Asset"
	EntityAssetRate        = "AssetRate"
	EntityBalance          = "Balance"
	EntityLedger           = "Ledger"
	EntityOperationRoute   = "OperationRoute"
	EntityOrganization     = "Organization"
	EntityPortfolio        = "Portfolio"
	EntitySegment          = "Segment"
	EntityTransaction      = "Transaction"
	EntityTransactionRoute = "TransactionRoute"
)
