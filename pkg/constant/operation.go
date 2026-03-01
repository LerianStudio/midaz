// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// Operation type constants.
const (
	DEBIT   = "DEBIT"
	CREDIT  = "CREDIT"
	ONHOLD  = "ON_HOLD"
	RELEASE = "RELEASE"

	// TransactionStatusApprovedCompensate is the status assigned to a transaction
	// that has been approved as part of a compensating (reversal) flow.
	TransactionStatusApprovedCompensate = "APPROVED_COMPENSATE"

	// MaxPGParams is PostgreSQL's parameter limit per prepared statement.
	MaxPGParams = 65535
)
