//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

type integrationClientProvider struct {
	client *redis.Client
	calls  int
}

type integrationCommandHook struct {
	evalSHA atomic.Int32
	eval    atomic.Int32
	noRetry atomic.Int32
}

func (h *integrationCommandHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *integrationCommandHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.NoRetry() {
			h.noRetry.Add(1)
		}
		switch strings.ToUpper(cmd.Name()) {
		case "EVALSHA":
			h.evalSHA.Add(1)
		case "EVAL":
			h.eval.Add(1)
		}
		return next(ctx, cmd)
	}
}

func (h *integrationCommandHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (p *integrationClientProvider) GetClient(context.Context) (redis.UniversalClient, error) {
	p.calls++
	return p.client, nil
}

func richAdapterExecution(t *testing.T) (command.EngineExecution, Limits) {
	t.Helper()
	request, _ := adapterResultFixture(t)
	request.ExecutionID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+":execution"))
	request.OrganizationID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+":organization"))
	request.LedgerID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+":ledger"))
	request.Transactions[0].ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+":transaction"))
	request.Transactions[0].Postings[0].DrawPolicy = core.DrawForbidden
	input := command.EngineExecution{Request: request}
	transaction := request.Transactions[0]
	balance := request.Balances[0]
	projectionBalance := command.FrozenProjectionBalance{}
	projectionBalance.ID, projectionBalance.AccountID = balance.ID.String(), balance.AccountID.String()
	projectionBalance.OrganizationID, projectionBalance.LedgerID = request.OrganizationID.String(), request.LedgerID.String()
	projectionBalance.Alias, projectionBalance.Key, projectionBalance.AssetCode = balance.Alias, balance.Key, balance.AssetCode
	projectionBalance.AccountType = balance.AccountType
	projectionBalance.Available, projectionBalance.Version = balance.Available, balance.Version
	date := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	payload := command.BalanceEngineRecoveryPayload{
		FormatVersion: 2, TransactionID: transaction.ID, OrganizationID: request.OrganizationID, LedgerID: request.LedgerID,
		ExecutionID: request.ExecutionID, TTL: date.Add(24 * time.Hour), TransactionDate: date,
		TransactionCreatedAt: date, TransactionUpdatedAt: date, OperationUpdatedAt: date,
		Action: "CREATE", TransactionStatus: "APPROVED",
		Projection: []command.FrozenProjectionContext{{
			TransactionID: transaction.ID, PostingRef: transaction.Postings[0].Ref, BalanceRef: balance.BalanceRef, Role: core.RolePrimary,
			Side: command.ProjectionSideFrom, RowType: "DEBIT", Direction: "credit", Balance: projectionBalance,
			RequestedAmount: decimal.NewFromInt(30), CompatibilityPath: command.ProjectionStandard,
		}},
	}
	input.Guards = []command.ExecutionGuard{{TransactionID: transaction.ID, NextToken: "executed-once"}}
	input.Recovery = []command.RecoveryIntent{{TransactionID: transaction.ID, Payload: encodeAdapterRecovery(t, &input, payload)}}
	require.NoError(t, command.ValidateBalanceEngineRecovery(input))
	return input, Limits{MaxTransactions: 4, MaxPostings: 16, MaxBalances: 16, MaxRecoveryBytes: 1 << 20, MaxRequestBytes: 1 << 20, MaxPreparedBytes: 1 << 20}
}

func encodeAdapterRecovery(t testing.TB, input *command.EngineExecution, payload command.BalanceEngineRecoveryPayload) json.RawMessage {
	t.Helper()
	require.Len(t, input.Request.Transactions, 1)
	transaction := command.BalanceEngineTransactionIntent{
		TransactionID: payload.TransactionID, ParentTransactionID: payload.ParentTransactionID,
		FeesSkipped: payload.FeesSkipped, TracerSkipped: payload.TracerSkipped, Action: payload.Action,
		TransactionStatus: payload.TransactionStatus, TransactionDate: payload.TransactionDate, Input: payload.TransactionInput,
		TransactionCreatedAt: payload.TransactionCreatedAt, TransactionUpdatedAt: payload.TransactionUpdatedAt, OperationUpdatedAt: payload.OperationUpdatedAt,
		PostingRefs: make([]string, 0, len(input.Request.Transactions[0].Postings)),
		Projection:  make([]command.FrozenProjectionIntent, 0, len(payload.Projection)),
	}
	for _, posting := range input.Request.Transactions[0].Postings {
		transaction.PostingRefs = append(transaction.PostingRefs, posting.Ref)
	}
	for _, projection := range payload.Projection {
		transaction.Projection = append(transaction.Projection, projection.Intent())
	}
	fingerprint, err := command.ComputeBalanceEngineIntentFingerprint(command.BalanceEngineIntent{
		TenantID: payload.TenantID, OrganizationID: input.Request.OrganizationID, LedgerID: input.Request.LedgerID,
		ExecutionID: input.Request.ExecutionID, Transactions: []command.BalanceEngineTransactionIntent{transaction},
	})
	require.NoError(t, err)
	input.IntentFingerprint, payload.IntentFingerprint = fingerprint, fingerprint
	encoded, err := command.EncodeBalanceEngineRecoveryPayload(payload)
	require.NoError(t, err)
	return encoded
}

func newAdapterValkey(t testing.TB) (*redis.Client, string, string) {
	t.Helper()
	ctx := context.Background()
	const password = "isolated-adapter-integration-password"
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "valkey/valkey:8", ExposedPorts: []string{"6379/tcp"},
			Cmd: []string{"valkey-server", "--requirepass", password}, WaitingFor: wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	address := net.JoinHostPort(host, port.Port())
	inspector := redis.NewClient(&redis.Options{Addr: address, Password: password, DB: 2, Protocol: 2, MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, inspector.Close()) })
	info, err := inspector.Info(ctx, "server").Result()
	require.NoError(t, err)
	require.Contains(t, info, "valkey_version:")
	return inspector, address, password
}

func TestIntegration_AdapterExecute_TransportAndReceipt(t *testing.T) {
	ctx := context.Background()
	inspector, address, password := newAdapterValkey(t)

	for _, drop := range []bool{false, true} {
		t.Run(fmt.Sprintf("drop_committed_response_%t", drop), func(t *testing.T) {
			require.NoError(t, inspector.ScriptFlush(ctx).Err())
			proxy := newAccountingProxy(t, address, drop)
			var connects, dials atomic.Int32
			options := &redis.Options{
				Addr: proxy.listener.Addr().String(), Username: "default", Password: password, DB: 2, Protocol: 2,
				MaxRetries: 3, ClientName: "adapter-integration", TLSConfig: proxy.clientTLS,
				Dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
					dials.Add(1)
					return (&tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: proxy.clientTLS}).DialContext(ctx, network, address)
				},
				OnConnect: func(context.Context, *redis.Conn) error { connects.Add(1); return nil },
			}
			shared := redis.NewClient(options)
			t.Cleanup(func() { require.NoError(t, shared.Close()) })
			provider := &integrationClientProvider{client: shared}
			input, limits := richAdapterExecution(t)
			adapter, err := NewAdapter(provider, limits)
			require.NoError(t, err)
			result, err := adapter.Execute(ctx, input)
			if drop {
				assertAdapterTechnical(t, err, "transport", true)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.True(t, result.Final[0].Available.Equal(decimal.NewFromInt(70)))
			}
			require.Equal(t, 1, proxy.count("EVALSHA"), "failed calls must not be retransmitted")
			require.Equal(t, 1, proxy.count("EVAL"), "only NOSCRIPT permits EVAL fallback")
			keys, err := resolveAdapterKeys(ctx, input.Request)
			require.NoError(t, err)
			before := captureAdapterState(t, inspector, keys)
			for range 2 {
				replayed, err := adapter.Execute(ctx, input)
				require.NoError(t, err)
				require.Len(t, replayed.Movements, 1)
				require.True(t, replayed.Final[0].Available.Equal(decimal.NewFromInt(70)))
				require.Equal(t, int64(8), replayed.Final[0].Version)
				require.Equal(t, before, captureAdapterState(t, inspector, keys), "explicit receipt replay must not mutate accounting or expiry")
			}
			require.Equal(t, 3, proxy.count("EVALSHA"))
			require.Equal(t, 1, proxy.count("EVAL"))
			require.Equal(t, 3, shared.Options().MaxRetries)
			require.Same(t, proxy.clientTLS, shared.Options().TLSConfig)
			require.Equal(t, "default", shared.Options().Username)
			require.Equal(t, password, shared.Options().Password)
			require.Equal(t, 2, shared.Options().DB)
			// The shared client may reuse one pooled connection across receipt replays;
			// a lost response can cause an additional reconnect, but never requires one
			// connection per Execute call.
			require.GreaterOrEqual(t, dials.Load(), int32(1))
			require.GreaterOrEqual(t, connects.Load(), int32(1))
			require.NoError(t, shared.Ping(ctx).Err(), "adapter must not close the shared client")
		})
	}
}

func TestIntegration_AdapterExecute_UsesSharedCommandHooks(t *testing.T) {
	ctx := context.Background()
	inspector, address, password := newAdapterValkey(t)
	shared := redis.NewClient(&redis.Options{
		Addr: address, Password: password, DB: 2, Protocol: 2, MaxRetries: 3,
	})
	t.Cleanup(func() { require.NoError(t, shared.Close()) })
	hook := &integrationCommandHook{}
	shared.AddHook(hook)

	input, limits := richAdapterExecution(t)
	adapter, err := NewAdapter(&integrationClientProvider{client: shared}, limits)
	require.NoError(t, err)
	_, err = adapter.Execute(ctx, input)
	require.NoError(t, err)

	require.GreaterOrEqual(t, hook.evalSHA.Load(), int32(1), "shared command hook must observe EVALSHA")
	require.GreaterOrEqual(t, hook.eval.Load(), int32(1), "shared command hook must observe NOSCRIPT EVAL fallback")
	require.GreaterOrEqual(t, hook.noRetry.Load(), hook.evalSHA.Load()+hook.eval.Load(), "script commands must be marked NoRetry")
	require.Equal(t, 3, shared.Options().MaxRetries)
	require.NoError(t, shared.Ping(ctx).Err(), "adapter must not close the shared client")
	require.NoError(t, inspector.ScriptFlush(ctx).Err())
}

func TestIntegration_AdapterExecute_WritesDualBalanceCacheContract(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)

	for _, preexisting := range []bool{false, true} {
		name := "absent_seed"
		if preexisting {
			name = "existing_mutation"
		}

		t.Run(name, func(t *testing.T) {
			input, limits := richAdapterExecution(t)
			balance := &input.Request.Balances[0]
			balance.Available = decimal.NewFromInt(100)
			balance.OnHold = decimal.Zero
			balance.OverdraftUsed = decimal.Zero
			balance.OverdraftLimit = decimal.Zero
			balance.Version = 9007199254740992
			balance.AllowSending = true
			balance.AllowReceiving = false
			balance.AllowOverdraft = true
			balance.OverdraftLimitEnabled = false

			payload, err := command.DecodeBalanceEngineRecoveryPayload(input.Recovery[0].Payload)
			require.NoError(t, err)
			payload.Projection[0].Balance.Available = balance.Available
			payload.Projection[0].Balance.Version = balance.Version
			input.Recovery[0].Payload = encodeAdapterRecovery(t, &input, *payload)
			require.NoError(t, command.ValidateBalanceEngineRecovery(input))

			keys, err := resolveAdapterKeys(ctx, input.Request)
			require.NoError(t, err)
			cacheKey := keys.Balances[balance.BalanceRef].Balance
			if preexisting {
				raw := fmt.Sprintf(
					`{
					"SchemaVersion":2,
					"ID":%q,"id":%q,"AccountID":%q,"accountId":%q,
					"AccountType":%q,"accountType":%q,"AssetCode":%q,"assetCode":%q,
					"Alias":%q,"alias":%q,"Key":%q,"key":%q,
					"Direction":%q,"direction":%q,"BalanceScope":%q,"balanceScope":%q,
					"Available":"100","available":"100","OnHold":"0","onHold":"0",
					"OverdraftUsed":"0","overdraftUsed":"0","Version":9007199254740992,"version":"9007199254740992",
					"AllowSending":1,"allowSending":true,"AllowReceiving":0,"allowReceiving":false,
					"AllowOverdraft":1,"allowOverdraft":true,"OverdraftLimitEnabled":0,"overdraftLimitEnabled":false,
					"OverdraftLimit":"0","overdraftLimit":"0",
					"writerExtension":{"exact":9007199254740997,"kept":true}
				}`,
					balance.ID.String(), balance.ID.String(), balance.AccountID.String(), balance.AccountID.String(),
					balance.AccountType, balance.AccountType, balance.AssetCode, balance.AssetCode,
					balance.Alias, balance.Alias, balance.Key, balance.Key,
					balance.Direction, balance.Direction, balance.BalanceScope, balance.BalanceScope,
				)
				require.NoError(t, inspector.Set(ctx, cacheKey, raw, 0).Err())
			}

			adapter, err := NewAdapter(&integrationClientProvider{client: inspector}, limits)
			require.NoError(t, err)
			result, err := adapter.Execute(ctx, input)
			require.NoError(t, err)
			require.NotNil(t, result)

			raw, err := inspector.Get(ctx, cacheKey).Bytes()
			require.NoError(t, err)
			assertAdapterDualBalanceCache(t, raw, input.Request.Balances[0], preexisting)
		})
	}
}

func assertAdapterDualBalanceCache(t *testing.T, raw []byte, balance core.BalanceSnapshot, hasExtension bool) {
	t.Helper()

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	wantFieldCount := 35
	if hasExtension {
		wantFieldCount++
	}
	require.Len(t, fields, wantFieldCount)
	require.Equal(t, json.RawMessage("2"), fields["SchemaVersion"])

	textPairs := map[string]struct {
		lower string
		want  string
	}{
		"ID":             {lower: "id", want: balance.ID.String()},
		"AccountID":      {lower: "accountId", want: balance.AccountID.String()},
		"AccountType":    {lower: "accountType", want: balance.AccountType},
		"AssetCode":      {lower: "assetCode", want: balance.AssetCode},
		"Alias":          {lower: "alias", want: balance.Alias},
		"Key":            {lower: "key", want: balance.Key},
		"Direction":      {lower: "direction", want: balance.Direction},
		"BalanceScope":   {lower: "balanceScope", want: balance.BalanceScope},
		"Available":      {lower: "available", want: "70"},
		"OnHold":         {lower: "onHold", want: "0"},
		"OverdraftUsed":  {lower: "overdraftUsed", want: "0"},
		"OverdraftLimit": {lower: "overdraftLimit", want: "0"},
	}
	for upper, expected := range textPairs {
		want, err := json.Marshal(expected.want)
		require.NoError(t, err)
		require.Equal(t, json.RawMessage(want), fields[upper], upper)
		require.Equal(t, json.RawMessage(want), fields[expected.lower], expected.lower)
	}

	require.Equal(t, json.RawMessage("9007199254740993"), fields["Version"])
	require.Equal(t, json.RawMessage(`"9007199254740993"`), fields["version"])
	for upper, expected := range map[string][3]string{
		"AllowSending":          {"allowSending", "1", "true"},
		"AllowReceiving":        {"allowReceiving", "0", "false"},
		"AllowOverdraft":        {"allowOverdraft", "1", "true"},
		"OverdraftLimitEnabled": {"overdraftLimitEnabled", "0", "false"},
	} {
		require.Equal(t, json.RawMessage(expected[1]), fields[upper], upper)
		require.Equal(t, json.RawMessage(expected[2]), fields[expected[0]], expected[0])
	}

	if hasExtension {
		require.Equal(t, json.RawMessage(`{"exact":9007199254740997,"kept":true}`), fields["writerExtension"])
	}
}

func TestIntegration_AdapterExecute_PostWriteFailureIsIndeterminate(t *testing.T) {
	ctx := context.Background()
	inspector, address, password := newAdapterValkey(t)
	require.NoError(t, inspector.Do(ctx, "ACL", "SETUSER", "commit-fault", "on", ">"+password, "~*", "+@all", "-zadd").Err())
	proxy := newAccountingProxy(t, address, false)
	shared := redis.NewClient(&redis.Options{
		Addr: proxy.listener.Addr().String(), Username: "commit-fault", Password: password,
		DB: 2, Protocol: 2, TLSConfig: proxy.clientTLS, MaxRetries: 3,
	})
	t.Cleanup(func() { require.NoError(t, shared.Close()) })
	input, limits := richAdapterExecution(t)
	adapter, err := NewAdapter(&integrationClientProvider{client: shared}, limits)
	require.NoError(t, err)
	keys, err := resolveAdapterKeys(ctx, input.Request)
	require.NoError(t, err)
	before := captureAdapterState(t, inspector, keys)
	result, err := adapter.Execute(ctx, input)
	require.Nil(t, result)
	assertAdapterTechnical(t, err, "indeterminate", true)
	var server redis.Error
	require.ErrorAs(t, err, &server)
	require.Equal(t, 1, proxy.count("EVALSHA"))
	require.Equal(t, 1, proxy.count("EVAL"), "post-write errors must never trigger financial replay")
	require.NotEqual(t, before, captureAdapterState(t, inspector, keys), "Valkey does not roll back the preceding SET")
	cacheKey := keys.Balances[input.Request.Balances[0].BalanceRef].Balance
	raw, err := inspector.Get(ctx, cacheKey).Bytes()
	require.NoError(t, err)
	var balance map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &balance))
	require.JSONEq(t, `"70"`, string(balance["available"]))
	require.JSONEq(t, `"8"`, string(balance["version"]))
	require.JSONEq(t, `"70"`, string(balance["Available"]))
	require.JSONEq(t, `8`, string(balance["Version"]))
	for _, key := range []string{keys.Schedule, keys.Recovery, keys.Guards, keys.Receipts} {
		exists, err := inspector.Exists(ctx, key).Result()
		require.NoError(t, err)
		require.Zero(t, exists, "later commit commands were not completed")
	}
}

func TestIntegration_AdapterExecute_CorruptReceiptIsIndeterminate(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	for _, corrupted := range []string{`{`, `[]`, `{}`, `{"formatVersion":1}`, "empty saved response"} {
		t.Run(corrupted, func(t *testing.T) {
			input, limits := richAdapterExecution(t)
			adapter, err := NewAdapter(&integrationClientProvider{client: inspector}, limits)
			require.NoError(t, err)
			_, err = adapter.Execute(ctx, input)
			require.NoError(t, err)
			keys, err := resolveAdapterKeys(ctx, input.Request)
			require.NoError(t, err)
			if corrupted == "empty saved response" {
				receipt, err := inspector.HGet(ctx, keys.Receipts, input.Request.ExecutionID.String()).Bytes()
				require.NoError(t, err)
				corrupted = string(mutateAdapterObject(t, receipt, "response", `{"protocolVersion":1,"movements":[],"final":[]}`))
			}
			require.NoError(t, inspector.HSet(ctx, keys.Receipts, input.Request.ExecutionID.String(), corrupted).Err())
			before := captureAdapterState(t, inspector, keys)
			_, err = adapter.Execute(ctx, input)
			assertAdapterTechnical(t, err, "invalid_receipt", true)
			require.Equal(t, before, captureAdapterState(t, inspector, keys))
		})
	}
}

func captureAdapterState(t *testing.T, client *redis.Client, keys resolvedExecutionKeys) map[string]any {
	t.Helper()
	state := make(map[string]any)
	inventory := []string{keys.Schedule, keys.Recovery, keys.Receipts, keys.Guards}
	for _, balance := range keys.Balances {
		inventory = append(inventory, balance.Balance, balance.Deleted)
	}
	for _, key := range inventory {
		dump, err := client.Dump(context.Background(), key).Result()
		if errors.Is(err, redis.Nil) {
			dump, err = "", nil
		}
		require.NoError(t, err)
		expiry, err := client.Do(context.Background(), "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		state[key] = []any{dump, expiry}
	}
	return state
}

func TestIntegration_AdapterExecute_RejectsRecoveryTenantBeforeProvider(t *testing.T) {
	input, limits := richAdapterExecution(t)
	payload, err := command.DecodeBalanceEngineRecoveryPayload(input.Recovery[0].Payload)
	require.NoError(t, err)
	payload.TenantID = "unrelated-tenant"
	input.Recovery[0].Payload = encodeAdapterRecovery(t, &input, *payload)
	require.NoError(t, command.ValidateBalanceEngineRecovery(input))
	provider := &integrationClientProvider{}
	adapter, err := NewAdapter(provider, limits)
	require.NoError(t, err)
	_, err = adapter.Execute(context.Background(), input)
	assertAdapterTechnical(t, err, "invalid_recovery", false)
	require.ErrorContains(t, err, "authenticated scope")
	require.Zero(t, provider.calls)
}

func TestIntegration_AdapterExecute_RejectsMutatedIntentBeforeProvider(t *testing.T) {
	input, limits := richAdapterExecution(t)
	payload, err := command.DecodeBalanceEngineRecoveryPayload(input.Recovery[0].Payload)
	require.NoError(t, err)
	payload.Projection[0].Description = "changed immutable attribution"
	input.Recovery[0].Payload, err = command.EncodeBalanceEngineRecoveryPayload(*payload)
	require.NoError(t, err)
	provider := &integrationClientProvider{}
	adapter, err := NewAdapter(provider, limits)
	require.NoError(t, err)
	_, err = adapter.Execute(context.Background(), input)
	assertAdapterTechnical(t, err, "invalid_recovery", false)
	require.ErrorContains(t, err, "fingerprint")
	require.Zero(t, provider.calls)
}

// accountingProxy forwards complete RESP messages so a dropped successful reply
// proves that Valkey applied EVAL before the caller observed a transport failure.
type accountingProxy struct {
	listener  net.Listener
	clientTLS *tls.Config
	upstream  string
	drop      atomic.Bool
	mu        sync.Mutex
	counts    map[string]int
	conns     map[net.Conn]bool
	replyHook func(string, []byte)
	wg        sync.WaitGroup
}

func newAccountingProxy(t *testing.T, upstream string, drop bool) *accountingProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverTLS, clientTLS := accountingProxyTLS(t)
	proxy := &accountingProxy{listener: tls.NewListener(listener, serverTLS), clientTLS: clientTLS, upstream: upstream, counts: make(map[string]int), conns: make(map[net.Conn]bool)}
	proxy.drop.Store(drop)
	proxy.wg.Add(1)
	go proxy.accept()
	t.Cleanup(func() {
		_ = proxy.listener.Close()
		proxy.mu.Lock()
		for conn := range proxy.conns {
			_ = conn.Close()
		}
		proxy.mu.Unlock()
		proxy.wg.Wait()
	})
	return proxy
}

func accountingProxyTLS(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"},
		NotBefore: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	encoded, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(encoded)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{encoded}, PrivateKey: private}}},
		&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "localhost"}
}

func (p *accountingProxy) accept() {
	defer p.wg.Done()
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		p.conns[conn] = true
		p.mu.Unlock()
		p.wg.Add(1)
		go p.forward(conn)
	}
}

func (p *accountingProxy) forward(downstream net.Conn) {
	defer p.wg.Done()
	defer func() {
		_ = downstream.Close()
		p.mu.Lock()
		delete(p.conns, downstream)
		p.mu.Unlock()
	}()
	upstream, err := net.DialTimeout("tcp", p.upstream, 5*time.Second)
	if err != nil {
		return
	}
	defer upstream.Close()
	requests, replies := bufio.NewReader(downstream), bufio.NewReader(upstream)
	for {
		request, err := readAccountingRESP(requests)
		if err != nil {
			return
		}
		command := accountingCommand(request)
		p.mu.Lock()
		p.counts[command]++
		p.mu.Unlock()
		if _, err := upstream.Write(request); err != nil {
			return
		}
		reply, err := readAccountingRESP(replies)
		if err != nil {
			return
		}
		p.mu.Lock()
		replyHook := p.replyHook
		p.mu.Unlock()
		if replyHook != nil {
			replyHook(command, reply)
		}
		if (command == "EVAL" || command == "EVALSHA") && len(reply) > 0 && reply[0] != '-' && p.drop.CompareAndSwap(true, false) {
			return
		}
		if _, err := downstream.Write(reply); err != nil {
			return
		}
	}
}

func (p *accountingProxy) count(command string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.counts[command]
}

func (p *accountingProxy) setReplyHook(hook func(string, []byte)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.replyHook = hook
}

func accountingCommand(raw []byte) string {
	reader := bufio.NewReader(bytes.NewReader(raw))
	_, _ = reader.ReadString('\n')
	line, err := reader.ReadString('\n')
	if err != nil || len(line) < 3 || line[0] != '$' {
		return ""
	}
	size, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil || size < 0 || size > 1024 {
		return ""
	}
	name := make([]byte, size)
	if _, err := io.ReadFull(reader, name); err != nil {
		return ""
	}
	return strings.ToUpper(string(name))
}

func readAccountingRESP(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 {
		return nil, errors.New("invalid RESP header")
	}
	switch line[0] {
	case '$', '*':
		size, err := strconv.Atoi(strings.TrimSpace(string(line[1:])))
		if err != nil || size < -1 || size > 1<<24 {
			return nil, errors.New("invalid RESP length")
		}
		if size == -1 {
			return line, nil
		}
		if line[0] == '$' {
			body := make([]byte, size+2)
			if _, err := io.ReadFull(reader, body); err != nil {
				return nil, err
			}
			return append(line, body...), nil
		}
		for range size {
			child, err := readAccountingRESP(reader)
			if err != nil {
				return nil, err
			}
			line = append(line, child...)
		}
	case '+', '-', ':':
	default:
		return nil, fmt.Errorf("unsupported RESP type %q", line[0])
	}
	return line, nil
}
