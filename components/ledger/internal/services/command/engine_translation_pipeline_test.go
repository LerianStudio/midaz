// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestEngineTranslationPipelinePreservesRepeatedLegsAndRecovery(t *testing.T) {
	t.Parallel()

	payload, _ := recoveryContractFixture(t)
	payload.TransactionStatus = constant.CREATED
	balance := func(alias, key string, available, version int64) *mmodel.Balance {
		return &mmodel.Balance{
			ID:             uuid.NewSHA1(uuid.NameSpaceOID, []byte(alias+"#"+key)).String(),
			AccountID:      uuid.NewSHA1(uuid.NameSpaceOID, []byte(alias)).String(),
			OrganizationID: payload.OrganizationID.String(), LedgerID: payload.LedgerID.String(),
			Alias: alias, Key: key, AssetCode: "USD", AccountType: "deposit", Direction: constant.DirectionCredit,
			Available: decimal.NewFromInt(available), Version: version, AllowSending: true, AllowReceiving: true,
			CreatedAt: payload.TransactionDate, UpdatedAt: payload.TransactionDate,
		}
	}
	source := balance("@source", constant.DefaultBalanceKey, 50, 7)
	target := balance("@target", constant.DefaultBalanceKey, 0, 4)
	companion := balance("@source", constant.OverdraftBalanceKey, 0, 11)
	companion.Direction = constant.DirectionDebit
	companion.Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal}
	reads := 0
	pool, err := LoadEngineSnapshotPool(context.Background(), payload.OrganizationID, payload.LedgerID,
		[]string{"@source#default", "@target#default"},
		func(_ context.Context, orgID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
			assert.Equal(t, payload.OrganizationID, orgID)
			assert.Equal(t, payload.LedgerID, ledgerID)
			reads++
			if reads == 1 {
				assert.ElementsMatch(t, []string{"@source#default", "@target#default"}, aliases)
				return []*mmodel.Balance{target, source}, nil
			}
			assert.ElementsMatch(t, []string{"@source#overdraft", "@target#overdraft"}, aliases)
			return []*mmodel.Balance{companion}, nil
		})
	require.NoError(t, err)
	require.Equal(t, 2, reads)
	require.Len(t, pool.ExplicitBalances, 2)
	require.Len(t, pool.Snapshots, 3)
	require.NoError(t, rejectInternalScopeBalances(context.Background(), pool.ExplicitBalances))
	require.Error(t, rejectInternalScopeBalances(context.Background(), []*mmodel.Balance{companion}))

	leg := func(alias, description string, isFrom bool) mtransaction.FromTo {
		return mtransaction.FromTo{
			AccountAlias: alias, BalanceKey: constant.DefaultBalanceKey, IsFrom: isFrom,
			Description: description, ChartOfAccounts: "customer", Metadata: map[string]any{"leg": description},
		}
	}
	payload.TransactionInput.Send = mtransaction.Send{
		Asset: "USD", Value: decimal.NewFromInt(60),
		Source: mtransaction.Source{From: []mtransaction.FromTo{
			leg("0#@source#default", "first", true), leg("1#@source#default", "second", true),
		}},
		Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{leg("0#@target#default", "target", false)}},
	}
	payload.Validate = &mtransaction.Responses{
		Asset: "USD", Total: decimal.NewFromInt(60),
		From: map[string]mtransaction.Amount{
			"1#@source#default": {Asset: "USD", Value: decimal.NewFromInt(30), Operation: constant.DEBIT},
			"0#@source#default": {Asset: "USD", Value: decimal.NewFromInt(30), Operation: constant.DEBIT},
		},
		To: map[string]mtransaction.Amount{"0#@target#default": {Asset: "USD", Value: decimal.NewFromInt(60), Operation: constant.CREDIT}},
	}
	translated, projection, err := TranslateEngineTransaction(EngineTranslationInput{
		TransactionID: payload.TransactionID, Action: payload.Action, TransactionStatus: payload.TransactionStatus,
		TransactionInput: payload.TransactionInput, Validate: payload.Validate, Balances: pool.Balances,
	})
	require.NoError(t, err)
	require.Len(t, translated.Postings, 3)
	assert.Equal(t, []string{"from:0:debit", "from:1:debit", "to:0:credit"}, []string{
		translated.Postings[0].Ref, translated.Postings[1].Ref, translated.Postings[2].Ref,
	})
	assert.Equal(t, accounting.DrawAllowed, translated.Postings[0].DrawPolicy)
	assert.Equal(t, accounting.DrawAllowed, translated.Postings[1].DrawPolicy)
	payload.OperationSpecs = projection
	payload.IntentFingerprint, err = ComputeEngineIntentFingerprint(EngineIntent{
		TenantID: payload.TenantID, OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID, ExecutionID: payload.ExecutionID,
		Transactions: []EngineTransactionIntent{transactionCompletionIntent(translated, payload)},
	})
	require.NoError(t, err)
	frozenPayload, err := EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)
	require.NoError(t, ValidateTransactionCompletion(EngineExecution{
		Execution: accounting.Execution{
			OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID, ExecutionID: payload.ExecutionID,
			Transactions: []accounting.Transaction{translated}, Balances: pool.Snapshots,
		},
		IntentFingerprint: payload.IntentFingerprint,
		Guards:            []ExecutionGuard{{TransactionID: payload.TransactionID, NextToken: "approved"}},
		CompletionPlans:   []CompletionPlanRecord{{TransactionID: payload.TransactionID, Payload: frozenPayload}},
	}))

	// This recorded outcome includes debt created under live settings, although
	// the earlier seed had no overdraft configuration. No split is calculated here.
	result := accounting.ExecutionResult{Movements: []accounting.Movement{
		{
			Ref: "first", TransactionID: payload.TransactionID, PostingRef: "from:0:debit", Role: accounting.RolePrimary,
			BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(30),
			Before: accounting.BalanceState{Available: decimal.NewFromInt(50), Version: 7},
			After:  accounting.BalanceState{Available: decimal.NewFromInt(20), Version: 8},
		},
		{
			Ref: "second", TransactionID: payload.TransactionID, PostingRef: "from:1:debit", Role: accounting.RolePrimary,
			BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(20), OverdraftDelta: decimal.NewFromInt(10),
			Before: accounting.BalanceState{Available: decimal.NewFromInt(20), Version: 8},
			After:  accounting.BalanceState{OverdraftUsed: decimal.NewFromInt(10), Version: 9},
		},
		{
			Ref: movementRef(payload.TransactionID, "from:1:debit", accounting.RoleOverdraftCompanion, 0), TransactionID: payload.TransactionID, PostingRef: "from:1:debit", Role: accounting.RoleOverdraftCompanion,
			BalanceRef: "@source#overdraft", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(10),
			Before: accounting.BalanceState{Version: 11}, After: accounting.BalanceState{Available: decimal.NewFromInt(10), Version: 12},
		},
		{
			Ref: "target", TransactionID: payload.TransactionID, PostingRef: "to:0:credit", Role: accounting.RolePrimary,
			BalanceRef: "@target#default", Type: accounting.PostingCredit, Amount: decimal.NewFromInt(60),
			Before: accounting.BalanceState{Version: 4}, After: accounting.BalanceState{Available: decimal.NewFromInt(60), Version: 5},
		},
	}}
	result.Final = recoveryContractFinal(payload, result.Movements)
	rows, err := BuildOperationRecordsFromMovements(payload, result)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	for i, amount := range []string{"30", "20", "10", "60"} {
		assert.Equal(t, amount, rows[i].Amount.Value.String())
	}
	assert.Equal(t, "first", rows[0].Metadata["leg"])
	assert.Equal(t, "second", rows[1].Metadata["leg"])
	assert.NotEqual(t, rows[0].ID, rows[1].ID)
	assert.Equal(t, constant.OVERDRAFT, rows[2].Type)
	assert.Empty(t, rows[2].Metadata)
	assert.Empty(t, rows[2].ChartOfAccounts)
	assert.Equal(t, "0", rows[0].Snapshot.OverdraftUsedAfter)
	assert.Equal(t, "10", rows[1].Snapshot.OverdraftUsedAfter)
	assert.Equal(t, payload.TransactionDate, rows[0].CreatedAt)
	for i, versions := range [][2]int64{{7, 8}, {8, 9}, {11, 12}, {4, 5}} {
		assert.Equal(t, versions[0], *rows[i].Balance.Version)
		assert.Equal(t, versions[1], *rows[i].BalanceAfter.Version)
	}

	envelope := recoveryContractEnvelope(t, payload, result)
	raw, err := EncodeTransactionCompletionRecord(envelope)
	require.NoError(t, err)
	decoded, err := DecodeTransactionCompletionRecord(raw)
	require.NoError(t, err)
	frozen, err := DecodeTransactionCompletionPlan([]byte(decoded.Payload))
	require.NoError(t, err)
	recovered, err := BuildOperationRecordsFromMovements(*frozen, decoded.Result)
	require.NoError(t, err)
	normalJSON, err := json.Marshal(rows)
	require.NoError(t, err)
	recoveredJSON, err := json.Marshal(recovered)
	require.NoError(t, err)
	assert.JSONEq(t, string(normalJSON), string(recoveredJSON))
}

// legRouteLabelPipeline translates a three-leg transfer whose second source leg
// draws overdraft, and returns the frozen plan with the engine outcome it records.
// Each leg carries the free-text route label given in labels, in leg order.
func legRouteLabelPipeline(t *testing.T, labels [3]string) (TransactionCompletionPlan, accounting.ExecutionResult) {
	t.Helper()

	payload, _ := recoveryContractFixture(t)
	payload.TransactionStatus = constant.CREATED
	balance := func(alias, key string, available, version int64) *mmodel.Balance {
		return &mmodel.Balance{
			ID:             uuid.NewSHA1(uuid.NameSpaceOID, []byte(alias+"#"+key)).String(),
			AccountID:      uuid.NewSHA1(uuid.NameSpaceOID, []byte(alias)).String(),
			OrganizationID: payload.OrganizationID.String(), LedgerID: payload.LedgerID.String(),
			Alias: alias, Key: key, AssetCode: "USD", AccountType: "deposit", Direction: constant.DirectionCredit,
			Available: decimal.NewFromInt(available), Version: version, AllowSending: true, AllowReceiving: true,
			CreatedAt: payload.TransactionDate, UpdatedAt: payload.TransactionDate,
		}
	}
	companion := balance("@source", constant.OverdraftBalanceKey, 0, 11)
	companion.Direction = constant.DirectionDebit
	companion.Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal}
	balances := []*mmodel.Balance{balance("@source", constant.DefaultBalanceKey, 50, 7), balance("@target", constant.DefaultBalanceKey, 0, 4), companion}

	leg := func(alias, label string, isFrom bool) mtransaction.FromTo {
		return mtransaction.FromTo{AccountAlias: alias, BalanceKey: constant.DefaultBalanceKey, IsFrom: isFrom, Route: label}
	}
	payload.TransactionInput.Send = mtransaction.Send{
		Asset: "USD", Value: decimal.NewFromInt(60),
		Source: mtransaction.Source{From: []mtransaction.FromTo{
			leg("0#@source#default", labels[0], true), leg("1#@source#default", labels[1], true),
		}},
		Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{leg("0#@target#default", labels[2], false)}},
	}
	payload.Validate = &mtransaction.Responses{
		Asset: "USD", Total: decimal.NewFromInt(60),
		From: map[string]mtransaction.Amount{
			"0#@source#default": {Asset: "USD", Value: decimal.NewFromInt(30), Operation: constant.DEBIT},
			"1#@source#default": {Asset: "USD", Value: decimal.NewFromInt(30), Operation: constant.DEBIT},
		},
		To: map[string]mtransaction.Amount{"0#@target#default": {Asset: "USD", Value: decimal.NewFromInt(60), Operation: constant.CREDIT}},
	}

	translated, projection, err := TranslateEngineTransaction(EngineTranslationInput{
		TransactionID: payload.TransactionID, Action: payload.Action, TransactionStatus: payload.TransactionStatus,
		TransactionInput: payload.TransactionInput, Validate: payload.Validate, Balances: balances,
	})
	require.NoError(t, err)

	payload.OperationSpecs = projection
	payload.IntentFingerprint, err = ComputeEngineIntentFingerprint(EngineIntent{
		TenantID: payload.TenantID, OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID, ExecutionID: payload.ExecutionID,
		Transactions: []EngineTransactionIntent{transactionCompletionIntent(translated, payload)},
	})
	require.NoError(t, err)

	result := accounting.ExecutionResult{Movements: []accounting.Movement{
		{
			Ref: "first", TransactionID: payload.TransactionID, PostingRef: "from:0:debit", Role: accounting.RolePrimary,
			BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(30),
			Before: accounting.BalanceState{Available: decimal.NewFromInt(50), Version: 7},
			After:  accounting.BalanceState{Available: decimal.NewFromInt(20), Version: 8},
		},
		{
			Ref: "second", TransactionID: payload.TransactionID, PostingRef: "from:1:debit", Role: accounting.RolePrimary,
			BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(20), OverdraftDelta: decimal.NewFromInt(10),
			Before: accounting.BalanceState{Available: decimal.NewFromInt(20), Version: 8},
			After:  accounting.BalanceState{OverdraftUsed: decimal.NewFromInt(10), Version: 9},
		},
		{
			Ref: movementRef(payload.TransactionID, "from:1:debit", accounting.RoleOverdraftCompanion, 0), TransactionID: payload.TransactionID, PostingRef: "from:1:debit", Role: accounting.RoleOverdraftCompanion,
			BalanceRef: "@source#overdraft", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(10),
			Before: accounting.BalanceState{Version: 11}, After: accounting.BalanceState{Available: decimal.NewFromInt(10), Version: 12},
		},
		{
			Ref: "target", TransactionID: payload.TransactionID, PostingRef: "to:0:credit", Role: accounting.RolePrimary,
			BalanceRef: "@target#default", Type: accounting.PostingCredit, Amount: decimal.NewFromInt(60),
			Before: accounting.BalanceState{Version: 4}, After: accounting.BalanceState{Available: decimal.NewFromInt(60), Version: 5},
		},
	}}
	result.Final = recoveryContractFinal(payload, result.Movements)

	return payload, result
}

func operationRouteLabels(rows []*operation.Operation) []string {
	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, row.Route) //nolint:staticcheck // the legacy leg label is the behavior under test
	}

	return labels
}

func TestEngineTranslationPipelineRecordsLegRouteLabelOnPrimaryRows(t *testing.T) {
	t.Parallel()

	payload, result := legRouteLabelPipeline(t, [3]string{"", "  overdrawn leg label  ", "target-label"})

	rows, err := BuildOperationRecordsFromMovements(payload, result)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	assert.Equal(t, constant.OVERDRAFT, rows[2].Type)
	assert.Equal(t, []string{"", "  overdrawn leg label  ", "", "target-label"}, operationRouteLabels(rows),
		"each primary row carries its own leg's label verbatim; an unlabeled leg and the overdraft companion of a labeled leg carry none")

	t.Run("a frozen plan keeps its format and recovery projects the same labels", func(t *testing.T) {
		envelope := recoveryContractEnvelope(t, payload, result)
		raw, err := EncodeTransactionCompletionRecord(envelope)
		require.NoError(t, err)
		decoded, err := DecodeTransactionCompletionRecord(raw)
		require.NoError(t, err)

		views, err := BuildTransactionEvidenceViews(*decoded)
		require.NoError(t, err)

		expected := []string{"", "  overdrawn leg label  ", "", "target-label"}
		assert.Equal(t, expected, operationRouteLabels(views.Projection.Transaction.Operations))
		assert.Equal(t, expected, operationRouteLabels(views.Lookup.Operations))
		assert.Equal(t, expected, operationRouteLabels(views.InitialResponse.Operations))

		var frozen struct {
			Projection []map[string]any `json:"projection"`
		}
		require.NoError(t, json.Unmarshal([]byte(envelope.Payload), &frozen))
		require.NotEmpty(t, frozen.Projection)
		for _, spec := range frozen.Projection {
			assert.NotContains(t, spec, "route", "the label rides on the frozen input, never as an operation spec field")
		}
	})
}

func TestEngineTranslationPipelineLeavesLabelEmptyWhenOriginDoesNotResolveTheLeg(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		mutate func(*TransactionCompletionPlan)
	}{
		{"origin reference is not a leg position", func(p *TransactionCompletionPlan) { p.OperationSpecs[0].OriginRef = "from:first" }},
		{"origin position is past the side's legs", func(p *TransactionCompletionPlan) { p.OperationSpecs[0].OriginRef = "from:7" }},
		{"origin names another leg on the same balance", func(p *TransactionCompletionPlan) { p.OperationSpecs[0].OriginRef = "from:1" }},
		{"leg at the origin is another balance", func(p *TransactionCompletionPlan) {
			p.TransactionInput.Send.Source.From[0].AccountAlias = "0#@elsewhere#default"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, result := legRouteLabelPipeline(t, [3]string{"source-label", "second-label", "target-label"})
			tc.mutate(&payload)

			rows, err := BuildOperationRecordsFromMovements(payload, result)
			require.NoError(t, err)
			require.Len(t, rows, 4)
			assert.Equal(t, []string{"", "second-label", "", "target-label"}, operationRouteLabels(rows))
		})
	}
}
