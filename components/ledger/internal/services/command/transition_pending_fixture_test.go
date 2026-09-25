// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// pendingTransaction builds the minimal PENDING transaction the transition accepts: a
// balanced single-leg body and the PENDING status the not-pending guard checks.
func pendingTransaction(skipTracer bool) *transaction.Transaction {
	amount := decimal.NewFromInt(100)

	body := mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "BRL",
			Value: amount,
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{
					AccountAlias: "@payer",
					IsFrom:       true,
					Amount:       &mtransaction.Amount{Asset: "BRL", Value: amount},
				}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{
					AccountAlias: "@payee",
					Amount:       &mtransaction.Amount{Asset: "BRL", Value: amount},
				}},
			},
		},
	}

	if skipTracer {
		body.Skip = &mtransaction.TransactionSkip{Tracer: true}
	}

	return &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		AssetCode:      "BRL",
		Amount:         &amount,
		Status:         transaction.Status{Code: constant.PENDING},
		Body:           body,
	}
}

// pendingTransitionInputFor names the transaction a fake reader will answer with.
func pendingTransitionInputFor(tran *transaction.Transaction) PendingTransitionInput {
	return PendingTransitionInput{
		OrganizationID: uuid.MustParse(tran.OrganizationID),
		LedgerID:       uuid.MustParse(tran.LedgerID),
		TransactionID:  tran.IDtoUUID(),
	}
}
