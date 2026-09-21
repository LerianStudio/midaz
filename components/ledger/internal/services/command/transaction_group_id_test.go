// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestBuildTransactionWriteSet_PreservesGroupID(t *testing.T) {
	t.Parallel()

	groupID := uuid.MustParse("01994f13-29b7-7000-8000-000000000201")
	transactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000202")
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000203")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000204")
	timestamp := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	amount := decimal.NewFromInt(10)

	writeSet, err := BuildTransactionWriteSet(TransactionCompletionPlan{
		FormatVersion:  TransactionCompletionFormatVersion,
		TransactionID:  transactionID,
		GroupID:        &groupID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
			Asset: "BRL",
			Value: amount,
			Source: mtransaction.Source{From: []mtransaction.FromTo{
				{AccountAlias: "@source", Amount: &mtransaction.Amount{Asset: "BRL", Value: amount}, IsFrom: true},
			}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{
				{AccountAlias: "@destination", Amount: &mtransaction.Amount{Asset: "BRL", Value: amount}},
			}},
		}},
		TransactionStatus:    constant.CREATED,
		TransactionCreatedAt: timestamp,
		TransactionUpdatedAt: timestamp,
		OperationUpdatedAt:   timestamp,
	}, accounting.ExecutionResult{})

	require.NoError(t, err)
	require.NotNil(t, writeSet.Transaction.GroupID)
	assert.Equal(t, groupID.String(), *writeSet.Transaction.GroupID)
}
