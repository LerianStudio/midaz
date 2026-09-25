// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

var (
	completionMembersGroupID     = uuid.MustParse("0199a600-0000-7000-8000-000000000001")
	completionMembersForeignTxID = uuid.MustParse("0199a600-0000-7000-8000-000000000002")
	completionMembersForeignOrg  = uuid.MustParse("0199a600-0000-7000-8000-000000000003")
	completionMembersForeignLdg  = uuid.MustParse("0199a600-0000-7000-8000-000000000004")
)

// groupedCompletionPlanFixture returns the contract fixture as a grouped plan
// whose execution also applied one transaction in another ledger.
func groupedCompletionPlanFixture(t *testing.T) TransactionCompletionPlan {
	t.Helper()

	payload, _ := recoveryContractFixture(t)
	payload.GroupID = &completionMembersGroupID
	payload.ExecutionMembers = []TransactionCompletionMember{
		{TransactionID: payload.TransactionID, OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID},
		{TransactionID: completionMembersForeignTxID, OrganizationID: completionMembersForeignOrg, LedgerID: completionMembersForeignLdg},
	}

	var err error
	payload.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)

	return payload
}

func TestTransactionCompletionPlan_UngroupedEncodingIsUnchanged(t *testing.T) {
	payload, _ := recoveryContractFixture(t)

	raw, err := EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)

	golden, err := os.ReadFile("testdata/completion_plan_ungrouped.golden.json")
	require.NoError(t, err)
	assert.Equal(t, string(golden), string(raw),
		"an ungrouped plan must encode byte-for-byte as before the execution member manifest existed")
	assert.NotContains(t, string(raw), "executionMembers")
}

func TestTransactionCompletionPlan_ExecutionMembersRoundTrip(t *testing.T) {
	payload := groupedCompletionPlanFixture(t)

	raw, err := EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)
	assert.Contains(t, string(raw),
		`"executionMembers":[{"transactionId":"`+payload.TransactionID.String()+`","organizationId":"`+payload.OrganizationID.String()+`","ledgerId":"`+payload.LedgerID.String()+`"}`)

	decoded, err := DecodeTransactionCompletionPlan(raw)
	require.NoError(t, err)
	assert.Equal(t, payload.ExecutionMembers, decoded.ExecutionMembers)
}

func TestTransactionCompletionPlan_GroupedPlanWithoutManifestStillDecodes(t *testing.T) {
	payload := groupedCompletionPlanFixture(t)
	payload.ExecutionMembers = nil

	raw, err := EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "executionMembers")

	decoded, err := DecodeTransactionCompletionPlan(raw)
	require.NoError(t, err)
	require.NotNil(t, decoded.GroupID)
	assert.Nil(t, decoded.ExecutionMembers)
}

func TestTransactionCompletionPlan_ExecutionMembersStayOutOfTheIntentFingerprint(t *testing.T) {
	payload := groupedCompletionPlanFixture(t)
	withoutManifest := payload
	withoutManifest.ExecutionMembers = nil

	withFingerprint, err := ComputeEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	withoutFingerprint, err := ComputeEngineIntentFingerprint(recoveryContractIntent(withoutManifest))
	require.NoError(t, err)
	require.Equal(t, withoutFingerprint, withFingerprint)

	assert.Equal(t,
		transactionCompletionIntent(completionMembersTransaction(payload), withoutManifest),
		transactionCompletionIntent(completionMembersTransaction(payload), payload),
		"the manifest is resolution metadata and must not change the engine transaction intent")
}

func TestTransactionCompletionPlan_RejectsMalformedExecutionMembers(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*TransactionCompletionPlan)
	}{
		{name: "manifest on an ungrouped plan", mutate: func(plan *TransactionCompletionPlan) {
			plan.GroupID = nil
		}},
		{name: "explicitly empty manifest", mutate: func(plan *TransactionCompletionPlan) {
			plan.ExecutionMembers = []TransactionCompletionMember{}
		}},
		{name: "own transaction missing", mutate: func(plan *TransactionCompletionPlan) {
			plan.ExecutionMembers = plan.ExecutionMembers[1:]
		}},
		{name: "own transaction in another scope", mutate: func(plan *TransactionCompletionPlan) {
			plan.ExecutionMembers[0].LedgerID = completionMembersForeignLdg
		}},
		{name: "duplicate transaction", mutate: func(plan *TransactionCompletionPlan) {
			plan.ExecutionMembers[1].TransactionID = plan.TransactionID
		}},
		{name: "nil transaction", mutate: func(plan *TransactionCompletionPlan) {
			plan.ExecutionMembers[1].TransactionID = uuid.Nil
		}},
		{name: "nil scope", mutate: func(plan *TransactionCompletionPlan) {
			plan.ExecutionMembers[1].OrganizationID = uuid.Nil
		}},
		{name: "more members than one execution can apply", mutate: func(plan *TransactionCompletionPlan) {
			for len(plan.ExecutionMembers) <= maxPreparedEngineTransactions {
				plan.ExecutionMembers = append(plan.ExecutionMembers, TransactionCompletionMember{
					TransactionID: uuid.New(), OrganizationID: completionMembersForeignOrg, LedgerID: completionMembersForeignLdg,
				})
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := groupedCompletionPlanFixture(t)
			payload.ExecutionMembers = append([]TransactionCompletionMember(nil), payload.ExecutionMembers...)
			test.mutate(&payload)

			_, err := EncodeTransactionCompletionPlan(payload)
			require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
		})
	}

	t.Run("decoder rejects a manifest it would not encode", func(t *testing.T) {
		payload := groupedCompletionPlanFixture(t)
		raw, err := EncodeTransactionCompletionPlan(payload)
		require.NoError(t, err)

		tampered := strings.Replace(string(raw), completionMembersForeignTxID.String(), payload.TransactionID.String(), 1)
		_, err = DecodeTransactionCompletionPlan([]byte(tampered))
		require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)

		unknownMemberField := strings.Replace(string(raw), `"executionMembers":[{`, `"executionMembers":[{"role":"origin",`, 1)
		_, err = DecodeTransactionCompletionPlan([]byte(unknownMemberField))
		require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	})
}

func completionMembersTransaction(payload TransactionCompletionPlan) accounting.Transaction {
	return accounting.Transaction{ID: payload.TransactionID}
}
