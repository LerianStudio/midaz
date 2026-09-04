// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func recoveryContractFixture(t *testing.T) (BalanceEngineRecoveryPayload, engine.Result) {
	t.Helper()
	date := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	payload := BalanceEngineRecoveryPayload{
		FormatVersion: BalanceEngineRecoveryVersion, HeaderID: "correlation-header", TenantID: "tenant-a",
		OrganizationID: uuid.MustParse("33333333-3333-4333-8333-333333333333"), LedgerID: uuid.MustParse("44444444-4444-4444-8444-444444444444"),
		TransactionID: uuid.MustParse("66666666-6666-4666-8666-666666666666"), ExecutionID: uuid.MustParse("88888888-8888-4888-8888-888888888888"),
		TransactionDate: date, TTL: date.Add(time.Hour), TransactionStatus: constant.APPROVED, Action: "direct",
		TransactionInput: mtransaction.Transaction{Description: "original intent", Send: mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(30)}},
		Validate:         &mtransaction.Responses{From: map[string]mtransaction.Amount{"@source": {Value: decimal.NewFromInt(30)}}},
	}
	before := engine.BalanceState{Available: decimal.NewFromInt(100)}
	after := engine.BalanceState{Available: decimal.NewFromInt(70), Version: 1}
	balance := rowContractBalance(rowContractLeg{Alias: "@source", Key: "default"}, rowContractState{"100", "0", "0", 0})
	routeID := "55555555-5555-4555-8555-555555555555"
	payload.Projection = []FrozenProjectionContext{{
		TransactionID: payload.TransactionID, PostingRef: "source:0", BalanceRef: "@source#default", Role: engine.RolePrimary,
		Side: ProjectionSideFrom, RowType: constant.DEBIT, Direction: constant.DirectionDebit,
		RouteID: &routeID, RouteCode: "DEBIT-USD", RouteDescription: "Customer debit", Description: "frozen description",
		ChartOfAccounts: "customer", Metadata: map[string]any{"purpose": "transfer"}, Balance: FrozenProjectionBalance(*balance),
		RequestedAmount: decimal.NewFromInt(30), CompatibilityPath: ProjectionStandard,
	}}
	var err error
	payload.IntentFingerprint, err = ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	result := engine.Result{Movements: []engine.Movement{{
		Ref: "movement:0", TransactionID: payload.TransactionID, PostingRef: "source:0", Role: engine.RolePrimary,
		BalanceRef: "@source#default", Type: engine.PostingDebit, Amount: decimal.NewFromInt(30), Before: before, After: after,
	}}}
	result.Final = recoveryContractFinal(payload, result.Movements)
	return payload, result
}

func recoveryContractIntent(payload BalanceEngineRecoveryPayload) BalanceEngineIntent {
	projection := make([]FrozenProjectionIntent, 0, len(payload.Projection))
	refs := make([]string, 0, len(payload.Projection))
	for _, context := range payload.Projection {
		projection = append(projection, context.Intent())
		if context.Role == engine.RolePrimary {
			refs = append(refs, context.PostingRef)
		}
	}
	return BalanceEngineIntent{
		TenantID: payload.TenantID, OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID, ExecutionID: payload.ExecutionID,
		Transactions: []BalanceEngineTransactionIntent{{
			TransactionID: payload.TransactionID, Action: payload.Action, TransactionStatus: payload.TransactionStatus,
			ParentTransactionID: payload.ParentTransactionID, FeesSkipped: payload.FeesSkipped, TracerSkipped: payload.TracerSkipped,
			TransactionDate: payload.TransactionDate, Input: payload.TransactionInput, PostingRefs: refs, Projection: projection,
		}},
	}
}

func recoveryContractFinal(payload BalanceEngineRecoveryPayload, movements []engine.Movement) []engine.BalanceSnapshot {
	final := make([]engine.BalanceSnapshot, 0)
	indices := make(map[string]int)
	for _, movement := range movements {
		index, exists := indices[movement.BalanceRef]
		if !exists {
			index = len(final)
			indices[movement.BalanceRef] = index
			var balance mmodel.Balance
			for _, context := range payload.Projection {
				if context.BalanceRef == movement.BalanceRef {
					balance = mmodel.Balance(context.Balance)
					break
				}
			}
			final = append(final, engine.BalanceSnapshot{
				BalanceRef: movement.BalanceRef, ID: uuid.MustParse(balance.ID), AccountID: uuid.MustParse(balance.AccountID),
				Alias: balance.Alias, Key: balance.Key, AssetCode: balance.AssetCode, AccountType: balance.AccountType, Direction: balance.Direction,
			})
		}
		final[index].Available, final[index].OnHold, final[index].OverdraftUsed, final[index].Version = movement.After.Available, movement.After.OnHold, movement.After.OverdraftUsed, movement.After.Version
	}
	return final
}

func recoveryContractEnvelope(t *testing.T, payload BalanceEngineRecoveryPayload, result engine.Result) BalanceEngineRecoveryEnvelope {
	t.Helper()
	raw, err := EncodeBalanceEngineRecoveryPayload(payload)
	require.NoError(t, err)
	return BalanceEngineRecoveryEnvelope{
		FormatVersion: BalanceEngineRecoveryVersion, TenantID: payload.TenantID, OrganizationID: payload.OrganizationID,
		LedgerID: payload.LedgerID, TransactionID: payload.TransactionID, ExecutionID: payload.ExecutionID, IntentFingerprint: payload.IntentFingerprint,
		Payload: string(raw), Result: result,
	}
}

func recoveryContractAddRepaymentCompanion(payload *BalanceEngineRecoveryPayload, result *engine.Result, primaryIndex int) {
	primary := result.Movements[primaryIndex]
	var companion FrozenProjectionContext
	for _, context := range payload.Projection {
		if context.PostingRef == primary.PostingRef && context.Role == engine.RolePrimary {
			companion = context
			break
		}
	}
	companion.Role, companion.RowType, companion.BalanceRef = engine.RoleOverdraftCompanion, constant.OVERDRAFT, "@source#overdraft"
	companion.CompatibilityPath = ProjectionStandard
	companion.Metadata, companion.ChartOfAccounts = nil, ""
	companion.Balance.ID = "99999999-9999-4999-8999-999999999999"
	companion.Balance.Key, companion.Balance.Direction = "overdraft", constant.DirectionDebit
	companion.Balance.Available, companion.Balance.OnHold, companion.Balance.OverdraftUsed = primary.OverdraftDelta.Abs(), decimal.Zero, decimal.Zero
	payload.Projection = append(payload.Projection, companion)
	movement := engine.Movement{
		Ref: "companion:" + primary.PostingRef, TransactionID: payload.TransactionID, PostingRef: primary.PostingRef,
		Role: engine.RoleOverdraftCompanion, BalanceRef: companion.BalanceRef, Type: engine.PostingCredit, Amount: primary.OverdraftDelta.Abs(),
		Before: engine.BalanceState{Available: primary.OverdraftDelta.Abs()}, After: engine.BalanceState{Version: 1},
	}
	result.Movements = append(result.Movements, engine.Movement{})
	copy(result.Movements[primaryIndex+2:], result.Movements[primaryIndex+1:])
	result.Movements[primaryIndex+1] = movement
}

func TestBalanceEngineRecoveryRoundTripAndProjection(t *testing.T) {
	payload, result := recoveryContractFixture(t)
	envelope := recoveryContractEnvelope(t, payload, result)
	encoded, err := EncodeBalanceEngineRecoveryEnvelope(envelope)
	require.NoError(t, err)
	decoded, err := DecodeBalanceEngineRecoveryEnvelope(encoded)
	require.NoError(t, err)
	require.Equal(t, envelope.Payload, decoded.Payload)
	decodedPayload, err := DecodeBalanceEngineRecoveryPayload([]byte(decoded.Payload))
	require.NoError(t, err)
	before, err := json.Marshal(result)
	require.NoError(t, err)
	normal, err := ProjectBalanceEngineOperations(payload, result)
	require.NoError(t, err)
	replay, err := ProjectBalanceEngineOperations(*decodedPayload, decoded.Result)
	require.NoError(t, err)
	normalJSON, err := json.Marshal(normal)
	require.NoError(t, err)
	replayJSON, err := json.Marshal(replay)
	require.NoError(t, err)
	assert.Equal(t, string(normalJSON), string(replayJSON))
	require.Len(t, normal, 1)
	assert.Equal(t, "DEBIT-USD", *normal[0].RouteCode)
	assert.Equal(t, payload.TransactionDate, normal[0].UpdatedAt)
	assert.Equal(t, rowContractGolden{
		constant.DEBIT, "@source", "default", "30", constant.DirectionDebit, *payload.Projection[0].RouteID,
		rowContractState{"100", "0", "0", 0},
		rowContractState{"70", "0", "0", 1},
		"0", "0", true,
	}, rowContractObserved(t, normal[0]))
	after, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Equal(t, before, after, "projection must not rewrite real movements")
	normal[0].Metadata["purpose"] = "changed"
	*normal[0].RouteID = "changed"
	assert.Equal(t, "transfer", payload.Projection[0].Metadata["purpose"])
	assert.NotEqual(t, "changed", *payload.Projection[0].RouteID)
}

func TestBalanceEngineRecoveryStrictPayload(t *testing.T) {
	payload, _ := recoveryContractFixture(t)
	raw, err := EncodeBalanceEngineRecoveryPayload(payload)
	require.NoError(t, err)
	for name, input := range map[string]string{
		"unsupported version": strings.Replace(string(raw), `"formatVersion":2`, `"formatVersion":3`, 1),
		"unknown field":       strings.TrimSuffix(string(raw), "}") + `,"unknown":true}`,
		"duplicate version":   strings.TrimSuffix(string(raw), "}") + `,"formatVersion":2}`,
		"escaped duplicate":   strings.TrimSuffix(string(raw), "}") + `,"format\u0056ersion":2}`,
		"trailing object":     string(raw) + `{}`,
		"null":                `null`, "array": `[]`, "malformed": `{"formatVersion":`,
		"projection object": strings.Replace(string(raw), `"projection":[`, `"projection":{`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeBalanceEngineRecoveryPayload([]byte(input))
			require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
		})
	}
}

func TestBalanceEngineRecoveryCorrelation(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*EngineExecution)
	}{
		{"missing guard", func(e *EngineExecution) { e.Guards = nil }},
		{"wrong guard", func(e *EngineExecution) { e.Guards[0].TransactionID = e.Request.ExecutionID }},
		{"no guard progress", func(e *EngineExecution) { e.Guards[0].ExpectedToken = e.Guards[0].NextToken }},
		{"wrong recovery transaction", func(e *EngineExecution) { e.Recovery[0].TransactionID = e.Request.ExecutionID }},
		{"different execution", func(e *EngineExecution) { e.Request.ExecutionID = e.Request.OrganizationID }},
		{"different ledger", func(e *EngineExecution) { e.Request.LedgerID = e.Request.OrganizationID }},
		{"different intent", func(e *EngineExecution) { e.IntentFingerprint = strings.Repeat("a", 64) }},
		{"changed frozen intent with original fingerprint", func(e *EngineExecution) {
			payload, err := DecodeBalanceEngineRecoveryPayload(e.Recovery[0].Payload)
			require.NoError(t, err)
			payload.Action = "cancel"
			e.Recovery[0].Payload, err = EncodeBalanceEngineRecoveryPayload(*payload)
			require.NoError(t, err)
		}},
		{"foreign projection account", func(e *EngineExecution) {
			payload, err := DecodeBalanceEngineRecoveryPayload(e.Recovery[0].Payload)
			require.NoError(t, err)
			payload.Projection[0].Balance.AccountID = "99999999-9999-4999-8999-999999999999"
			e.Recovery[0].Payload, err = EncodeBalanceEngineRecoveryPayload(*payload)
			require.NoError(t, err)
		}},
		{"different posting", func(e *EngineExecution) { e.Request.Transactions[0].Postings[0].Ref = "other" }},
		{"different balance", func(e *EngineExecution) { e.Request.Transactions[0].Postings[0].BalanceRef = "other" }},
		{"duplicate posting", func(e *EngineExecution) {
			e.Request.Transactions[0].Postings = append(e.Request.Transactions[0].Postings, e.Request.Transactions[0].Postings[0])
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			payload, result := recoveryContractFixture(t)
			raw, err := EncodeBalanceEngineRecoveryPayload(payload)
			require.NoError(t, err)
			execution := EngineExecution{
				Request: engine.Request{
					OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID, ExecutionID: payload.ExecutionID,
					Balances:     result.Final,
					Transactions: []engine.Transaction{{ID: payload.TransactionID, Postings: []engine.Posting{{Ref: "source:0", BalanceRef: "@source#default"}}}},
				},
				IntentFingerprint: payload.IntentFingerprint, Recovery: []RecoveryIntent{{TransactionID: payload.TransactionID, Payload: raw}},
				Guards: []ExecutionGuard{{TransactionID: payload.TransactionID, NextToken: "opaque-next"}},
			}
			require.NoError(t, ValidateBalanceEngineRecovery(execution))
			scenario.mutate(&execution)
			require.ErrorIs(t, ValidateBalanceEngineRecovery(execution), ErrInvalidBalanceEngineRecovery)
		})
	}
}

func TestBalanceEngineRecoveryIntentFingerprint(t *testing.T) {
	payload, _ := recoveryContractFixture(t)
	before, err := ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	payload.Projection[0].Balance.Available = decimal.NewFromInt(999)
	payload.Projection[0].Balance.Version = 999
	payload.Validate.From["@source"] = mtransaction.Amount{Value: decimal.NewFromInt(1), OverdraftAmount: decimal.NewFromInt(29)}
	after, err := ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	companion := payload.Projection[0]
	companion.Role, companion.BalanceRef = engine.RoleOverdraftCompanion, "@source#overdraft"
	companion.Metadata, companion.ChartOfAccounts = nil, ""
	payload.Projection = append(payload.Projection, companion)
	withCompanion, err := ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	assert.Equal(t, before, withCompanion, "engine-derived companion need must not change immutable intent")
	for _, scenario := range []struct {
		name   string
		mutate func(*BalanceEngineRecoveryPayload)
	}{
		{"amount", func(p *BalanceEngineRecoveryPayload) { p.TransactionInput.Send.Value = decimal.NewFromInt(31) }},
		{"route", func(p *BalanceEngineRecoveryPayload) { p.Projection[0].RouteCode = "OTHER" }},
		{"metadata", func(p *BalanceEngineRecoveryPayload) { p.Projection[0].Metadata["purpose"] = "other" }},
		{"action", func(p *BalanceEngineRecoveryPayload) { p.Action = "cancel" }},
		{"date", func(p *BalanceEngineRecoveryPayload) { p.TransactionDate = p.TransactionDate.Add(time.Second) }},
		{"parent transaction", func(p *BalanceEngineRecoveryPayload) { parent := p.OrganizationID; p.ParentTransactionID = &parent }},
		{"fees skipped", func(p *BalanceEngineRecoveryPayload) { p.FeesSkipped = true }},
		{"tracer skipped", func(p *BalanceEngineRecoveryPayload) { p.TracerSkipped = true }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			payload, _ := recoveryContractFixture(t)
			scenario.mutate(&payload)
			changed, err := ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
			require.NoError(t, err)
			assert.NotEqual(t, before, changed)
		})
	}
}

func TestBalanceEngineRecoveryFrozenAuditRoundTrip(t *testing.T) {
	for _, parentPresent := range []bool{false, true} {
		for _, feesSkipped := range []bool{false, true} {
			for _, tracerSkipped := range []bool{false, true} {
				t.Run(fmt.Sprintf("parent=%t/fees=%t/tracer=%t", parentPresent, feesSkipped, tracerSkipped), func(t *testing.T) {
					payload, result := recoveryContractFixture(t)
					if parentPresent {
						parent := uuid.MustParse("99999999-9999-4999-8999-999999999999")
						payload.ParentTransactionID = &parent
					}
					payload.FeesSkipped, payload.TracerSkipped = feesSkipped, tracerSkipped
					var err error
					payload.IntentFingerprint, err = ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
					require.NoError(t, err)
					envelope := recoveryContractEnvelope(t, payload, result)
					raw, err := EncodeBalanceEngineRecoveryEnvelope(envelope)
					require.NoError(t, err)
					decoded, err := DecodeBalanceEngineRecoveryEnvelope(raw)
					require.NoError(t, err)
					frozen, err := DecodeBalanceEngineRecoveryPayload([]byte(decoded.Payload))
					require.NoError(t, err)
					assert.Equal(t, payload.ParentTransactionID, frozen.ParentTransactionID)
					assert.Equal(t, feesSkipped, frozen.FeesSkipped)
					assert.Equal(t, tracerSkipped, frozen.TracerSkipped)
					fingerprint, err := ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(*frozen))
					require.NoError(t, err)
					assert.Equal(t, payload.IntentFingerprint, fingerprint)
					assert.Contains(t, decoded.Payload, `"parentTransactionId":`)
					assert.Contains(t, decoded.Payload, fmt.Sprintf(`"feesSkipped":%t`, feesSkipped))
					assert.Contains(t, decoded.Payload, fmt.Sprintf(`"tracerSkipped":%t`, tracerSkipped))
				})
			}
		}
	}
}

func TestBalanceEngineRecoveryRejectsInvalidParent(t *testing.T) {
	for _, name := range []string{"zero", "self"} {
		t.Run(name, func(t *testing.T) {
			payload, _ := recoveryContractFixture(t)
			parent := uuid.Nil
			if name == "self" {
				parent = payload.TransactionID
			}
			payload.ParentTransactionID = &parent
			_, err := EncodeBalanceEngineRecoveryPayload(payload)
			require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
			_, err = ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
			require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
			raw, err := json.Marshal(payload)
			require.NoError(t, err)
			_, err = DecodeBalanceEngineRecoveryPayload(raw)
			require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
		})
	}
}

func TestBalanceEngineRecoveryDeterministicOperationIDs(t *testing.T) {
	payload, _ := recoveryContractFixture(t)
	id, err := DeterministicOperationID(payload.ExecutionID, payload.TransactionID, "a:0\x00primary", engine.RolePrimary, 0)
	require.NoError(t, err)
	replay, err := DeterministicOperationID(payload.ExecutionID, payload.TransactionID, "a:0\x00primary", engine.RolePrimary, 0)
	require.NoError(t, err)
	assert.Equal(t, id, replay)
	assert.Equal(t, uuid.Version(5), id.Version())
	other, err := DeterministicOperationID(payload.OrganizationID, payload.TransactionID, "a:0\x00primary", engine.RolePrimary, 0)
	require.NoError(t, err)
	assert.NotEqual(t, id, other)
	ordinal, err := DeterministicOperationID(payload.ExecutionID, payload.TransactionID, "a:0\x00primary", engine.RolePrimary, 1)
	require.NoError(t, err)
	assert.NotEqual(t, id, ordinal)
	_, err = DeterministicOperationID(uuid.Nil, payload.TransactionID, "a", engine.RolePrimary, 0)
	require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
}

func TestBalanceEngineRecoveryCancelProjection(t *testing.T) {
	payload, _ := recoveryContractFixture(t)
	payload.Action, payload.TransactionStatus = "cancel", constant.CANCELED
	context := payload.Projection[0]
	context.OriginRef, context.PostingRef = "source-leg:0", "source-leg:0:release"
	context.RowType, context.CompatibilityPath = constant.RELEASE, ProjectionValidatedCancelRelease
	context.RequestedAmount = decimal.NewFromInt(50)
	context.Balance.Available, context.Balance.OnHold, context.Balance.OverdraftUsed = decimal.Zero, decimal.NewFromInt(50), decimal.NewFromInt(50)
	context.Balance.Version = 2
	credit := context
	credit.PostingRef, credit.RowType, credit.Direction, credit.CompatibilityPath = "source-leg:0:credit", constant.CREDIT, constant.DirectionCredit, ProjectionValidatedCancelCredit
	payload.Projection = []FrozenProjectionContext{context, credit}
	result := engine.Result{Movements: []engine.Movement{
		{
			Ref: "release", TransactionID: payload.TransactionID, PostingRef: context.PostingRef, BalanceRef: context.BalanceRef, Role: engine.RolePrimary,
			Type: engine.PostingUnreserve, Amount: decimal.NewFromInt(50), Before: engine.BalanceState{OnHold: decimal.NewFromInt(50), OverdraftUsed: decimal.NewFromInt(50), Version: 2}, After: engine.BalanceState{OverdraftUsed: decimal.NewFromInt(50), Version: 3},
		},
		{
			Ref: "credit", TransactionID: payload.TransactionID, PostingRef: credit.PostingRef, BalanceRef: credit.BalanceRef, Role: engine.RolePrimary,
			Type: engine.PostingCredit, Amount: decimal.Zero, OverdraftDelta: decimal.NewFromInt(-50), Before: engine.BalanceState{OverdraftUsed: decimal.NewFromInt(50), Version: 3}, After: engine.BalanceState{Version: 4},
		},
	}}
	recoveryContractAddRepaymentCompanion(&payload, &result, 1)
	result.Final = recoveryContractFinal(payload, result.Movements)
	rows, err := ProjectBalanceEngineOperations(payload, result)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	want := []rowContractGolden{
		{constant.RELEASE, "@source", "default", "50", constant.DirectionDebit, *context.RouteID, rowContractState{"0", "50", "50", 2}, rowContractState{"0", "0", "0", 3}, "50", "0", true},
		{constant.CREDIT, "@source", "default", "50", constant.DirectionCredit, *context.RouteID, rowContractState{"0", "0", "50", 3}, rowContractState{"50", "0", "0", 4}, "50", "0", true},
	}
	for i, row := range rows[:2] {
		assert.Equal(t, want[i], rowContractObserved(t, row))
	}
	assert.True(t, result.Final[0].Available.IsZero(), "real state must not be replaced by the synthetic credit row")
	encoded, err := EncodeBalanceEngineRecoveryEnvelope(recoveryContractEnvelope(t, payload, result))
	require.NoError(t, err)
	decoded, err := DecodeBalanceEngineRecoveryEnvelope(encoded)
	require.NoError(t, err)
	thawed, err := DecodeBalanceEngineRecoveryPayload([]byte(decoded.Payload))
	require.NoError(t, err)
	replayed, err := ProjectBalanceEngineOperations(*thawed, decoded.Result)
	require.NoError(t, err)
	normalJSON, err := json.Marshal(rows)
	require.NoError(t, err)
	replayedJSON, err := json.Marshal(replayed)
	require.NoError(t, err)
	assert.Equal(t, string(normalJSON), string(replayedJSON))
}

func TestBalanceEngineRecoveryCompanionProjection(t *testing.T) {
	payload, result := recoveryContractFixture(t)
	primary := payload.Projection[0]
	primary.RowType, primary.Direction, primary.Side = constant.CREDIT, constant.DirectionCredit, ProjectionSideTo
	primary.RequestedAmount = decimal.NewFromInt(50)
	primary.Balance.Available, primary.Balance.OverdraftUsed = decimal.Zero, decimal.NewFromInt(50)
	companion := primary
	companion.Role, companion.RowType, companion.BalanceRef = engine.RoleOverdraftCompanion, constant.OVERDRAFT, "@source#overdraft"
	companion.Metadata, companion.ChartOfAccounts = nil, ""
	companion.Balance.Key, companion.Balance.Direction = "overdraft", constant.DirectionDebit
	companion.Balance.Available, companion.Balance.OverdraftUsed = decimal.NewFromInt(50), decimal.Zero
	payload.Projection = []FrozenProjectionContext{primary, companion}
	result.Movements[0].Type, result.Movements[0].Amount = engine.PostingCredit, decimal.Zero
	result.Movements[0].Before = engine.BalanceState{OverdraftUsed: decimal.NewFromInt(50)}
	result.Movements[0].After = engine.BalanceState{Version: 1}
	result.Movements[0].OverdraftDelta = decimal.NewFromInt(-50)
	result.Movements = append(result.Movements, engine.Movement{
		Ref: "companion", TransactionID: payload.TransactionID, PostingRef: primary.PostingRef, Role: engine.RoleOverdraftCompanion,
		BalanceRef: companion.BalanceRef, Type: engine.PostingCredit, Amount: decimal.NewFromInt(50), Before: engine.BalanceState{Available: decimal.NewFromInt(50)}, After: engine.BalanceState{Version: 1},
	})
	result.Final = recoveryContractFinal(payload, result.Movements)
	rows, err := ProjectBalanceEngineOperations(payload, result)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.True(t, rows[0].Amount.Value.IsZero())
	assert.Equal(t, "50", rows[1].Amount.Value.String())
	assert.Equal(t, rows[0].RouteID, rows[1].RouteID)
	assert.Equal(t, rows[0].Snapshot, rows[1].Snapshot)
	assert.Empty(t, rows[1].Metadata)
	assert.Empty(t, rows[1].ChartOfAccounts)
	missing := result
	missing.Movements = result.Movements[:1]
	missing.Final = result.Final[:1]
	_, err = ProjectBalanceEngineOperations(payload, missing)
	require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery, "a debt change requires its recorded companion")
	for _, scenario := range []struct {
		name   string
		mutate func(*engine.Result)
	}{
		{"wrong companion direction", func(r *engine.Result) { r.Movements[1].Type = engine.PostingDebit }},
		{"unexpected companion", func(r *engine.Result) {
			r.Movements[0].Before.OverdraftUsed = decimal.Zero
			r.Movements[0].OverdraftDelta = decimal.Zero
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			invalid := result
			invalid.Movements = append([]engine.Movement(nil), result.Movements...)
			scenario.mutate(&invalid)
			_, err := ProjectBalanceEngineOperations(payload, invalid)
			require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
		})
	}
	for _, scenario := range []struct {
		name   string
		mutate func(*FrozenProjectionContext)
	}{
		{"different account", func(c *FrozenProjectionContext) { c.Balance.AccountID = "99999999-9999-4999-8999-999999999999" }},
		{"different route", func(c *FrozenProjectionContext) { c.RouteID = nil }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			invalid := payload
			invalid.Projection = append([]FrozenProjectionContext(nil), payload.Projection...)
			scenario.mutate(&invalid.Projection[1])
			_, err := ProjectBalanceEngineOperations(invalid, result)
			require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
		})
	}
}

func TestBalanceEngineRecoveryRejectsUnrelatedResult(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*engine.Result)
	}{
		{"foreign transaction", func(r *engine.Result) { r.Movements[0].TransactionID = uuid.Nil }},
		{"unknown posting", func(r *engine.Result) { r.Movements[0].PostingRef = "unknown" }},
		{"unknown role", func(r *engine.Result) { r.Movements[0].Role = "unknown" }},
		{"unknown posting type", func(r *engine.Result) { r.Movements[0].Type = "unknown" }},
		{"unknown balance", func(r *engine.Result) { r.Movements[0].BalanceRef = "unknown" }},
		{"missing movement", func(r *engine.Result) { r.Movements = []engine.Movement{} }},
		{"duplicate movement", func(r *engine.Result) { r.Movements = append(r.Movements, r.Movements[0]) }},
		{"different final state", func(r *engine.Result) { r.Final[0].Available = decimal.NewFromInt(999) }},
		{"different final identity", func(r *engine.Result) { r.Final[0].ID = uuid.Nil }},
		{"different final account type", func(r *engine.Result) { r.Final[0].AccountType = "external" }},
		{"inconsistent debt delta", func(r *engine.Result) { r.Movements[0].OverdraftDelta = decimal.NewFromInt(30) }},
		{"missing final state", func(r *engine.Result) { r.Final = []engine.BalanceSnapshot{} }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			payload, result := recoveryContractFixture(t)
			scenario.mutate(&result)
			_, err := ProjectBalanceEngineOperations(payload, result)
			require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
		})
	}
}

func TestBalanceEngineRecoveryMultipleTransactionsKeepIntermediateState(t *testing.T) {
	first, firstResult := recoveryContractFixture(t)
	second, secondResult := recoveryContractFixture(t)
	second.TransactionID = uuid.MustParse("99999999-9999-4999-8999-999999999999")
	second.Projection[0].TransactionID = second.TransactionID
	second.Projection[0].RequestedAmount = decimal.NewFromInt(20)
	second.TransactionInput.Send.Value = decimal.NewFromInt(20)
	secondResult.Movements[0].TransactionID = second.TransactionID
	secondResult.Movements[0].Amount = decimal.NewFromInt(20)
	secondResult.Movements[0].Before = firstResult.Movements[0].After
	secondResult.Movements[0].After = engine.BalanceState{Available: decimal.NewFromInt(50), Version: 2}
	secondResult.Final = recoveryContractFinal(second, secondResult.Movements)
	intent := recoveryContractIntent(first)
	intent.Transactions = append(intent.Transactions, recoveryContractIntent(second).Transactions...)
	fingerprint, err := ComputeBalanceEngineIntentFingerprint(intent)
	require.NoError(t, err)
	first.IntentFingerprint, second.IntentFingerprint = fingerprint, fingerprint
	input := EngineExecution{IntentFingerprint: fingerprint, Request: engine.Request{OrganizationID: first.OrganizationID, LedgerID: first.LedgerID, ExecutionID: first.ExecutionID}}
	for _, payload := range []BalanceEngineRecoveryPayload{first, second} {
		raw, err := EncodeBalanceEngineRecoveryPayload(payload)
		require.NoError(t, err)
		input.Recovery = append(input.Recovery, RecoveryIntent{TransactionID: payload.TransactionID, Payload: raw})
		input.Guards = append(input.Guards, ExecutionGuard{TransactionID: payload.TransactionID, NextToken: payload.TransactionID.String()})
		input.Request.Transactions = append(input.Request.Transactions, engine.Transaction{ID: payload.TransactionID, Postings: []engine.Posting{{Ref: "source:0", BalanceRef: "@source#default"}}})
	}
	require.NoError(t, ValidateBalanceEngineRecovery(input))
	firstRows, err := ProjectBalanceEngineOperations(first, firstResult)
	require.NoError(t, err)
	secondRows, err := ProjectBalanceEngineOperations(second, secondResult)
	require.NoError(t, err)
	assert.Equal(t, "70", firstRows[0].BalanceAfter.Available.String())
	assert.Equal(t, "70", secondRows[0].Balance.Available.String())
	assert.Equal(t, "50", secondRows[0].BalanceAfter.Available.String())
	assert.NotEqual(t, firstRows[0].ID, secondRows[0].ID)
	_, err = EncodeBalanceEngineRecoveryEnvelope(recoveryContractEnvelope(t, first, secondResult))
	require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
	second.TenantID = ""
	raw, err := EncodeBalanceEngineRecoveryPayload(second)
	require.NoError(t, err)
	input.Recovery[1].Payload = raw
	require.ErrorIs(t, ValidateBalanceEngineRecovery(input), ErrInvalidBalanceEngineRecovery)
	first.TenantID = ""
	raw, err = EncodeBalanceEngineRecoveryPayload(first)
	require.NoError(t, err)
	input.Recovery[0].Payload = raw
	intent = recoveryContractIntent(first)
	intent.Transactions = append(intent.Transactions, recoveryContractIntent(second).Transactions...)
	fingerprint, err = ComputeBalanceEngineIntentFingerprint(intent)
	require.NoError(t, err)
	input.IntentFingerprint = fingerprint
	for index := range input.Recovery {
		payload, err := DecodeBalanceEngineRecoveryPayload(input.Recovery[index].Payload)
		require.NoError(t, err)
		payload.IntentFingerprint = fingerprint
		input.Recovery[index].Payload, err = EncodeBalanceEngineRecoveryPayload(*payload)
		require.NoError(t, err)
	}
	require.NoError(t, ValidateBalanceEngineRecovery(input), "an empty authenticated single-tenant scope is supported")
}

func TestBalanceEngineRecoveryRepeatedAliasUsesOriginNotAlias(t *testing.T) {
	payload, _ := recoveryContractFixture(t)
	payload.Action, payload.TransactionStatus = "cancel", constant.CANCELED
	base := payload.Projection[0]
	base.RequestedAmount = decimal.NewFromInt(50)
	base.Balance.Available, base.Balance.OnHold, base.Balance.OverdraftUsed, base.Balance.Version = decimal.Zero, decimal.NewFromInt(100), decimal.NewFromInt(50), 2
	payload.Projection = nil
	result := engine.Result{Movements: []engine.Movement{}}
	states := []engine.BalanceState{
		{OnHold: decimal.NewFromInt(100), OverdraftUsed: decimal.NewFromInt(50), Version: 2},
		{OnHold: decimal.NewFromInt(50), OverdraftUsed: decimal.NewFromInt(50), Version: 3},
		{OnHold: decimal.NewFromInt(50), Version: 4},
		{Version: 5},
		{Available: decimal.NewFromInt(50), Version: 6},
	}
	for index, ref := range []string{"first-release", "first-credit", "second-release", "second-credit"} {
		context := base
		context.PostingRef = ref
		context.OriginRef = "first"
		if index >= 2 {
			context.OriginRef = "second"
			route := "77777777-7777-4777-8777-777777777777"
			context.RouteID = &route
		}
		kind, amount := engine.PostingUnreserve, decimal.NewFromInt(50)
		context.RowType, context.Direction, context.CompatibilityPath = constant.RELEASE, constant.DirectionDebit, ProjectionValidatedCancelRelease
		if index%2 == 1 {
			kind = engine.PostingCredit
			context.RowType, context.Direction, context.CompatibilityPath = constant.CREDIT, constant.DirectionCredit, ProjectionValidatedCancelCredit
		}
		if index == 1 {
			amount = decimal.Zero
		}
		payload.Projection = append(payload.Projection, context)
		result.Movements = append(result.Movements, engine.Movement{
			Ref: ref, TransactionID: payload.TransactionID, PostingRef: ref, Role: engine.RolePrimary,
			BalanceRef: base.BalanceRef, Type: kind, Amount: amount, Before: states[index], After: states[index+1],
			OverdraftDelta: states[index+1].OverdraftUsed.Sub(states[index].OverdraftUsed),
		})
	}
	recoveryContractAddRepaymentCompanion(&payload, &result, 1)
	result.Final = recoveryContractFinal(payload, result.Movements)
	rows, err := ProjectBalanceEngineOperations(payload, result)
	require.NoError(t, err)
	require.Len(t, rows, 5)
	for index, row := range rows[:4] {
		assert.EqualValues(t, index+2, *row.Balance.Version)
		assert.EqualValues(t, index+3, *row.BalanceAfter.Version)
		if index < 2 {
			assert.Equal(t, "50", row.Snapshot.OverdraftUsedBefore)
			assert.Equal(t, *base.RouteID, *row.RouteID)
		} else {
			assert.Equal(t, "0", row.Snapshot.OverdraftUsedBefore)
			assert.Equal(t, "77777777-7777-4777-8777-777777777777", *row.RouteID)
		}
	}
	assert.Equal(t, "50", rows[1].BalanceAfter.OnHold.String())
	assert.Equal(t, "0", rows[3].BalanceAfter.OnHold.String())
	assert.Equal(t, "50", rows[3].BalanceAfter.Available.String())
}

func TestBalanceEngineRecoveryPostingPaths(t *testing.T) {
	state := func(a, h string, version int64) rowContractState { return rowContractState{a, h, "0", version} }
	for _, scenario := range []struct {
		name, action, status, amount string
		states                       []rowContractState
		types                        []engine.PostingType
		rows, directions, paths      []string
	}{
		{"direct credit", "direct", constant.APPROVED, "30", []rowContractState{state("100", "0", 0), state("130", "0", 1)}, []engine.PostingType{engine.PostingCredit}, []string{constant.CREDIT}, []string{constant.DirectionCredit}, []string{ProjectionStandard}},
		{"pending off", "hold", constant.PENDING, "60", []rowContractState{state("100", "0", 0), state("40", "60", 1)}, []engine.PostingType{engine.PostingHold}, []string{constant.ONHOLD}, []string{constant.DirectionDebit}, []string{ProjectionStandard}},
		{"pending on", "hold", constant.PENDING, "60", []rowContractState{state("100", "0", 0), state("40", "0", 1), state("40", "60", 2)}, []engine.PostingType{engine.PostingDebit, engine.PostingReserve}, []string{constant.DEBIT, constant.ONHOLD}, []string{constant.DirectionDebit, constant.DirectionCredit}, []string{ProjectionValidatedHoldDebit, ProjectionValidatedHoldReserve}},
		{"commit off", "commit", constant.APPROVED, "60", []rowContractState{state("40", "60", 1), state("40", "0", 2)}, []engine.PostingType{engine.PostingUnreserve}, []string{constant.DEBIT}, []string{constant.DirectionDebit}, []string{ProjectionStandard}},
		{"commit on", "commit", constant.APPROVED, "60", []rowContractState{state("40", "60", 2), state("40", "0", 3)}, []engine.PostingType{engine.PostingUnreserve}, []string{constant.ONHOLD}, []string{constant.DirectionDebit}, []string{ProjectionStandard}},
		{"cancel off", "cancel", constant.CANCELED, "60", []rowContractState{state("40", "60", 1), state("100", "0", 2)}, []engine.PostingType{engine.PostingRelease}, []string{constant.RELEASE}, []string{constant.DirectionCredit}, []string{ProjectionStandard}},
		{"cancel on", "cancel", constant.CANCELED, "60", []rowContractState{state("40", "60", 2), state("40", "0", 3), state("100", "0", 4)}, []engine.PostingType{engine.PostingUnreserve, engine.PostingCredit}, []string{constant.RELEASE, constant.CREDIT}, []string{constant.DirectionDebit, constant.DirectionCredit}, []string{ProjectionValidatedCancelRelease, ProjectionValidatedCancelCredit}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			payload, _ := recoveryContractFixture(t)
			payload.Action, payload.TransactionStatus = scenario.action, scenario.status
			base := payload.Projection[0]
			payload.Projection = nil
			result := engine.Result{Movements: []engine.Movement{}}
			for index, postingType := range scenario.types {
				context := base
				context.PostingRef = fmt.Sprintf("source:%d", index)
				context.RowType, context.Direction, context.CompatibilityPath = scenario.rows[index], scenario.directions[index], scenario.paths[index]
				context.RequestedAmount = decimal.RequireFromString(scenario.amount)
				if context.CompatibilityPath != ProjectionStandard {
					context.OriginRef = "source-leg"
				}
				payload.Projection = append(payload.Projection, context)
				before, after := scenario.states[index], scenario.states[index+1]
				result.Movements = append(result.Movements, engine.Movement{
					Ref: context.PostingRef, PostingRef: context.PostingRef, TransactionID: payload.TransactionID,
					Role: engine.RolePrimary, BalanceRef: context.BalanceRef, Type: postingType, Amount: context.RequestedAmount,
					Before: engine.BalanceState{Available: decimal.RequireFromString(before.Available), OnHold: decimal.RequireFromString(before.OnHold), Version: before.Version},
					After:  engine.BalanceState{Available: decimal.RequireFromString(after.Available), OnHold: decimal.RequireFromString(after.OnHold), Version: after.Version},
				})
			}
			result.Final = recoveryContractFinal(payload, result.Movements)
			rows, err := ProjectBalanceEngineOperations(payload, result)
			require.NoError(t, err)
			require.Len(t, rows, len(scenario.rows))
			for index, row := range rows {
				want := rowContractGolden{
					scenario.rows[index], "@source", "default", scenario.amount, scenario.directions[index], *base.RouteID,
					scenario.states[index], scenario.states[index+1], "0", "0", true,
				}
				assert.Equal(t, want, rowContractObserved(t, row))
			}
		})
	}
}

func TestBalanceEngineRecoveryLosslessPrecision(t *testing.T) {
	payload, result := recoveryContractFixture(t)
	const version int64 = 9007199254740993
	amount := decimal.RequireFromString("0.00000000000000000001")
	payload.Projection[0].RequestedAmount = amount
	payload.Projection[0].Metadata["counter"] = json.Number("9007199254740993")
	result.Movements[0].Amount = amount
	result.Movements[0].Before = engine.BalanceState{Available: decimal.RequireFromString("1.00000000000000000001"), Version: version}
	result.Movements[0].After = engine.BalanceState{Available: decimal.NewFromInt(1), Version: version + 1}
	result.Final = recoveryContractFinal(payload, result.Movements)
	raw, err := EncodeBalanceEngineRecoveryEnvelope(recoveryContractEnvelope(t, payload, result))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"version":9007199254740993`)
	decoded, err := DecodeBalanceEngineRecoveryEnvelope(raw)
	require.NoError(t, err)
	assert.Equal(t, version, decoded.Result.Movements[0].Before.Version)
	assert.Equal(t, "0.00000000000000000001", decoded.Result.Movements[0].Amount.String())
	thawed, err := DecodeBalanceEngineRecoveryPayload([]byte(decoded.Payload))
	require.NoError(t, err)
	assert.Equal(t, json.Number("9007199254740993"), thawed.Projection[0].Metadata["counter"])
	rows, err := ProjectBalanceEngineOperations(*thawed, decoded.Result)
	require.NoError(t, err)
	assert.Equal(t, version+1, *rows[0].BalanceAfter.Version)
	assert.Equal(t, "0.00000000000000000001", rows[0].Amount.Value.String())
}
