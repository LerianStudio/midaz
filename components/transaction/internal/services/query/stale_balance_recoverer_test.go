// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

func TestExtractRecoveredBalances_AccumulatesAcrossReplayedRecords(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	aliasWithKey := "@alice#default"
	queueAlias := "0#@alice#default"
	aliases := map[string]struct{}{aliasWithKey: {}}

	recordOne := mustReplayRecord(
		t,
		organizationID,
		ledgerID,
		queueAlias,
		decimal.RequireFromString("1000"),
		decimal.Zero,
		10,
		pkgTransaction.Amount{
			Asset:           "USD",
			Value:           decimal.RequireFromString("50"),
			Operation:       constant.DEBIT,
			TransactionType: constant.CREATED,
		},
	)

	recordTwo := mustReplayRecord(
		t,
		organizationID,
		ledgerID,
		queueAlias,
		decimal.RequireFromString("1000"),
		decimal.Zero,
		10,
		pkgTransaction.Amount{
			Asset:           "USD",
			Value:           decimal.RequireFromString("30"),
			Operation:       constant.DEBIT,
			TransactionType: constant.CREATED,
		},
	)

	recovered, err := extractRecoveredBalances(recordOne, organizationID, ledgerID, aliases, nil)
	require.NoError(t, err)

	recovered, err = extractRecoveredBalances(recordTwo, organizationID, ledgerID, aliases, recovered)
	require.NoError(t, err)

	balance, ok := recovered[aliasWithKey]
	require.True(t, ok)
	require.NotNil(t, balance)

	assert.Equal(t, "@alice", balance.Alias)
	assert.Equal(t, constant.DefaultBalanceKey, balance.Key)
	assert.True(t, decimal.RequireFromString("920").Equal(balance.Available))
	assert.True(t, decimal.Zero.Equal(balance.OnHold))
	assert.Equal(t, int64(12), balance.Version)
}

func TestExtractRecoveredBalances_IgnoresMismatchedOrganizationOrLedger(t *testing.T) {
	t.Parallel()

	record := mustReplayRecord(
		t,
		uuid.New(),
		uuid.New(),
		"0#@alice#default",
		decimal.RequireFromString("100"),
		decimal.Zero,
		1,
		pkgTransaction.Amount{
			Asset:           "USD",
			Value:           decimal.RequireFromString("10"),
			Operation:       constant.DEBIT,
			TransactionType: constant.CREATED,
		},
	)

	recovered, err := extractRecoveredBalances(
		record,
		uuid.New(),
		uuid.New(),
		map[string]struct{}{"@alice#default": {}},
		nil,
	)

	require.NoError(t, err)
	assert.Empty(t, recovered)
}

func TestExtractRecoveredBalances_ReturnsErrorOnMalformedPayload(t *testing.T) {
	t.Parallel()

	_, err := extractRecoveredBalances(
		[]byte("not-msgpack"),
		uuid.New(),
		uuid.New(),
		map[string]struct{}{},
		nil,
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to unmarshal queue envelope")
}

func TestFranzStaleBalanceRecoverer_NoOpCases(t *testing.T) {
	t.Parallel()

	r := &FranzStaleBalanceRecoverer{}

	recovered, err := r.RecoverLaggedAliases(
		context.Background(),
		"ledger.balance.operations",
		uuid.New(),
		uuid.New(),
		map[int32][]string{1: {"@alice#default"}},
	)
	require.NoError(t, err)
	assert.Empty(t, recovered)

	recovered, err = r.replayPartitionRange(
		context.Background(),
		"ledger.balance.operations",
		0,
		10,
		10,
		uuid.New(),
		uuid.New(),
		map[string]struct{}{},
	)
	require.NoError(t, err)
	assert.Empty(t, recovered)
}

func mustReplayRecord(
	t *testing.T,
	organizationID, ledgerID uuid.UUID,
	alias string,
	available decimal.Decimal,
	onHold decimal.Decimal,
	version int64,
	amount pkgTransaction.Amount,
) []byte {
	t.Helper()

	payload := transaction.TransactionProcessingPayload{
		Validate: &pkgTransaction.Responses{
			From: map[string]pkgTransaction.Amount{alias: amount},
			To:   map[string]pkgTransaction.Amount{},
		},
		Balances: []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      uuid.New().String(),
				Alias:          alias,
				Key:            constant.DefaultBalanceKey,
				AssetCode:      "USD",
				Available:      available,
				OnHold:         onHold,
				Version:        version,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
			},
		},
	}

	payloadBytes, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	queueEnvelope := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData: []mmodel.QueueData{
			{ID: uuid.New(), Value: payloadBytes},
		},
	}

	envelopeBytes, err := msgpack.Marshal(queueEnvelope)
	require.NoError(t, err)

	return envelopeBytes
}

// mustEnvelopeFromPayloads builds a raw msgpack queue envelope containing multiple
// QueueData items, each from a separate TransactionProcessingPayload. This allows
// tests to exercise multi-batch records and mixed From/To scenarios that the
// simpler mustReplayRecord helper cannot express.
func mustEnvelopeFromPayloads(
	t *testing.T,
	organizationID, ledgerID uuid.UUID,
	payloads []transaction.TransactionProcessingPayload,
) []byte {
	t.Helper()

	queueItems := make([]mmodel.QueueData, 0, len(payloads))

	for _, p := range payloads {
		b, err := msgpack.Marshal(p)
		require.NoError(t, err)

		queueItems = append(queueItems, mmodel.QueueData{
			ID:    uuid.New(),
			Value: b,
		})
	}

	envelope := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueItems,
	}

	out, err := msgpack.Marshal(envelope)
	require.NoError(t, err)

	return out
}

// makeBalance is a compact helper to build an *mmodel.Balance for test payloads.
func makeBalance(
	orgID, ledgerID uuid.UUID,
	alias string,
	available, onHold decimal.Decimal,
	version int64,
) *mmodel.Balance {
	return &mmodel.Balance{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      uuid.New().String(),
		Alias:          alias,
		Key:            constant.DefaultBalanceKey,
		AssetCode:      "USD",
		Available:      available,
		OnHold:         onHold,
		Version:        version,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
}

// ---------------------------------------------------------------------------
// Additional tests for extractRecoveredBalances and replayPartitionRange
// ---------------------------------------------------------------------------.

// TestExtractRecoveredBalances_MultipleBatchesInSingleRecord verifies that when
// a single queue envelope holds multiple QueueData items (batches), every balance
// operation across all batches is extracted and accumulated correctly.
func TestExtractRecoveredBalances_MultipleBatchesInSingleRecord(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	aliasWithKey := "@alice#default"
	queueAlias := "0#@alice#default"
	aliases := map[string]struct{}{aliasWithKey: {}}

	// Batch 1: debit 100 from 1000 → available 900
	payloadOne := transaction.TransactionProcessingPayload{
		Validate: &pkgTransaction.Responses{
			From: map[string]pkgTransaction.Amount{
				queueAlias: {
					Asset:           "USD",
					Value:           decimal.RequireFromString("100"),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{},
		},
		Balances: []*mmodel.Balance{
			makeBalance(organizationID, ledgerID, queueAlias,
				decimal.RequireFromString("1000"), decimal.Zero, 5),
		},
	}

	// Batch 2: debit 200 more (the function should use accumulated balance
	// from batch 1 as the base, so 900 - 200 = 700)
	payloadTwo := transaction.TransactionProcessingPayload{
		Validate: &pkgTransaction.Responses{
			From: map[string]pkgTransaction.Amount{
				queueAlias: {
					Asset:           "USD",
					Value:           decimal.RequireFromString("200"),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{},
		},
		Balances: []*mmodel.Balance{
			makeBalance(organizationID, ledgerID, queueAlias,
				decimal.RequireFromString("1000"), decimal.Zero, 5),
		},
	}

	record := mustEnvelopeFromPayloads(t, organizationID, ledgerID,
		[]transaction.TransactionProcessingPayload{payloadOne, payloadTwo})

	recovered, err := extractRecoveredBalances(record, organizationID, ledgerID, aliases, nil)
	require.NoError(t, err)

	balance, ok := recovered[aliasWithKey]
	require.True(t, ok)
	require.NotNil(t, balance)

	// 1000 - 100 = 900 (batch 1), then 900 - 200 = 700 (batch 2).
	assert.True(t, decimal.RequireFromString("700").Equal(balance.Available),
		"expected 700, got %s", balance.Available)
	assert.True(t, decimal.Zero.Equal(balance.OnHold))
	// Version starts at 5, incremented twice: 5 → 6 → 7
	assert.Equal(t, int64(7), balance.Version)
}

// TestExtractRecoveredBalances_CreditAndDebitInSameRecord verifies that a single
// queue record containing both From (debit) and To (credit) operations for
// different aliases applies both correctly.
func TestExtractRecoveredBalances_CreditAndDebitInSameRecord(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	aliceAlias := "0#@alice#default"
	bobAlias := "1#@bob#default"

	aliases := map[string]struct{}{
		"@alice#default": {},
		"@bob#default":   {},
	}

	payload := transaction.TransactionProcessingPayload{
		Validate: &pkgTransaction.Responses{
			From: map[string]pkgTransaction.Amount{
				aliceAlias: {
					Asset:           "USD",
					Value:           decimal.RequireFromString("250"),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{
				bobAlias: {
					Asset:           "USD",
					Value:           decimal.RequireFromString("250"),
					Operation:       constant.CREDIT,
					TransactionType: constant.CREATED,
				},
			},
		},
		Balances: []*mmodel.Balance{
			makeBalance(organizationID, ledgerID, aliceAlias,
				decimal.RequireFromString("500"), decimal.Zero, 1),
			makeBalance(organizationID, ledgerID, bobAlias,
				decimal.RequireFromString("100"), decimal.Zero, 3),
		},
	}

	record := mustEnvelopeFromPayloads(t, organizationID, ledgerID,
		[]transaction.TransactionProcessingPayload{payload})

	recovered, err := extractRecoveredBalances(record, organizationID, ledgerID, aliases, nil)
	require.NoError(t, err)
	require.Len(t, recovered, 2)

	alice := recovered["@alice#default"]
	require.NotNil(t, alice)
	assert.True(t, decimal.RequireFromString("250").Equal(alice.Available),
		"expected alice available=250, got %s", alice.Available)
	assert.Equal(t, int64(2), alice.Version)

	bob := recovered["@bob#default"]
	require.NotNil(t, bob)
	assert.True(t, decimal.RequireFromString("350").Equal(bob.Available),
		"expected bob available=350, got %s", bob.Available)
	assert.Equal(t, int64(4), bob.Version)
}

// TestExtractRecoveredBalances_SkipsNilBalances ensures that nil entries in the
// Balances slice are safely skipped without causing a nil-pointer dereference.
func TestExtractRecoveredBalances_SkipsNilBalances(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	queueAlias := "0#@alice#default"
	aliases := map[string]struct{}{"@alice#default": {}}

	payload := transaction.TransactionProcessingPayload{
		Validate: &pkgTransaction.Responses{
			From: map[string]pkgTransaction.Amount{
				queueAlias: {
					Asset:           "USD",
					Value:           decimal.RequireFromString("10"),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{},
		},
		Balances: []*mmodel.Balance{
			nil, // deliberately nil — must be skipped
			makeBalance(organizationID, ledgerID, queueAlias,
				decimal.RequireFromString("500"), decimal.Zero, 1),
			nil, // trailing nil
		},
	}

	record := mustEnvelopeFromPayloads(t, organizationID, ledgerID,
		[]transaction.TransactionProcessingPayload{payload})

	recovered, err := extractRecoveredBalances(record, organizationID, ledgerID, aliases, nil)
	require.NoError(t, err)

	balance, ok := recovered["@alice#default"]
	require.True(t, ok, "expected alice balance to be recovered despite nil siblings")
	require.NotNil(t, balance)
	assert.True(t, decimal.RequireFromString("490").Equal(balance.Available))
	assert.Equal(t, int64(2), balance.Version)
}

// TestExtractRecoveredBalances_OnlyRecoverRequestedAliases verifies that only
// aliases present in the requested set are recovered; operations for other
// aliases in the same record are silently ignored.
func TestExtractRecoveredBalances_OnlyRecoverRequestedAliases(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	aliceAlias := "0#@alice#default"
	bobAlias := "1#@bob#default"

	// Only request alice — bob should be ignored.
	aliases := map[string]struct{}{
		"@alice#default": {},
	}

	payload := transaction.TransactionProcessingPayload{
		Validate: &pkgTransaction.Responses{
			From: map[string]pkgTransaction.Amount{
				aliceAlias: {
					Asset:           "USD",
					Value:           decimal.RequireFromString("50"),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
				bobAlias: {
					Asset:           "USD",
					Value:           decimal.RequireFromString("75"),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{},
		},
		Balances: []*mmodel.Balance{
			makeBalance(organizationID, ledgerID, aliceAlias,
				decimal.RequireFromString("1000"), decimal.Zero, 1),
			makeBalance(organizationID, ledgerID, bobAlias,
				decimal.RequireFromString("800"), decimal.Zero, 2),
		},
	}

	record := mustEnvelopeFromPayloads(t, organizationID, ledgerID,
		[]transaction.TransactionProcessingPayload{payload})

	recovered, err := extractRecoveredBalances(record, organizationID, ledgerID, aliases, nil)
	require.NoError(t, err)

	require.Len(t, recovered, 1, "only alice should be recovered")
	_, hasBob := recovered["@bob#default"]
	assert.False(t, hasBob, "bob must not be recovered when not in alias set")

	alice := recovered["@alice#default"]
	require.NotNil(t, alice)
	assert.True(t, decimal.RequireFromString("950").Equal(alice.Available))
}

// TestExtractRecoveredBalances_MalformedInnerPayload verifies that when the
// outer queue envelope is valid msgpack but the inner QueueData.Value is
// malformed, the function returns a descriptive error.
func TestExtractRecoveredBalances_MalformedInnerPayload(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Build a valid envelope with garbage inner payload.
	envelope := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData: []mmodel.QueueData{
			{ID: uuid.New(), Value: []byte("this-is-not-valid-msgpack")},
		},
	}

	envelopeBytes, err := msgpack.Marshal(envelope)
	require.NoError(t, err)

	aliases := map[string]struct{}{"@alice#default": {}}

	_, err = extractRecoveredBalances(envelopeBytes, organizationID, ledgerID, aliases, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to unmarshal queue payload")
}

// TestExtractRecoveredBalances_PendingTransactionOnHold verifies the ON_HOLD
// operation path: a PENDING transaction should decrease Available and increase
// OnHold by the operation value.
func TestExtractRecoveredBalances_PendingTransactionOnHold(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	queueAlias := "0#@alice#default"
	aliases := map[string]struct{}{"@alice#default": {}}

	payload := transaction.TransactionProcessingPayload{
		Validate: &pkgTransaction.Responses{
			From: map[string]pkgTransaction.Amount{
				queueAlias: {
					Asset:           "USD",
					Value:           decimal.RequireFromString("300"),
					Operation:       constant.ONHOLD,
					TransactionType: constant.PENDING,
				},
			},
			To: map[string]pkgTransaction.Amount{},
		},
		Balances: []*mmodel.Balance{
			makeBalance(organizationID, ledgerID, queueAlias,
				decimal.RequireFromString("1000"), decimal.RequireFromString("200"), 4),
		},
	}

	record := mustEnvelopeFromPayloads(t, organizationID, ledgerID,
		[]transaction.TransactionProcessingPayload{payload})

	recovered, err := extractRecoveredBalances(record, organizationID, ledgerID, aliases, nil)
	require.NoError(t, err)

	balance, ok := recovered["@alice#default"]
	require.True(t, ok)
	require.NotNil(t, balance)

	// ON_HOLD+PENDING: available = 1000 - 300 = 700, onHold = 200 + 300 = 500
	assert.True(t, decimal.RequireFromString("700").Equal(balance.Available),
		"expected available=700, got %s", balance.Available)
	assert.True(t, decimal.RequireFromString("500").Equal(balance.OnHold),
		"expected onHold=500, got %s", balance.OnHold)
	assert.Equal(t, int64(5), balance.Version)
}

// TestReplayPartitionRange_NoOpWhenStartEqualsEnd directly verifies the
// short-circuit guard: when startOffset >= endOffset, the function must return
// an empty map immediately without attempting any Kafka connection.
func TestReplayPartitionRange_NoOpWhenStartEqualsEnd(t *testing.T) {
	t.Parallel()

	r := &FranzStaleBalanceRecoverer{
		brokers: []string{"broker-that-does-not-exist:9092"},
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	aliases := map[string]struct{}{"@alice#default": {}}

	tt := []struct {
		name        string
		startOffset int64
		endOffset   int64
	}{
		{name: "start equals end", startOffset: 42, endOffset: 42},
		{name: "start exceeds end", startOffset: 100, endOffset: 50},
		{name: "both zero", startOffset: 0, endOffset: 0},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := r.replayPartitionRange(
				context.Background(),
				"ledger.balance.operations",
				0,
				tc.startOffset,
				tc.endOffset,
				orgID,
				ledgerID,
				aliases,
			)
			require.NoError(t, err)
			assert.Empty(t, result)
		})
	}
}

// TestRecoverLaggedAliases_NilAndEmptyGuards exercises every guard clause in
// RecoverLaggedAliases to confirm that nil/empty inputs yield empty results
// without error and without attempting any Kafka interaction.
func TestRecoverLaggedAliases_NilAndEmptyGuards(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	topic := "ledger.balance.operations"
	nonEmptyPartitions := map[int32][]string{0: {"@alice#default"}}

	tt := []struct {
		name       string
		recoverer  *FranzStaleBalanceRecoverer
		topic      string
		partitions map[int32][]string
	}{
		{
			name:       "nil recoverer",
			recoverer:  nil,
			topic:      topic,
			partitions: nonEmptyPartitions,
		},
		{
			name:       "nil adminClient",
			recoverer:  &FranzStaleBalanceRecoverer{adminClient: nil},
			topic:      topic,
			partitions: nonEmptyPartitions,
		},
		{
			name:       "empty topic",
			recoverer:  &FranzStaleBalanceRecoverer{},
			topic:      "",
			partitions: nonEmptyPartitions,
		},
		{
			name:       "nil partition map",
			recoverer:  &FranzStaleBalanceRecoverer{},
			topic:      topic,
			partitions: nil,
		},
		{
			name:       "empty partition map",
			recoverer:  &FranzStaleBalanceRecoverer{},
			topic:      topic,
			partitions: map[int32][]string{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := tc.recoverer.RecoverLaggedAliases(
				context.Background(),
				tc.topic,
				orgID,
				ledgerID,
				tc.partitions,
			)
			require.NoError(t, err)
			assert.Empty(t, result)
		})
	}
}
