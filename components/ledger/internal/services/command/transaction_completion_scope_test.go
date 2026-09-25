// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// TestBuildTransactionCompletionIntentScopesOnlyScopedTransactions locks the
// compatibility rule of the frozen intent: the transaction scope enters the
// fingerprint only when the engine transaction carries one. A record frozen
// before per-item scope existed names no transaction scope and must stay
// verifiable, and a fingerprint that disagrees with that rule must be refused.
func TestBuildTransactionCompletionIntentScopesOnlyScopedTransactions(t *testing.T) {
	for _, test := range []struct {
		name   string
		scoped bool
	}{
		{name: "scoped transaction", scoped: true},
		{name: "unscoped transaction record", scoped: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, result := recoveryContractFixture(t)
			transaction := accounting.Transaction{
				ID:       payload.TransactionID,
				Postings: []accounting.Posting{{Ref: "source:0", BalanceRef: "@source#default"}},
			}
			if test.scoped {
				transaction.OrganizationID, transaction.LedgerID = payload.OrganizationID, payload.LedgerID
			}

			intent := BuildTransactionCompletionIntent(transaction, payload)
			if test.scoped {
				assert.Equal(t, payload.OrganizationID.String(), intent.OrganizationID)
				assert.Equal(t, payload.LedgerID.String(), intent.LedgerID)
			} else {
				assert.Empty(t, intent.OrganizationID)
				assert.Empty(t, intent.LedgerID)
			}

			execution := completionScopeExecution(t, payload, result, transaction, intent)
			require.NoError(t, ValidateTransactionCompletion(execution))

			drifted := intent
			if test.scoped {
				drifted.OrganizationID, drifted.LedgerID = "", ""
			} else {
				drifted.OrganizationID, drifted.LedgerID = payload.OrganizationID.String(), payload.LedgerID.String()
			}

			execution = completionScopeExecution(t, payload, result, transaction, drifted)
			require.ErrorIs(t, ValidateTransactionCompletion(execution), ErrInvalidTransactionCompletionRecord)
		})
	}
}

// completionScopeExecution freezes payload under the fingerprint of intent and
// assembles the execution the validator receives.
func completionScopeExecution(
	t *testing.T,
	payload TransactionCompletionPlan,
	result accounting.ExecutionResult,
	transaction accounting.Transaction,
	intent EngineTransactionIntent,
) EngineExecution {
	t.Helper()

	fingerprint, err := ComputeEngineIntentFingerprint(EngineIntent{
		TenantID: payload.TenantID, OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID,
		ExecutionID: payload.ExecutionID, Transactions: []EngineTransactionIntent{intent},
	})
	require.NoError(t, err)

	payload.IntentFingerprint = fingerprint
	raw, err := EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)

	return EngineExecution{
		Execution: accounting.Execution{
			OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID, ExecutionID: payload.ExecutionID,
			Balances: result.Final, Transactions: []accounting.Transaction{transaction},
		},
		IntentFingerprint: fingerprint,
		CompletionPlans:   []CompletionPlanRecord{{TransactionID: payload.TransactionID, Payload: raw}},
		Guards:            []ExecutionGuard{{TransactionID: payload.TransactionID, NextToken: "opaque-next"}},
	}
}
