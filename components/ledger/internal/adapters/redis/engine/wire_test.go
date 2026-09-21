// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libZap "github.com/LerianStudio/lib-observability/v4/zap"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	ledgerin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	ledgerfee "github.com/LerianStudio/midaz/v4/components/ledger/pkg/fee"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	feemodel "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestPrepareExecutionDeterministicLosslessWire(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	original, err := json.Marshal(input)
	require.NoError(t, err)
	first, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	second, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Contains(t, string(first.Payload), `"completionPlan":`)
	require.NotContains(t, string(first.Payload), `"recoveryPayload":`)
	after, err := json.Marshal(input)
	require.NoError(t, err)
	require.Equal(t, original, after, "preparation must not mutate domain input")

	var wire wireRequest
	require.NoError(t, json.Unmarshal(first.Payload, &wire))
	require.Equal(t, 3, wire.ProtocolVersion)
	require.Equal(t, resolved.TenantID, wire.TenantID)
	require.Equal(t, input.Execution.OrganizationID.String(), wire.OrganizationID)
	require.Equal(t, input.Execution.LedgerID.String(), wire.LedgerID)
	require.Equal(t, input.Execution.ExecutionID.String(), wire.ExecutionID)
	require.Equal(t, input.IntentFingerprint, wire.IntentFingerprint)
	accountID := input.Execution.Balances[0].AccountID
	require.Equal(t, []string{
		resolved.Schedule, resolved.Recovery, resolved.Receipts, resolved.Guards, resolved.Protection, resolved.TransactionIndex, resolved.Evidence,
		resolved.Balances["@source#default"].Balance, resolved.Balances["@source#default"].Deleted,
		resolved.Balances["@source#default"].LegacyDeleted,
		resolved.Accounts[accountID].Closing, resolved.Accounts[accountID].Closed, resolved.Accounts[accountID].Ownership,
	}, first.Keys)
	require.Equal(t, 1, wire.ScheduleKeyIndex)
	require.Equal(t, 2, wire.RecoveryKeyIndex)
	require.Equal(t, 3, wire.ReceiptKeyIndex)
	require.Equal(t, 4, wire.GuardKeyIndex)
	require.Equal(t, 5, wire.ProtectionKeyIndex)
	require.Equal(t, 6, wire.TransactionIndexKeyIndex)
	require.Equal(t, 7, wire.EvidenceKeyIndex)
	require.Equal(t, defaultRetentionSeconds, wire.RetentionSeconds)
	require.Len(t, wire.Balances, 1)
	require.Equal(t, 8, wire.Balances[0].KeyIndex)
	require.Equal(t, 9, wire.Balances[0].DeleteKeyIndex)
	require.Equal(t, 10, wire.Balances[0].LegacyDeleteKeyIndex)
	require.Equal(t, "9223372036854775807", wire.Balances[0].Snapshot.Version)
	require.Equal(t, "12345678901234567890.1234567890123456789", wire.Balances[0].Snapshot.Available)
	require.Equal(t, "0.0000000000000000001", wire.Transactions[0].Postings[0].Amount)
	require.Equal(t, "0", wire.Transactions[0].Postings[0].OverdraftAmount)
	require.Equal(t, string(input.CompletionPlans[0].Payload), wire.Transactions[0].CompletionPlan)
	require.Equal(t, "direct", wire.Transactions[0].Action)
	require.Nil(t, wire.Transactions[0].ParentTransactionID)
	require.Equal(t, input.Execution.Transactions[0].ID.String()+":"+input.Execution.ExecutionID.String(), wire.Transactions[0].RecoveryField)
	require.Equal(t, input.Execution.Transactions[0].ID.String(), wire.Transactions[0].GuardField)
	require.Equal(t, input.Execution.ExecutionID.String(), wire.ReceiptField)
	for _, key := range first.Keys {
		require.NotContains(t, string(first.Payload), key, "physical keys belong exclusively in KEYS")
	}
	input.CompletionPlans[0].Payload[0] = '['
	input.Execution.Transactions[0].Postings[0].Ref = "changed"
	require.Equal(t, first.Payload, second.Payload, "prepared bytes must not alias input storage")
}

func TestPrepareExecutionCarriesPerItemScopeInProtocolV3(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	organizationID := uuid.MustParse("a998807f-5e85-4469-8c9d-e40880113bb0")
	ledgerID := uuid.MustParse("f334b1ce-f18f-4163-b326-713cf7011fe2")
	input.Execution.Balances[0].OrganizationID = organizationID
	input.Execution.Balances[0].LedgerID = ledgerID
	input.Execution.Transactions[0].OrganizationID = organizationID
	input.Execution.Transactions[0].LedgerID = ledgerID

	ctx := core.ContextWithTenantID(context.Background(), "fixture")
	resolved, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)
	prepared, err := prepareExecution(ctx, input, limits, resolved)
	require.NoError(t, err)

	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.Equal(t, 3, wire.ProtocolVersion)
	require.Equal(t, organizationID.String(), wire.Balances[0].OrganizationID)
	require.Equal(t, ledgerID.String(), wire.Balances[0].LedgerID)
	require.Equal(t, organizationID.String(), wire.Transactions[0].OrganizationID)
	require.Equal(t, ledgerID.String(), wire.Transactions[0].LedgerID)
}

func TestResolveAdapterKeysUsesBalanceAndTransactionScope(t *testing.T) {
	t.Parallel()

	input, _, _ := validWireExecutionWithGrant()
	organizationID := uuid.MustParse("a998807f-5e85-4469-8c9d-e40880113bb0")
	ledgerID := uuid.MustParse("f334b1ce-f18f-4163-b326-713cf7011fe2")
	input.Execution.Balances[0].OrganizationID = organizationID
	input.Execution.Balances[0].LedgerID = ledgerID
	input.Execution.Transactions[0].OrganizationID = organizationID
	input.Execution.Transactions[0].LedgerID = ledgerID

	ctx := core.ContextWithTenantID(context.Background(), "fixture")
	resolved, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)
	key := scopedBalanceRef(organizationID, ledgerID, "@source#default")
	require.Contains(t, resolved.Balances[key].Balance, organizationID.String()+":"+ledgerID.String())
	require.Contains(t, resolved.AccountBlockExceptions[input.Execution.Transactions[0].AccountBlockException.ExceptionID], organizationID.String()+":"+ledgerID.String())
}

func TestEngineWriteBehindAtomicTransactionBatchBudgetCountsDependenciesAndIndexWire(t *testing.T) {
	input, limits, resolved := validWireExecution()
	transactionID := input.Execution.Transactions[0].ID
	dependency := command.TransactionEvidenceReference{
		Kind: command.TransactionDependencyPredecessor, TenantID: resolved.TenantID,
		OrganizationID: input.Execution.OrganizationID, LedgerID: input.Execution.LedgerID,
		TransactionID: transactionID, ExecutionID: uuid.New(),
	}

	without, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	input.CompletionPlans[0].Dependencies = []command.TransactionEvidenceReference{dependency}
	dependencyPayload, err := json.Marshal(input.CompletionPlans[0].Dependencies)
	require.NoError(t, err)
	limits.MaxCompletionPlanBytes = len(input.CompletionPlans[0].Payload) + len(dependencyPayload)
	with, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	require.Greater(t, len(with.Payload), len(without.Payload))
	require.Contains(t, string(with.Payload), `"kind":"predecessor"`)
	require.Contains(t, string(with.Payload), `"transactionIndexKeyIndex":6`)
	require.Contains(t, string(with.Payload), `"evidenceKeyIndex":7`)

	limits.MaxCompletionPlanBytes--
	_, err = prepareExecution(context.Background(), input, limits, resolved)
	require.ErrorContains(t, err, "completion plan exceeds byte limit")
}

func TestPrepareExecutionPreservesTransactionAndSnapshotOrder(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	secondID := uuid.MustParse("1935edb9-c953-4f87-bea4-c98f57dff8b4")
	second := input.Execution.Transactions[0]
	second.ID = secondID
	input.Execution.Transactions = append(input.Execution.Transactions, second)
	input.Guards = append([]command.ExecutionGuard{{TransactionID: secondID, NextToken: "next-two"}}, input.Guards...)
	input.CompletionPlans = append([]command.CompletionPlanRecord{{TransactionID: secondID, Payload: json.RawMessage(`{"value":"two","action":"direct"}`)}}, input.CompletionPlans...)
	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.Equal(t, input.Execution.Transactions[0].ID.String(), wire.Transactions[0].ID)
	require.Equal(t, secondID.String(), wire.Transactions[1].ID)
	require.Equal(t, "next-two", wire.Transactions[1].NextGuard)
	require.Equal(t, `{"value":"two","action":"direct"}`, wire.Transactions[1].CompletionPlan)
	require.Len(t, wire.Balances, 1)
}

func TestPrepareExecutionCarriesOpaqueCompletionPlanWireFields(t *testing.T) {
	input, limits, resolved := validWireExecution()
	limits.MaxCompletionPlanBytes = 256 * 1024
	limits.MaxRequestBytes = 512 * 1024
	parentTransactionID := uuid.New()
	input.CompletionPlans[0].Payload = json.RawMessage(fmt.Sprintf(`{"action":"revert","parentTransactionId":"%s","padding":"%s"}`,
		parentTransactionID, strings.Repeat("x", 128*1024)))

	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.Equal(t, "revert", wire.Transactions[0].Action)
	require.NotNil(t, wire.Transactions[0].ParentTransactionID)
	require.Equal(t, parentTransactionID.String(), *wire.Transactions[0].ParentTransactionID)
	require.Equal(t, string(input.CompletionPlans[0].Payload), wire.Transactions[0].CompletionPlan)
	require.NotContains(t, engineRequestScript, "decodeJSON(transaction.completionPlan)")
}

func TestPrepareExecutionEnforcesTrustedCountBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("exact limits", func(t *testing.T) {
		input, limits, resolved := validWireExecution()
		limits.MaxTransactions, limits.MaxPostings, limits.MaxBalances = 1, 1, 1

		prepared, err := prepareExecution(context.Background(), input, limits, resolved)
		require.NoError(t, err)
		require.NotNil(t, prepared)
	})

	t.Run("transaction excess", func(t *testing.T) {
		input, limits, resolved := validWireExecution()
		expandWireTransactions(&input, 2)
		limits.MaxTransactions = 1

		prepared, err := prepareExecution(context.Background(), input, limits, resolved)
		require.ErrorContains(t, err, "transaction or balance limits")
		require.Nil(t, prepared)
	})

	t.Run("posting excess", func(t *testing.T) {
		input, limits, resolved := validWireExecution()
		second := input.Execution.Transactions[0].Postings[0]
		second.Ref = "debit-1"
		input.Execution.Transactions[0].Postings = append(input.Execution.Transactions[0].Postings, second)
		limits.MaxPostings = 1

		prepared, err := prepareExecution(context.Background(), input, limits, resolved)
		require.ErrorContains(t, err, "posting limit")
		require.Nil(t, prepared)
	})

	t.Run("balance excess", func(t *testing.T) {
		input, limits, resolved := validWireExecution()
		addWireBalance(&input, &resolved)
		limits.MaxBalances = 1

		prepared, err := prepareExecution(context.Background(), input, limits, resolved)
		require.ErrorContains(t, err, "transaction or balance limits")
		require.Nil(t, prepared)
	})
}

func TestPrepareExecutionAcceptsProductionTransactionMaximum(t *testing.T) {
	t.Parallel()

	input, _, resolved := validWireExecution()
	expandWireTransactions(&input, maxTransactionsPerExecution)
	prepared, err := prepareExecution(context.Background(), input, hardLimits(), resolved)
	require.NoError(t, err)
	require.NotNil(t, prepared)

	expandWireTransactions(&input, maxTransactionsPerExecution+1)
	prepared, err = prepareExecution(context.Background(), input, hardLimits(), resolved)
	require.ErrorContains(t, err, "transaction or balance limits")
	require.Nil(t, prepared)
}

func TestPrepareExecutionRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*command.EngineExecution, *Limits, *resolvedExecutionKeys)
	}{
		{"zero scope", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.Execution.LedgerID = uuid.Nil }},
		{"zero execution", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.ExecutionID = uuid.Nil
		}},
		{"empty fingerprint", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.IntentFingerprint = " " }},
		{"zero limits", func(_ *command.EngineExecution, l *Limits, _ *resolvedExecutionKeys) { l.MaxBalances = 0 }},
		{"request bytes", func(_ *command.EngineExecution, l *Limits, _ *resolvedExecutionKeys) { l.MaxRequestBytes = 600 }},
		{"recovery bytes", func(_ *command.EngineExecution, l *Limits, _ *resolvedExecutionKeys) { l.MaxCompletionPlanBytes = 1 }},
		{"missing guard", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.Guards = nil }},
		{"unrelated guard", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Guards[0].TransactionID = x.Execution.ExecutionID
		}},
		{"unchanged guard", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Guards[0].ExpectedToken = x.Guards[0].NextToken
		}},
		{"empty next guard", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.Guards[0].NextToken = "" }},
		{"missing recovery", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) { x.CompletionPlans = nil }},
		{"unrelated recovery", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.CompletionPlans[0].TransactionID = x.Execution.ExecutionID
		}},
		{"invalid recovery JSON", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.CompletionPlans[0].Payload = json.RawMessage(`{"broken":}`)
		}},
		{"recovery array", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.CompletionPlans[0].Payload = json.RawMessage(`[]`)
		}},
		{"recovery null", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.CompletionPlans[0].Payload = json.RawMessage(`null`)
		}},
		{"zero amount", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings[0].Amount = decimal.Zero
		}},
		{"negative override", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings[0].OverdraftAmount = decimal.NewFromInt(-1)
		}},
		{"huge positive exponent", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings[0].Amount = decimal.New(1, math.MaxInt32)
		}},
		{"huge negative exponent", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings[0].Amount = decimal.New(1, math.MinInt32)
		}},
		{"unknown posting", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings[0].Type = "unknown"
		}},
		{"unknown policy", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings[0].DrawPolicy = "unknown"
		}},
		{"empty posting reference", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings[0].Ref = ""
		}},
		{"duplicate posting reference", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings = append(x.Execution.Transactions[0].Postings, x.Execution.Transactions[0].Postings[0])
		}},
		{"unknown balance", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings[0].BalanceRef = "@missing#default"
		}},
		{"unknown requirement balance", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].BalanceRequirements = []accounting.BalanceRequirement{{BalanceRef: "@missing#default", AssetCode: "USD", Permission: accounting.BalancePermissionSend}}
		}},
		{"empty requirement asset", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].BalanceRequirements = []accounting.BalanceRequirement{{BalanceRef: "@source#default", Permission: accounting.BalancePermissionSend}}
		}},
		{"unknown requirement permission", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].BalanceRequirements = []accounting.BalanceRequirement{{BalanceRef: "@source#default", AssetCode: "USD", Permission: "unknown"}}
		}},
		{"empty postings", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Transactions[0].Postings = nil
		}},
		{"posting count", func(x *command.EngineExecution, l *Limits, _ *resolvedExecutionKeys) {
			second := x.Execution.Transactions[0].Postings[0]
			second.Ref = "second"
			x.Execution.Transactions[0].Postings = append(x.Execution.Transactions[0].Postings, second)
			l.MaxPostings = 1
		}},
		{"unknown direction", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Balances[0].Direction = "unknown"
		}},
		{"negative version", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Balances[0].Version = -1
		}},
		{"negative onhold", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Balances[0].OnHold = decimal.NewFromInt(-1)
		}},
		{"negative debt", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Balances[0].OverdraftUsed = decimal.NewFromInt(-1)
		}},
		{"negative limit", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Balances[0].OverdraftLimit = decimal.NewFromInt(-1)
		}},
		{"negative nonexternal", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Balances[0].Available = decimal.NewFromInt(-1)
		}},
		{"unknown balance scope", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Balances[0].BalanceScope = "unknown"
		}},
		{"inconsistent alias", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Balances[0].Alias = "@different"
		}},
		{"zero balance identity", func(x *command.EngineExecution, _ *Limits, _ *resolvedExecutionKeys) {
			x.Execution.Balances[0].ID = uuid.Nil
		}},
		{"physical reference", func(x *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) {
			old := x.Execution.Balances[0].BalanceRef
			x.Execution.Balances[0].Alias = "balance:{transactions}:@source"
			x.Execution.Balances[0].BalanceRef = x.Execution.Balances[0].Alias + "#default"
			k.Balances[x.Execution.Balances[0].BalanceRef] = k.Balances[old]
			delete(k.Balances, old)
		}},
		{"missing resolved keys", func(_ *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) { k.Balances = nil }},
		{"missing protection key", func(_ *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) { k.Protection = "" }},
		{"wrong hash slot", func(_ *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) {
			k.Schedule = "schedule:{other}:sync"
		}},
		{"duplicate resolved key", func(_ *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) { k.Schedule = k.Recovery }},
		{"unrelated marker", func(_ *command.EngineExecution, _ *Limits, k *resolvedExecutionKeys) {
			pair := k.Balances["@source#default"]
			pair.Deleted += ":other"
			k.Balances["@source#default"] = pair
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input, limits, resolved := validWireExecution()
			tt.mutate(&input, &limits, &resolved)
			prepared, err := prepareExecution(context.Background(), input, limits, resolved)
			require.Error(t, err)
			require.Nil(t, prepared)
			var failure *accounting.Failure
			require.NotErrorAs(t, err, &failure, "invalid input is not a financial refusal")
		})
	}
}

func TestPrepareExecutionCarriesBalanceRequirements(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	input.Execution.Transactions[0].BalanceRequirements = []accounting.BalanceRequirement{{
		BalanceRef: "@source#default", AssetCode: "USD", Permission: accounting.BalancePermissionSend, ForbidExternal: true,
	}}

	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)

	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.Equal(t, []wireBalanceRequirement{{
		BalanceRef: "@source#default", AssetCode: "USD", Permission: accounting.BalancePermissionSend, ForbidExternal: true,
	}}, wire.Transactions[0].BalanceRequirements)
}

func TestPrepareExecutionCarriesBlockedAccountControl(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	input.Execution.Transactions[0].RejectBlockedBalances = true
	input.Execution.Balances[0].Blocked = true

	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)

	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.True(t, wire.Transactions[0].RejectBlockedBalances)
	require.True(t, wire.Balances[0].Snapshot.Blocked)
}

func TestPrepareExecutionCarriesAccountBlockExceptionAfterBalanceKeys(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecutionWithGrant()
	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)

	exception := input.Execution.Transactions[0].AccountBlockException
	require.NotNil(t, exception)
	accountID := input.Execution.Balances[0].AccountID
	require.Equal(t, []string{
		resolved.Schedule, resolved.Recovery, resolved.Receipts, resolved.Guards, resolved.Protection, resolved.TransactionIndex, resolved.Evidence,
		resolved.Balances["@source#default"].Balance,
		resolved.Balances["@source#default"].Deleted,
		resolved.Balances["@source#default"].LegacyDeleted,
		resolved.AccountBlockExceptions[exception.ExceptionID],
		resolved.Accounts[accountID].Closing, resolved.Accounts[accountID].Closed, resolved.Accounts[accountID].Ownership,
	}, prepared.Keys)

	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.Len(t, wire.Transactions, 1)
	require.Equal(t, &wireAccountBlockException{
		KeyIndex: 11, ExceptionID: exception.ExceptionID.String(), Alias: "@source",
		Amount: "0.0000000000000000001", PrimaryPostingRef: "debit-0",
	}, wire.Transactions[0].AccountBlockException)
	require.NotContains(t, string(prepared.Payload), resolved.AccountBlockExceptions[exception.ExceptionID])
}

func TestPrepareExecutionRejectsInvalidAccountBlockExceptionInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*command.EngineExecution, *resolvedExecutionKeys)
	}{
		{"nil id", func(input *command.EngineExecution, _ *resolvedExecutionKeys) {
			input.Execution.Transactions[0].AccountBlockException.ExceptionID = uuid.Nil
		}},
		{"empty alias", func(input *command.EngineExecution, _ *resolvedExecutionKeys) {
			input.Execution.Transactions[0].AccountBlockException.Alias = ""
		}},
		{"zero amount", func(input *command.EngineExecution, _ *resolvedExecutionKeys) {
			input.Execution.Transactions[0].AccountBlockException.Amount = decimal.Zero
		}},
		{"unknown posting", func(input *command.EngineExecution, _ *resolvedExecutionKeys) {
			input.Execution.Transactions[0].AccountBlockException.PrimaryPostingRef = "unknown"
		}},
		{"overdraft primary", func(input *command.EngineExecution, resolved *resolvedExecutionKeys) {
			balance := &input.Execution.Balances[0]
			delete(resolved.Balances, balance.BalanceRef)
			balance.Key, balance.BalanceRef = "overdraft", "@source#overdraft"
			input.Execution.Transactions[0].Postings[0].BalanceRef = balance.BalanceRef
			resolved.Balances[balance.BalanceRef] = testResolvedBalanceKeys("tenant:fixture:balance:{transactions}:@source#overdraft")
		}},
		{"alias mismatch", func(input *command.EngineExecution, _ *resolvedExecutionKeys) {
			input.Execution.Transactions[0].AccountBlockException.Alias = "@other"
		}},
		{"amount mismatch", func(input *command.EngineExecution, _ *resolvedExecutionKeys) {
			input.Execution.Transactions[0].AccountBlockException.Amount = decimal.NewFromInt(1)
		}},
		{"missing resolved key", func(input *command.EngineExecution, resolved *resolvedExecutionKeys) {
			delete(resolved.AccountBlockExceptions, input.Execution.Transactions[0].AccountBlockException.ExceptionID)
		}},
		{"balance key collision", func(input *command.EngineExecution, resolved *resolvedExecutionKeys) {
			id := input.Execution.Transactions[0].AccountBlockException.ExceptionID
			resolved.AccountBlockExceptions[id] = resolved.Balances["@source#default"].Balance
		}},
		{"grant key without hash tag", func(input *command.EngineExecution, resolved *resolvedExecutionKeys) {
			id := input.Execution.Transactions[0].AccountBlockException.ExceptionID
			resolved.AccountBlockExceptions[id] = "tenant:fixture:account_block_exception:" + id.String()
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input, limits, resolved := validWireExecutionWithGrant()
			test.mutate(&input, &resolved)
			prepared, err := prepareExecution(context.Background(), input, limits, resolved)
			require.Error(t, err)
			require.Nil(t, prepared)
		})
	}
}

func TestPrepareExecutionCanceledContext(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	prepared, err := prepareExecution(ctx, input, limits, resolved)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, prepared)
}

func TestBoundedDecimal(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"0", "-0", "1.2300", "1e3", "0.00001", "-15.5"} {
		t.Run(text, func(t *testing.T) {
			value := decimal.RequireFromString(text)
			got, err := boundedDecimal(value, 100)
			require.NoError(t, err)
			require.Equal(t, value.String(), got)
		})
	}
	got, err := boundedDecimal(decimal.New(0, math.MaxInt32), 1)
	require.NoError(t, err)
	require.Equal(t, "0", got)
}

func validWireExecution() (command.EngineExecution, Limits, resolvedExecutionKeys) {
	organizationID := uuid.MustParse("ef73c171-f889-4a9d-a5d3-421ee069d736")
	ledgerID := uuid.MustParse("fbe630e6-fde0-4396-bb3b-b5e5da509ae0")
	executionID := uuid.MustParse("f00b8ce5-fad0-45da-b4a8-0bcaec270b08")
	transactionID := uuid.MustParse("53d2c279-4d51-4274-8a3d-ee1955ddcaa0")
	input := command.EngineExecution{
		Execution: accounting.Execution{
			OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID,
			Transactions: []accounting.Transaction{{ID: transactionID, Postings: []accounting.Posting{{Ref: "debit-0", BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.RequireFromString("0.0000000000000000001"), DrawPolicy: accounting.DrawAllowed, OverdraftAmount: decimal.Zero}}}},
			Balances:     []accounting.BalanceSnapshot{{BalanceRef: "@source#default", ID: uuid.MustParse("d9e5a2be-9128-43ab-9d3c-0c14e3a8b889"), AccountID: uuid.MustParse("1f3da5d4-1571-4619-938f-1aef51560726"), AccountType: "deposit", AssetCode: "USD", Alias: "@source", Key: "default", Direction: "credit", BalanceScope: "transactional", Available: decimal.RequireFromString("12345678901234567890.1234567890123456789"), OnHold: decimal.Zero, OverdraftUsed: decimal.Zero, OverdraftLimit: decimal.NewFromInt(1000), Version: math.MaxInt64, AllowSending: true, AllowReceiving: true}},
		},
		IntentFingerprint: "immutable-intent",
		Guards:            []command.ExecutionGuard{{TransactionID: transactionID, NextToken: "pending-token"}},
		CompletionPlans:   []command.CompletionPlanRecord{{TransactionID: transactionID, Payload: json.RawMessage("{\n  \"version\": 9223372036854775807, \"amount\": \"123456789.123456789\", \"action\": \"direct\"\n}")}},
	}
	prefix := "tenant:fixture:"
	scope := organizationID.String() + ":" + ledgerID.String()
	balanceKey := prefix + "balance:{transactions}:" + scope + ":@source#default"
	resolved := resolvedExecutionKeys{TenantID: "fixture", Schedule: prefix + "schedule:{transactions}:balance-sync-v2", Recovery: prefix + cachepolicy.EngineRecoverQueue, Receipts: prefix + "engine:{transactions}:receipts:" + scope, Guards: prefix + "engine:{transactions}:guards:" + scope, Protection: prefix + "engine:{transactions}:protection:" + scope, TransactionIndex: prefix + "engine:{transactions}:transaction-index:" + scope, Evidence: prefix + "engine:{transactions}:evidence:" + scope, Balances: map[string]resolvedBalanceKeys{"@source#default": testResolvedBalanceKeys(balanceKey)}}
	resolved.Accounts = testResolvedAccountKeys(prefix, input.Execution, testAdmissionToken)
	return input, Limits{MaxTransactions: 10, MaxPostings: 100, MaxBalances: 100, MaxCompletionPlanBytes: 4096, MaxRequestBytes: 16384, MaxPreparedBytes: 1048576}, resolved
}

func validWireExecutionWithGrant() (command.EngineExecution, Limits, resolvedExecutionKeys) {
	input, limits, resolved := validWireExecution()
	exceptionID := uuid.MustParse("6e0ebc70-6039-4edf-b039-4bb5d85afafe")
	posting := input.Execution.Transactions[0].Postings[0]
	input.Execution.Transactions[0].AccountBlockException = &accounting.AccountBlockException{
		ExceptionID: exceptionID, Alias: "@source", Amount: posting.Amount, PrimaryPostingRef: posting.Ref,
	}
	resolved.AccountBlockExceptions = map[uuid.UUID]string{
		exceptionID: "tenant:fixture:account_block_exception:{transactions}:" + input.Execution.OrganizationID.String() + ":" + input.Execution.LedgerID.String() + ":" + exceptionID.String(),
	}

	return input, limits, resolved
}

func expandWireTransactions(input *command.EngineExecution, count int) {
	template := input.Execution.Transactions[0]
	templateGuard := input.Guards[0]
	templatePlan := input.CompletionPlans[0]
	input.Execution.Transactions = make([]accounting.Transaction, 0, count)
	input.Guards = make([]command.ExecutionGuard, 0, count)
	input.CompletionPlans = make([]command.CompletionPlanRecord, 0, count)

	for index := range count {
		transactionID := uuid.NewSHA1(template.ID, []byte{byte(index), byte(index >> 8)})
		transaction := template
		transaction.ID = transactionID
		transaction.Postings = append([]accounting.Posting(nil), template.Postings...)
		guard := templateGuard
		guard.TransactionID = transactionID
		guard.NextToken = transactionID.String()
		plan := templatePlan
		plan.TransactionID = transactionID
		plan.Payload = append(json.RawMessage(nil), templatePlan.Payload...)

		input.Execution.Transactions = append(input.Execution.Transactions, transaction)
		input.Guards = append(input.Guards, guard)
		input.CompletionPlans = append(input.CompletionPlans, plan)
	}
}

func addWireBalance(input *command.EngineExecution, resolved *resolvedExecutionKeys) {
	balance := input.Execution.Balances[0]
	balance.ID = uuid.MustParse("0d174365-c8e6-4557-aeb8-672e446fa9cc")
	balance.AccountID = uuid.MustParse("8e081845-44c7-451a-970f-97673183819c")
	balance.Alias = "@secondary"
	balance.BalanceRef = balance.Alias + "#" + balance.Key
	input.Execution.Balances = append(input.Execution.Balances, balance)
	resolved.Balances[balance.BalanceRef] = testResolvedBalanceKeys(
		"tenant:fixture:balance:{transactions}:" + input.Execution.OrganizationID.String() + ":" +
			input.Execution.LedgerID.String() + ":" + balance.BalanceRef,
	)
}

func TestWireArraysAreNotNull(t *testing.T) {
	t.Parallel()

	input, limits, resolved := validWireExecution()
	prepared, err := prepareExecution(context.Background(), input, limits, resolved)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(prepared.Payload), ":null"))
	var wire wireRequest
	require.NoError(t, json.Unmarshal(prepared.Payload, &wire))
	require.False(t, reflect.ValueOf(wire.Transactions).IsNil())
	require.False(t, reflect.ValueOf(wire.Balances).IsNil())
	require.False(t, reflect.ValueOf(wire.Transactions[0].BalanceRequirements).IsNil())
	require.False(t, reflect.ValueOf(wire.Transactions[0].Postings).IsNil())
}

// TestPreparedExecutionMeasurements keeps the engine bound below the largest
// currently accepted HTTP body. The wire is deterministic, so these are
// contract measurements rather than machine-dependent timing observations.
// The pool is intentionally measured separately from touched postings: v2
// permits a scoped pool larger than the legs, and the pool must not be treated
// as user-requested cardinality.
func TestPreparedExecutionMeasurements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		postings   int
		pool       int
		wantBytes  int
		maxTouched int
	}{
		{name: "two postings", postings: 2, pool: 2, wantBytes: 3247, maxTouched: 2},
		{name: "ten postings", postings: 10, pool: 20, wantBytes: 21567, maxTouched: 10},
		{name: "fifty postings", postings: 50, pool: 100, wantBytes: 104047, maxTouched: 50},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, limits, resolved := measuredWireExecution(test.postings, test.pool)
			limits.MaxPostings, limits.MaxBalances, limits.MaxRequestBytes = 500, 500, 4*1024*1024
			prepared, err := prepareExecution(context.Background(), input, limits, resolved)
			require.NoError(t, err)
			require.Equal(t, test.wantBytes, len(prepared.Payload))
			require.LessOrEqual(t, test.postings, test.maxTouched)
			require.Equal(t, test.pool, len(input.Execution.Balances))
			t.Logf("postings=%d full_pool=%d touched=%d request_bytes=%d", test.postings, test.pool, test.postings, len(prepared.Payload))
		})
	}
}

// TestV1NearBodyLimitExpansionLowerBound is boundary evidence for the legacy
// request contract. v1 has no leg cap; each leg may also carry flat metadata
// (100-byte keys and 2,000-byte values, subject to the global JSON key guard).
// The generated body is therefore a valid request just below Fiber's 4 MiB body
// limit, with one maximum-size value chosen to exercise JSON quote, slash, and
// newline escaping.
//
// The measurements intentionally do not activate limits. This fixture uses the
// fully decoded and validated transaction plus its validation response, but
// deliberately supplies only the minimum two engine postings/snapshots and one
// projection context. It therefore establishes a lower bound, not the fully
// translated worst case. The completion plan contains the original transaction
// and stable projection metadata, while the prepared wire contains that plan
// again. Route
// expansion and one projection context per leg can add bytes after this point,
// so a mathematically safe v1
// MaxCompletionPlanBytes/MaxRequestBytes/MaxPreparedBytes or cardinality cap cannot be
// chosen from this single request without changing the shipped v1 acceptance
// surface. This test is the guard against doing so accidentally.
func TestV1NearBodyLimitExpansionLowerBound(t *testing.T) {
	const bodyLimit = 4 * 1024 * 1024

	body, input := nearV1Body(t, bodyLimit-1024)
	require.GreaterOrEqual(t, len(body), bodyLimit-16*1024)
	require.Less(t, len(body), bodyLimit)

	decoded := new(ledgerin.CreateTransactionRequest)
	_, err := pkgHTTP.DecodeAndValidate(body, decoded)
	require.NoError(t, err)
	transaction := *decoded.BuildTransaction()
	validate, err := mtransaction.ValidateSendSourceAndDistribute(context.Background(), transaction, constant.CREATED)
	require.NoError(t, err)
	require.Greater(t, len(transaction.Send.Source.From), 1000)
	require.Equal(t, len(transaction.Send.Source.From), len(transaction.Send.Distribute.To))

	orgID := uuid.MustParse("ef73c171-f889-4a9d-a5d3-421ee069d736")
	ledgerID := uuid.MustParse("fbe630e6-fde0-4396-bb3b-b5e5da509ae0")
	txID := uuid.MustParse("53d2c279-4d51-4274-8a3d-ee1955ddcaa0")
	executionID := uuid.MustParse("f00b8ce5-fad0-45da-b4a8-0bcaec270b08")
	sourceID := uuid.MustParse("d9e5a2be-9128-43ab-9d3c-0c14e3a8b889")
	destinationID := uuid.MustParse("1f3da5d4-1571-4619-938f-1aef51560726")

	sourceRef := "@source#default"
	destinationRef := "@destination#default"
	request := accounting.Execution{
		OrganizationID: orgID, LedgerID: ledgerID, ExecutionID: executionID,
		Transactions: []accounting.Transaction{{ID: txID, Postings: []accounting.Posting{
			{Ref: "from:0:debit", BalanceRef: sourceRef, Type: accounting.PostingDebit, Amount: decimal.NewFromInt(1), DrawPolicy: accounting.DrawForbidden},
			{Ref: "to:0:credit", BalanceRef: destinationRef, Type: accounting.PostingCredit, Amount: decimal.NewFromInt(1), DrawPolicy: accounting.DrawForbidden},
		}}},
		Balances: []accounting.BalanceSnapshot{
			{BalanceRef: sourceRef, ID: sourceID, AccountID: sourceID, AccountType: "deposit", AssetCode: "USD", Alias: "@source", Key: "default", Direction: "credit", BalanceScope: "transactional", Available: decimal.NewFromInt(2), AllowSending: true, AllowReceiving: true},
			{BalanceRef: destinationRef, ID: destinationID, AccountID: destinationID, AccountType: "deposit", AssetCode: "USD", Alias: "@destination", Key: "default", Direction: "credit", BalanceScope: "transactional", Available: decimal.Zero, AllowSending: true, AllowReceiving: true},
		},
	}
	projectionBalance := command.OperationBalanceContext{OrganizationID: orgID.String(), LedgerID: ledgerID.String(), ID: sourceID.String(), AccountID: sourceID.String(), Alias: "@source", Key: "default", AssetCode: "USD", AccountType: "deposit"}
	payload := command.TransactionCompletionPlan{
		FormatVersion: command.TransactionCompletionFormatVersion, TenantID: "fixture", HeaderID: "header", TransactionID: txID,
		OrganizationID: orgID, LedgerID: ledgerID, ExecutionID: executionID, IntentFingerprint: strings.Repeat("a", 64), TransactionInput: transaction, Validate: validate, TTL: fixedSizingTime(),
		TransactionStatus: constant.CREATED, Action: constant.ActionCommit, TransactionDate: fixedSizingTime(), TransactionCreatedAt: fixedSizingTime(), TransactionUpdatedAt: fixedSizingTime(), OperationUpdatedAt: fixedSizingTime(),
		OperationSpecs: []command.OperationRecordSpec{{TransactionID: txID, PostingRef: "from:0:debit", BalanceRef: sourceRef, Role: "primary", Side: command.OperationSpecSideFrom, RowType: "DEBIT", Direction: "credit", Description: "description", ChartOfAccounts: "1000", Metadata: input.Send.Source.From[0].Metadata, Balance: projectionBalance, RequestedAmount: decimal.NewFromInt(1), CompatibilityPath: command.OperationRecordStandard}},
	}
	recovery, err := command.EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)

	limits := Limits{MaxTransactions: 1, MaxPostings: 2, MaxBalances: 2, MaxCompletionPlanBytes: len(recovery) + len(`[]`) + 1, MaxRequestBytes: bodyLimit * 4, MaxPreparedBytes: bodyLimit * 4}
	inputExecution := command.EngineExecution{Execution: request, IntentFingerprint: "immutable-intent", Guards: []command.ExecutionGuard{{TransactionID: txID, ExpectedToken: "old", NextToken: "next"}}, CompletionPlans: []command.CompletionPlanRecord{{TransactionID: txID, Payload: recovery}}}
	resolved := sizingResolvedKeys(request)
	prepared, err := prepareExecution(context.Background(), inputExecution, limits, resolved)
	require.NoError(t, err)

	t.Logf("v1 lower-bound bytes: original=%d frozen_recovery=%d v1_legs=%d wire_postings=%d snapshots=%d final_wire=%d", len(body), len(recovery), len(transaction.Send.Source.From)+len(transaction.Send.Distribute.To), len(request.Transactions[0].Postings), len(request.Balances), len(prepared.Payload))
	require.Equal(t, 4193188, len(body))
	require.Equal(t, 11175426, len(recovery))
	require.Equal(t, 13064885, len(prepared.Payload))
	require.Greater(t, len(recovery), len(body), "completion plan must retain transaction and stable projection data")
	require.Greater(t, len(prepared.Payload), len(recovery), "wire must carry the completion plan plus engine postings and snapshots")
	require.Equal(t, 2, len(request.Transactions[0].Postings), "v1 retains both logical legs; no v1 leg cap is introduced")
}

// TestV2TransactionBodyDoesNotBoundFeeExpandedEngineBytes exercises the complete
// v2 decode/translation, fee calculation, second validation, snapshot-pool,
// engine translation, recovery, and wire preparation path. Both cases use the
// same small accepted v2 body. Only the valid fee package cardinality changes;
// its fees map has a minimum but no application-level maximum.
//
// This is not a claim that storage has infinite capacity. It proves the narrower
// safety fact: transaction-body limits and application validators do not bound
// the engine's expanded byte and cardinality workload.
func TestV2TransactionBodyDoesNotBoundFeeExpandedEngineBytes(t *testing.T) {
	t.Parallel()

	small := prepareTransactionBodyWithFees(t, 1)
	large := prepareTransactionBodyWithFees(t, 8)

	require.Equal(t, small.bodyBytes, large.bodyBytes)
	require.Equal(t, 508, large.postings-small.postings)
	require.Equal(t, 7, large.snapshots-small.snapshots)
	require.Equal(t, 508, large.projections-small.projections)
	require.Greater(t, large.recoveryBytes, small.recoveryBytes)
	require.Greater(t, large.wireBytes, small.wireBytes)
	t.Logf("same transaction body=%d bytes: fees=1/8 postings=%d/%d projections=%d/%d snapshots=%d/%d recovery=%d/%d wire=%d/%d",
		small.bodyBytes, small.postings, large.postings, small.projections, large.projections,
		small.snapshots, large.snapshots, small.recoveryBytes, large.recoveryBytes, small.wireBytes, large.wireBytes)
}

type preparedSize struct {
	bodyBytes     int
	recoveryBytes int
	wireBytes     int
	postings      int
	snapshots     int
	projections   int
}

func prepareTransactionBodyWithFees(t *testing.T, feeCount int) preparedSize {
	t.Helper()

	organizationID := uuid.MustParse("ef73c171-f889-4a9d-a5d3-421ee069d736")
	ledgerID := uuid.MustParse("fbe630e6-fde0-4396-bb3b-b5e5da509ae0")
	input := ledgerin.CreateTransactionV2Request{
		Asset: "USD", Amount: "1",
		Debits:  []ledgerin.TransactionV2LegRequest{{Alias: "@source", OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), Amount: "1"}},
		Credits: []ledgerin.TransactionV2LegRequest{{Alias: "@destination", OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), Amount: "1"}},
	}
	body, err := json.Marshal(input)
	require.NoError(t, err)

	decoded := new(ledgerin.CreateTransactionV2Request)
	_, err = pkgHTTP.DecodeAndValidate(body, decoded)
	require.NoError(t, err)
	transaction, scope, err := decoded.Translate(false)
	require.NoError(t, err)
	require.Equal(t, organizationID.String(), scope.OrganizationID)
	require.Equal(t, ledgerID.String(), scope.LedgerID)
	validate, err := mtransaction.ValidateSendSourceAndDistribute(t.Context(), transaction, constant.CREATED)
	require.NoError(t, err)

	fees := make(map[string]feemodel.Fee, feeCount)
	notDeductible := false
	for i := range feeCount {
		key := fmt.Sprintf("fee-%06d", i)
		fees[key] = feemodel.Fee{
			FeeLabel: key,
			CalculationModel: &feemodel.CalculationModel{
				ApplicationRule: feeconstant.AppRuleFlatFee,
				Calculations:    []feemodel.Calculation{{Type: feeconstant.FeeTypeFlat, Value: "1"}},
			},
			ReferenceAmount:  feeconstant.ReferenceAmountOriginalAmount,
			Priority:         i + 1,
			IsDeductibleFrom: &notDeductible,
			CreditAccount:    fmt.Sprintf("@fee-%06d", i),
		}
		candidate := fees[key]
		require.NoError(t, candidate.ValidateNewFee(key, decimal.Zero))
	}
	enabled := true
	packageInput := feemodel.CreatePackageInput{
		FeeGroupLabel: "engine sizing", MinAmount: "0", MaxAmount: "1000000",
		Fee: fees, Enable: &enabled,
	}
	packageBody, err := json.Marshal(packageInput)
	require.NoError(t, err)
	validatedPackage := new(feemodel.CreatePackageInput)
	_, err = pkgHTTP.DecodeAndValidate(packageBody, validatedPackage)
	require.NoError(t, err)
	require.NoError(t, validatedPackage.ValidateFees())

	logger, err := libZap.New(libZap.Config{Environment: libZap.EnvironmentLocal, OTelLibraryName: "engine-wire-sizing"})
	require.NoError(t, err)
	calculation := &feemodel.FeeCalculate{Transaction: transaction}
	feePackage := &pack.Package{Fees: fees, WaivedAccounts: &[]string{}}
	require.NoError(t, ledgerfee.CalculateFee(logger, calculation, feePackage, validate, nil))
	transaction = calculation.Transaction

	mtransaction.ApplyDefaultBalanceKeys(transaction.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(transaction.Send.Distribute.To)
	mtransaction.MutateConcatAliases(transaction.Send.Source.From)
	mtransaction.MutateConcatAliases(transaction.Send.Distribute.To)
	validate, err = mtransaction.ValidateSendSourceAndDistribute(t.Context(), transaction, constant.CREATED)
	require.NoError(t, err)

	transactionID := uuid.MustParse("53d2c279-4d51-4274-8a3d-ee1955ddcaa0")
	executionID := uuid.MustParse("f00b8ce5-fad0-45da-b4a8-0bcaec270b08")
	balances := make([]*mmodel.Balance, 0, len(validate.Aliases))
	for _, ref := range validate.Aliases {
		alias, key, found := strings.Cut(ref, mtransaction.AliasSeparatorString)
		require.True(t, found)
		available := int64(0)
		if alias == "@source" {
			available = int64(feeCount + 1)
		}
		balances = append(balances, sizingBalance(organizationID, ledgerID, alias, key, available))
	}
	pool, err := command.BuildEngineSnapshotPool(t.Context(), organizationID, ledgerID, validate.Aliases, balances, balances)
	require.NoError(t, err)

	translated, projection, err := command.TranslateEngineTransaction(command.EngineTranslationInput{
		TransactionID: transactionID, Action: constant.ActionDirect, TransactionStatus: constant.CREATED,
		TransactionInput: transaction, Validate: validate, Balances: pool.Balances,
	})
	require.NoError(t, err)
	require.Len(t, translated.Postings, 1<<(feeCount+1))
	require.Len(t, projection, 1<<(feeCount+1))

	payload := command.TransactionCompletionPlan{
		FormatVersion: command.TransactionCompletionFormatVersion, TenantID: "fixture", HeaderID: "header", TransactionID: transactionID,
		OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID, IntentFingerprint: strings.Repeat("a", 64),
		TransactionInput: transaction, Validate: validate, TTL: fixedSizingTime(), TransactionStatus: constant.CREATED,
		Action: constant.ActionDirect, TransactionDate: fixedSizingTime(), TransactionCreatedAt: fixedSizingTime(),
		TransactionUpdatedAt: fixedSizingTime(), OperationUpdatedAt: fixedSizingTime(), OperationSpecs: projection,
	}
	recovery, err := command.EncodeTransactionCompletionPlan(payload)
	require.NoError(t, err)

	request := accounting.Execution{
		OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID,
		Transactions: []accounting.Transaction{translated}, Balances: pool.Snapshots,
	}
	limits := Limits{
		MaxTransactions: 1, MaxPostings: len(translated.Postings), MaxBalances: len(pool.Snapshots),
		MaxCompletionPlanBytes: len(recovery) + len(`[]`) + 1, MaxRequestBytes: 16 * 1024 * 1024, MaxPreparedBytes: 16 * 1024 * 1024,
	}
	execution := command.EngineExecution{
		Execution: request, IntentFingerprint: "immutable-intent",
		Guards:          []command.ExecutionGuard{{TransactionID: transactionID, ExpectedToken: "old", NextToken: "next"}},
		CompletionPlans: []command.CompletionPlanRecord{{TransactionID: transactionID, Payload: recovery}},
	}
	prepared, err := prepareExecution(t.Context(), execution, limits, sizingResolvedKeys(request))
	require.NoError(t, err)

	return preparedSize{
		bodyBytes: len(body), recoveryBytes: len(recovery), wireBytes: len(prepared.Payload),
		postings: len(translated.Postings), snapshots: len(pool.Snapshots), projections: len(projection),
	}
}

func sizingBalance(organizationID, ledgerID uuid.UUID, alias, key string, available int64) *mmodel.Balance {
	identity := uuid.NewSHA1(uuid.NameSpaceOID, []byte(alias+"#"+key))

	return &mmodel.Balance{
		ID: identity.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), AccountID: identity.String(),
		Alias: alias, Key: key, AssetCode: "USD", AccountType: "deposit",
		Available: decimal.NewFromInt(available), Direction: constant.DirectionCredit,
		AllowSending: true, AllowReceiving: true, CreatedAt: fixedSizingTime(), UpdatedAt: fixedSizingTime(),
	}
}

func nearV1Body(t *testing.T, target int) ([]byte, ledgerin.CreateTransactionRequest) {
	t.Helper()
	value := strings.Repeat("\\\"\n", 666) + "\\\""
	makeInput := func(count int) ledgerin.CreateTransactionRequest {
		from := make([]mtransaction.FromTo, count)
		to := make([]mtransaction.FromTo, count)
		for i := 0; i < count; i++ {
			amount := &mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(1)}
			from[i] = mtransaction.FromTo{AccountAlias: "@source-" + fmt.Sprintf("%05d", i), Amount: amount}
			to[i] = mtransaction.FromTo{AccountAlias: "@destination-" + fmt.Sprintf("%05d", i), Amount: &mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(1)}}
		}
		from[0].Metadata = map[string]any{"escaped": value}
		return ledgerin.CreateTransactionRequest{Send: mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(int64(count)), Source: mtransaction.Source{From: from}, Distribute: mtransaction.Distribute{To: to}}}
	}
	low, high := 0, 100_000
	for low+1 < high {
		mid := (low + high) / 2
		body, err := json.Marshal(makeInput(mid))
		require.NoError(t, err)
		if len(body) < target {
			low = mid
		} else {
			high = mid
		}
	}
	input := makeInput(low)
	body, err := json.Marshal(input)
	require.NoError(t, err)
	return body, input
}

func fixedSizingTime() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func sizingResolvedKeys(request accounting.Execution) resolvedExecutionKeys {
	balances := request.Balances
	resolved := resolvedExecutionKeys{TenantID: "fixture", Schedule: "tenant:fixture:schedule:{transactions}", Recovery: "tenant:fixture:recovery:{transactions}", Receipts: "tenant:fixture:receipts:{transactions}", Guards: "tenant:fixture:guards:{transactions}", Protection: "tenant:fixture:protection:{transactions}", TransactionIndex: "tenant:fixture:transaction-index:{transactions}", Evidence: "tenant:fixture:evidence:{transactions}", Balances: make(map[string]resolvedBalanceKeys, len(balances))}
	for _, balance := range balances {
		key := "tenant:fixture:balance:{transactions}:" + balance.BalanceRef
		resolved.Balances[balance.BalanceRef] = testResolvedBalanceKeys(key)
	}
	sizingResolvedAccountKeys(request, &resolved)
	return resolved
}

// sizingResolvedAccountKeys completes a sizing fixture whose pool was rebuilt
// after construction, so the account protection block matches the new accounts.
func sizingResolvedAccountKeys(request accounting.Execution, resolved *resolvedExecutionKeys) {
	resolved.Accounts = testResolvedAccountKeys("tenant:fixture:", request, testAdmissionToken)
}

func measuredWireExecution(postingCount, poolCount int) (command.EngineExecution, Limits, resolvedExecutionKeys) {
	input, limits, resolved := validWireExecution()
	baseBalance := input.Execution.Balances[0]
	input.Execution.Balances = make([]accounting.BalanceSnapshot, poolCount)
	resolved.Balances = make(map[string]resolvedBalanceKeys, poolCount)
	for i := 0; i < poolCount; i++ {
		index := strconv.Itoa(i)
		balance := baseBalance
		balance.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("measurement-balance:"+index))
		balance.AccountID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("measurement-account:"+index))
		balance.Alias = "@source-" + index
		balance.BalanceRef = balance.Alias + "#default"
		input.Execution.Balances[i] = balance
		key := "tenant:fixture:balance:{transactions}:measurement:" + index
		resolved.Balances[balance.BalanceRef] = testResolvedBalanceKeys(key)
	}
	sizingResolvedAccountKeys(input.Execution, &resolved)
	input.Execution.Transactions[0].Postings = make([]accounting.Posting, postingCount)
	for i := 0; i < postingCount; i++ {
		input.Execution.Transactions[0].Postings[i] = accounting.Posting{Ref: "debit-" + strconv.Itoa(i), BalanceRef: input.Execution.Balances[i].BalanceRef, Type: accounting.PostingDebit, Amount: decimal.NewFromInt(1), DrawPolicy: accounting.DrawAllowed}
	}
	return input, limits, resolved
}

// testAdmissionToken is the administrative token the fixtures hold over every
// account of their pool. It stands for the ownership a cache-miss load takes and
// keeps alive until the execution answers.
const testAdmissionToken = "fixture-admission-token"

// testResolvedAccountKeys mirrors the adapter's account protection resolution for
// one fixture pool: the closing and closed markers and the administrative
// ownership of every account, each under the fixture's tenant prefix.
func testResolvedAccountKeys(prefix string, request accounting.Execution, token string) map[uuid.UUID]resolvedAccountKeys {
	scope := request.OrganizationID.String() + ":" + request.LedgerID.String() + ":"
	accounts := make(map[uuid.UUID]resolvedAccountKeys, len(request.Balances))

	for _, accountID := range protectedAccounts(request) {
		accounts[accountID] = resolvedAccountKeys{
			Closing:        prefix + "account-closing:" + cachepolicy.HashTag + ":" + scope + accountID.String(),
			Closed:         prefix + "account-closed:" + cachepolicy.HashTag + ":" + scope + accountID.String(),
			Ownership:      prefix + "account-admin-ownership:" + cachepolicy.HashTag + ":" + scope + accountID.String(),
			AdmissionToken: token,
		}
	}

	return accounts
}

func testResolvedBalanceKeys(key string) resolvedBalanceKeys {
	deleted, ok := cachepolicy.DeletionMarkerKey(key)
	if !ok {
		panic("test balance key does not use the balance namespace")
	}

	return resolvedBalanceKeys{
		Balance: key, Deleted: deleted, LegacyDeleted: key + cachepolicy.DeletionMarkerSuffix,
	}
}
