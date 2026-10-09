// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package systemfacts

import (
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// The direction values the contract admits.
const (
	directionInflow   = "INFLOW"
	directionOutflow  = "OUTFLOW"
	directionInternal = "INTERNAL"
	directionMixed    = "MIXED"
)

// TransactionCommittedPayload is the data member of a
// midaz.transaction_committed fact, typed independently of the domain so the
// wire shape is what the contract declares and nothing more.
type TransactionCommittedPayload struct {
	Action                   string                          `json:"action"`
	Status                   string                          `json:"status"`
	Direction                string                          `json:"direction"`
	Amount                   string                          `json:"amount"`
	AssetCode                string                          `json:"assetCode"`
	ChartOfAccountsGroupName string                          `json:"chartOfAccountsGroupName,omitempty"`
	Scheme                   string                          `json:"scheme,omitempty"`
	Description              string                          `json:"description,omitempty"`
	ParentTransactionID      string                          `json:"parentTransactionId,omitempty"`
	GroupID                  string                          `json:"groupId,omitempty"`
	CreatedAt                string                          `json:"createdAt"`
	Operations               []TransactionCommittedOperation `json:"operations"`
}

// TransactionCommittedOperation is one leg of the fact: the account is an
// opaque identifier, the alias never leaves the ledger.
type TransactionCommittedOperation struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Amount          string `json:"amount"`
	AssetCode       string `json:"assetCode"`
	ChartOfAccounts string `json:"chartOfAccounts,omitempty"`
	AccountID       string `json:"accountId"`
	External        bool   `json:"external"`
	FeeLeg          bool   `json:"feeLeg"`
}

// NewTransactionCommitted derives the fact payload of a completion write set.
// It reports false, deriving nothing, when the write is not a fact: no
// transaction, an action other than direct, commit or revert (hold and cancel
// never are), or a status other than APPROVED. A missing transaction or leg
// amount is treated the same way: no persistence path writes an APPROVED row
// without them, and the builder must not invent a number.
func NewTransactionCommitted(record command.TransactionWriteSet) (TransactionCommittedPayload, bool) {
	tx := record.Transaction
	if tx == nil || !isFactAction(record.Action) || tx.Status.Code != constant.APPROVED || tx.Amount == nil {
		return TransactionCommittedPayload{}, false
	}

	operations := make([]TransactionCommittedOperation, 0, len(tx.Operations))

	for _, op := range tx.Operations {
		if op == nil || op.Amount.Value == nil {
			return TransactionCommittedPayload{}, false
		}

		operations = append(operations, newTransactionCommittedOperation(op))
	}

	return TransactionCommittedPayload{
		Action:                   record.Action,
		Status:                   constant.APPROVED,
		Direction:                directionOf(operations),
		Amount:                   tx.Amount.String(),
		AssetCode:                tx.AssetCode,
		ChartOfAccountsGroupName: tx.ChartOfAccountsGroupName,
		Scheme:                   tx.Scheme,
		Description:              tx.Description,
		ParentTransactionID:      stringValue(tx.ParentTransactionID),
		GroupID:                  stringValue(tx.GroupID),
		CreatedAt:                tx.CreatedAt.UTC().Format(time.RFC3339Nano),
		Operations:               operations,
	}, true
}

func isFactAction(action string) bool {
	switch action {
	case constant.ActionDirect, constant.ActionCommit, constant.ActionRevert:
		return true
	default:
		return false
	}
}

func newTransactionCommittedOperation(op *operation.Operation) TransactionCommittedOperation {
	return TransactionCommittedOperation{
		ID:              op.ID,
		Type:            op.Type,
		Amount:          op.Amount.Value.String(),
		AssetCode:       op.AssetCode,
		ChartOfAccounts: op.ChartOfAccounts,
		AccountID:       op.AccountID,
		External:        strings.HasPrefix(op.AccountAlias, constant.DefaultExternalAccountAliasPrefix),
		FeeLeg:          isFeeLeg(op.Metadata),
	}
}

// isFeeLeg reads the fee engine's mark, which reaches a row either as the
// string the engine writes or as a boolean once it has crossed a JSON decode.
func isFeeLeg(metadata map[string]any) bool {
	switch mark := metadata[constant.MetadataKeyFeeLeg].(type) {
	case string:
		return mark == constant.MetadataValueFeeLeg
	case bool:
		return mark
	default:
		return false
	}
}

// directionOf classifies the movement by which legs touch the external
// counterparty. Legs are classified by Type, because Direction may be empty
// on a persisted row:
//
//	INFLOW    every DEBIT leg is external and no CREDIT leg is
//	OUTFLOW   every CREDIT leg is external and no DEBIT leg is
//	INTERNAL  no leg is external
//	MIXED     anything else, including a transaction with no DEBIT or no
//	          CREDIT leg when some leg is external
func directionOf(operations []TransactionCommittedOperation) string {
	var debits, externalDebits, credits, externalCredits, externals int

	for _, op := range operations {
		if op.External {
			externals++
		}

		switch op.Type {
		case constant.DEBIT:
			debits++

			if op.External {
				externalDebits++
			}
		case constant.CREDIT:
			credits++

			if op.External {
				externalCredits++
			}
		}
	}

	switch {
	case externals == 0:
		return directionInternal
	case debits == 0 || credits == 0:
		return directionMixed
	case externalDebits == debits && externalCredits == 0:
		return directionInflow
	case externalCredits == credits && externalDebits == 0:
		return directionOutflow
	default:
		return directionMixed
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
