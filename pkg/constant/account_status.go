// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// Account status codes. Stored in the account.status column and compared
// against at transaction-eligibility time. Keep in sync with the enum in
// pkg/mmodel/status.go and the CHECK constraint intent documented for the
// account table.
//
// PENDING_CRM_LINK and FAILED_CRM_LINK support the Ledger-owned saga that
// coordinates Ledger account creation with CRM holder-alias creation
// (see docs/plans/plan-mode-crm-ledger-abstraction-layer-*.md). An account
// in either of those states — regardless of its balance's AllowSending /
// AllowReceiving flags — must not participate in transactions.
const (
	AccountStatusActive         = "ACTIVE"
	AccountStatusInactive       = "INACTIVE"
	AccountStatusClosed         = "CLOSED"
	AccountStatusPendingCRMLink = "PENDING_CRM_LINK"
	AccountStatusFailedCRMLink  = "FAILED_CRM_LINK"
)
