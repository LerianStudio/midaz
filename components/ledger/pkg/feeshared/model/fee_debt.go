// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// FeeDebt is the Fees projection of one engine debt; the engine's live list is the truth.
// Amounts are Decimal128 so changes apply as commuting $inc (rounded past 34 significant
// digits); entry amounts are exact text. OpenedAt stays zero until the opened change lands.
type FeeDebt struct {
	ID                  string          `bson:"_id"`
	OrganizationID      string          `bson:"organization_id"`
	LedgerID            string          `bson:"ledger_id"`
	DebtorBalanceRef    string          `bson:"debtor_balance_ref"`
	CreditBalanceRef    string          `bson:"credit_balance_ref"`
	OriginTransactionID string          `bson:"origin_transaction_id"`
	FeePackageID        string          `bson:"fee_package_id,omitempty"`
	AssetCode           string          `bson:"asset_code"`
	Seq                 int64           `bson:"seq"`
	OpenedAmount        bson.Decimal128 `bson:"opened_amount"`
	Remaining           bson.Decimal128 `bson:"remaining"`
	Entries             []FeeDebtEntry  `bson:"entries"`
	OpenedAt            time.Time       `bson:"opened_at,omitempty"`
	CreatedAt           time.Time       `bson:"created_at"`
	UpdatedAt           time.Time       `bson:"updated_at"`
}

// FeeDebtEntry is one recorded change. Key is "<transactionId>:<postingRef>:<kind>",
// unique within its debt, which is what makes recording a change idempotent.
type FeeDebtEntry struct {
	Key           string    `bson:"key"`
	Kind          string    `bson:"kind"`
	TransactionID string    `bson:"transaction_id"`
	PostingRef    string    `bson:"posting_ref"`
	Amount        string    `bson:"amount"`
	AppliedAt     time.Time `bson:"applied_at"`
}
