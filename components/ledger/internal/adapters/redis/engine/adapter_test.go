// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

type accountingReply string

func (r accountingReply) Error() string { return string(r) }
func (r accountingReply) RedisError()   {}

type countingProvider struct{ calls int }

func (p *countingProvider) GetClient(context.Context) (redis.UniversalClient, error) {
	p.calls++
	return nil, errors.New("unexpected provider access")
}

func TestTechnicalError_NeutralClassificationPreservesCause(t *testing.T) {
	cause := errors.New("redis unavailable")

	confirmed := &TechnicalError{Code: "execution_guard_conflict", Err: cause}
	require.Equal(t, "execution_guard_conflict", confirmed.EngineFailureCode())
	require.False(t, confirmed.OutcomeIndeterminate())
	require.True(t, errors.Is(confirmed, cause))

	uncertain := &TechnicalError{Code: "execution_outcome_unknown", Indeterminate: true, Err: cause}
	require.Equal(t, "execution_outcome_unknown", uncertain.EngineFailureCode())
	require.True(t, uncertain.OutcomeIndeterminate())
	var classified *TechnicalError
	require.True(t, errors.As(uncertain, &classified))
	require.Same(t, uncertain, classified)

	var nilError *TechnicalError
	require.Empty(t, nilError.EngineFailureCode())
	require.False(t, nilError.OutcomeIndeterminate())
}

func adapterResultFixture(t *testing.T) (core.Request, resultEnvelope) {
	t.Helper()
	var request core.Request
	require.NoError(t, json.Unmarshal([]byte(`{"transactions":[{"id":"11111111-1111-4111-8111-111111111111","postings":[{"ref":"postação","balanceRef":"@source#default","type":"DEBIT","amount":"30"}]}]}`), &request))
	require.Len(t, request.Transactions, 1)
	require.Len(t, request.Transactions[0].Postings, 1)
	request.Transactions[0].Postings[0].Type = core.PostingDebit
	request.OrganizationID = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	request.LedgerID = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	request.Balances = []core.BalanceSnapshot{{
		ID: uuid.MustParse("44444444-4444-4444-8444-444444444444"), AccountID: uuid.MustParse("55555555-5555-4555-8555-555555555555"),
		BalanceRef: "@source#default", Alias: "@source", Key: "default", AccountType: "deposit", AssetCode: "BRL", Direction: "credit", BalanceScope: "transactional",
		Available: decimal.NewFromInt(100), OverdraftLimit: decimal.NewFromInt(1000), Version: 7, AllowSending: true, AllowReceiving: true,
	}}
	posting := request.Transactions[0].Postings[0]
	movement := resultMovement{
		Ref:           fmt.Sprintf("%s:%d:%s:%s:0", request.Transactions[0].ID, len(posting.Ref), posting.Ref, core.RolePrimary),
		TransactionID: request.Transactions[0].ID.String(), PostingRef: posting.Ref, Role: core.RolePrimary, BalanceRef: posting.BalanceRef,
		Type: core.PostingDebit, Amount: "30", OverdraftDelta: "0",
		Before: adapterJSON(t, resultState{"100", "0", "0", "7"}), After: adapterJSON(t, resultState{"70", "0", "0", "8"}),
	}
	final := request.Balances[0]
	final.Available, final.Version = decimal.NewFromInt(70), 8
	snapshot, err := prepareSnapshot(final, 1<<20)
	require.NoError(t, err)
	return request, resultEnvelope{1, []json.RawMessage{adapterJSON(t, movement)}, []json.RawMessage{adapterJSON(t, resultBalance{final.BalanceRef, snapshot})}}
}

func adapterJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func mutateAdapterObject(t *testing.T, raw json.RawMessage, field string, value any) json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &object))
	if value == nil {
		delete(object, field)
	} else {
		object[field] = adapterJSON(t, value)
	}
	return adapterJSON(t, object)
}

func TestDecodeResult_StrictEnvelopeAndIdentity(t *testing.T) {
	request, response := adapterResultFixture(t)
	result, err := DecodeResult(adapterJSON(t, response), request)
	require.NoError(t, err)
	require.Len(t, result.Movements, 1)
	require.True(t, result.Final[0].Available.Equal(decimal.NewFromInt(70)))
	require.Equal(t, int64(8), result.Final[0].Version)

	cases := []struct {
		name, section, field string
		value                any
	}{
		{"unknown protocol", "root", "protocolVersion", 2},
		{"unknown field", "root", "extension", true},
		{"missing movements", "root", "movements", nil},
		{"null movements", "root", "movements", json.RawMessage("null")},
		{"object movements", "root", "movements", map[string]any{}},
		{"missing final", "root", "final", nil},
		{"numeric amount", "movement", "amount", 30},
		{"scientific amount", "movement", "amount", "3E+1"},
		{"trailing zero amount", "movement", "amount", "30.0"},
		{"signed zero delta", "movement", "overdraftDelta", "-0"},
		{"missing role", "movement", "role", nil},
		{"null role", "movement", "role", json.RawMessage("null")},
		{"foreign transaction", "movement", "transactionId", request.OrganizationID.String()},
		{"invalid transaction", "movement", "transactionId", "invalid"},
		{"foreign posting", "movement", "postingRef", "foreign"},
		{"foreign balance", "movement", "balanceRef", "@other#default"},
		{"unknown role", "movement", "role", "future"},
		{"invalid reference", "movement", "ref", "arbitrary"},
		{"wrong type", "movement", "type", core.PostingCredit},
		{"unknown type", "movement", "type", "future"},
		{"excess amount", "movement", "amount", "31"},
		{"foreign final identity", "final", "id", request.OrganizationID.String()},
		{"missing final setting", "final", "allowSending", nil},
		{"null final setting", "final", "allowSending", json.RawMessage("null")},
		{"numeric final version", "final", "version", 8},
		{"overflow final version", "final", "version", "9223372036854775808"},
		{"wrong final state", "final", "available", "69"},
		{"fractional version", "after", "version", "8.5"},
		{"leading zero version", "after", "version", "08"},
		{"negative hold", "after", "onHold", "-1"},
		{"missing state", "after", "available", nil},
		{"wrong debt delta", "after", "overdraftUsed", "1"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request, response := adapterResultFixture(t)
			raw := adapterJSON(t, response)
			switch test.section {
			case "root":
				raw = mutateAdapterObject(t, raw, test.field, test.value)
			case "movement":
				response.Movements[0] = mutateAdapterObject(t, response.Movements[0], test.field, test.value)
				raw = adapterJSON(t, response)
			case "final":
				response.Final[0] = mutateAdapterObject(t, response.Final[0], test.field, test.value)
				raw = adapterJSON(t, response)
			case "after":
				var movement resultMovement
				require.NoError(t, json.Unmarshal(response.Movements[0], &movement))
				movement.After = mutateAdapterObject(t, movement.After, test.field, test.value)
				response.Movements[0] = adapterJSON(t, movement)
				raw = adapterJSON(t, response)
			}
			_, err := DecodeResult(raw, request)
			require.Error(t, err)
		})
	}
}

func TestDecodeResult_DuplicateKeysOrderAndExactPrecision(t *testing.T) {
	request, response := adapterResultFixture(t)
	raw := string(adapterJSON(t, response))
	_, err := DecodeResult([]byte(strings.Replace(raw, `"protocolVersion":1`, `"protocolVersion":1,"protocol\u0056ersion":1`, 1)), request)
	require.Error(t, err)
	_, err = DecodeResult([]byte(raw+` {}`), request)
	require.Error(t, err)
	response.Movements = append(response.Movements, response.Movements[0])
	_, err = DecodeResult(adapterJSON(t, response), request)
	require.Error(t, err)

	request, response = adapterResultFixture(t)
	var movement resultMovement
	require.NoError(t, json.Unmarshal(response.Movements[0], &movement))
	movement.Before = adapterJSON(t, resultState{"100", "0", "0", "9223372036854775806"})
	movement.After = adapterJSON(t, resultState{"70", "0", "0", "9223372036854775807"})
	response.Movements[0] = adapterJSON(t, movement)
	response.Final[0] = mutateAdapterObject(t, response.Final[0], "version", "9223372036854775807")
	result, err := DecodeResult(adapterJSON(t, response), request)
	require.NoError(t, err)
	require.Equal(t, int64(9223372036854775807), result.Final[0].Version)
}

func TestDecodeResult_RequiresDebtCompanion(t *testing.T) {
	request, response := adapterResultFixture(t)
	var movement resultMovement
	require.NoError(t, json.Unmarshal(response.Movements[0], &movement))
	movement.OverdraftDelta = "10"
	movement.After = adapterJSON(t, resultState{"70", "0", "10", "8"})
	response.Movements[0] = adapterJSON(t, movement)
	response.Final[0] = mutateAdapterObject(t, response.Final[0], "overdraftUsed", "10")
	_, err := DecodeResult(adapterJSON(t, response), request)
	require.Error(t, err)
}

func TestAccountingError_ClosedServerProtocol(t *testing.T) {
	request, _ := adapterResultFixture(t)
	failure := core.Failure{Code: core.FailureInsufficientFunds, TransactionIndex: 0, PostingIndex: 0, BalanceRef: request.Balances[0].BalanceRef}
	for _, prefix := range []string{"", "ERR "} {
		err := classifyAccountingError(accountingReply(prefix+"MIDAZ_ENGINE_V1 "+string(adapterJSON(t, failure))), request, nil)
		var business *core.Failure
		require.ErrorAs(t, err, &business)
		require.Equal(t, failure, *business)
	}
	for _, field := range []string{"transactionIndex", "postingIndex", "balanceRef", "code"} {
		raw := mutateAdapterObject(t, adapterJSON(t, failure), field, nil)
		err := classifyAccountingError(accountingReply("MIDAZ_ENGINE_V1 "+string(raw)), request, nil)
		assertAdapterTechnical(t, err, "invalid_failure", true)
	}
	for _, invalid := range []core.Failure{
		{Code: "future", TransactionIndex: 0, PostingIndex: 0, BalanceRef: failure.BalanceRef},
		{Code: failure.Code, TransactionIndex: 1, PostingIndex: 0, BalanceRef: failure.BalanceRef},
		{Code: failure.Code, TransactionIndex: 0, PostingIndex: -1, BalanceRef: failure.BalanceRef},
		{Code: failure.Code, TransactionIndex: 0, PostingIndex: 0, BalanceRef: "@foreign#default"},
	} {
		err := classifyAccountingError(accountingReply("MIDAZ_ENGINE_V1 "+string(adapterJSON(t, invalid))), request, nil)
		assertAdapterTechnical(t, err, "invalid_failure", true)
	}

	payload := "MIDAZ_ENGINE_V1 " + string(adapterJSON(t, failure))
	for _, prefix := range []string{"ERR ERR ", "prefix ", ""} {
		var cause error = accountingReply(prefix + payload)
		code := "script_runtime"
		if prefix == "" {
			cause, code = errors.New(payload), "transport"
		}
		err := classifyAccountingError(cause, request, nil)
		assertAdapterTechnical(t, err, code, true)
		require.ErrorIs(t, err, cause)
	}
}

func TestAccountingError_OutcomeCertaintyAndNormalizationScope(t *testing.T) {
	request, _ := adapterResultFixture(t)
	for _, test := range []struct {
		code      string
		uncertain bool
	}{
		{"execution_guard_conflict", false},
		{"execution_fingerprint_conflict", false},
		{"script_runtime_failed", false},
		{"execution_outcome_unknown", true},
		{"invalid_receipt", true},
		{"indeterminate", true},
		{"future", true},
	} {
		cause := accountingReply(`ERR MIDAZ_ENGINE_TECH_V1 {"code":"` + test.code + `","message":"detail"}`)
		code := test.code
		if code == "future" {
			code = "unknown_technical_failure"
		}
		err := classifyAccountingError(cause, request, nil)
		assertAdapterTechnical(t, err, code, test.uncertain)
		require.ErrorIs(t, err, cause)
	}
	keys := []string{"schedule", "recovery", "receipt", "guard", "balance", "balance:deleted"}
	for _, test := range []struct {
		raw, code string
		uncertain bool
	}{
		{`["balance"]`, "normalization_required", false},
		{`["balance:deleted"]`, "invalid_normalization_failure", true},
		{`[]`, "invalid_normalization_failure", true},
		{`{}`, "invalid_normalization_failure", true},
	} {
		err := classifyAccountingError(accountingReply("BALANCE_LIMIT_NORMALIZATION_REQUIRED:"+test.raw), request, keys)
		assertAdapterTechnical(t, err, test.code, test.uncertain)
	}
	require.True(t, isNoScript(accountingReply("NOSCRIPT missing")))
	require.False(t, isNoScript(errors.New("NOSCRIPT missing")))
	require.False(t, isNoScript(accountingReply("ERR NOSCRIPT missing")))
	require.False(t, isNoScript(accountingReply("runtime NOSCRIPT missing")))
}

func assertAdapterTechnical(t *testing.T, err error, code string, uncertain bool) {
	t.Helper()
	var technical *TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, code, technical.Code)
	require.Equal(t, uncertain, technical.Indeterminate)
}

func TestNewAdapter_ValidationBeforeProviderAccess(t *testing.T) {
	limits := Limits{MaxTransactions: 1, MaxPostings: 2, MaxBalances: 2, MaxRecoveryBytes: 4096, MaxRequestBytes: 8192, MaxPreparedBytes: 8192}
	provider := &countingProvider{}
	_, err := NewAdapter(nil, limits)
	require.Error(t, err)
	var missingProvider *countingProvider
	_, err = NewAdapter(missingProvider, limits)
	require.Error(t, err)
	for _, field := range []string{"transactions", "postings", "balances", "recovery", "request", "prepared"} {
		invalid := limits
		switch field {
		case "transactions":
			invalid.MaxTransactions = 0
		case "postings":
			invalid.MaxPostings = 0
		case "balances":
			invalid.MaxBalances = 0
		case "recovery":
			invalid.MaxRecoveryBytes = 0
		case "request":
			invalid.MaxRequestBytes = 0
		case "prepared":
			invalid.MaxPreparedBytes = 0
		}
		_, err := NewAdapter(provider, invalid)
		require.Error(t, err)
	}
	adapter, err := NewAdapter(provider, limits)
	require.NoError(t, err)
	_, err = adapter.Execute(context.Background(), command.EngineExecution{})
	require.Error(t, err)
	require.Zero(t, provider.calls)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = adapter.Execute(ctx, command.EngineExecution{})
	assertAdapterTechnical(t, err, "context_canceled", false)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, provider.calls)
}
