// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

// Limits bounds one execution. Callers must provide measured, positive limits.
type Limits struct {
	MaxTransactions  int
	MaxPostings      int
	MaxBalances      int
	MaxRecoveryBytes int
	MaxRequestBytes  int
	MaxPreparedBytes int
}

// resolvedExecutionKeys is supplied by the authenticated adapter boundary,
// never by request JSON. Hash fields remain in the payload, not in KEYS.
type resolvedExecutionKeys struct {
	TenantID   string
	Schedule   string
	Recovery   string
	Receipts   string
	Guards     string
	Protection string
	Balances   map[string]resolvedBalanceKeys
}

type resolvedBalanceKeys struct {
	Balance string
	Deleted string
}

type preparedExecution struct {
	Keys    []string
	Payload []byte
}

type wireRequest struct {
	ProtocolVersion    int               `json:"protocolVersion"`
	TenantID           string            `json:"tenantId"`
	OrganizationID     string            `json:"organizationId"`
	LedgerID           string            `json:"ledgerId"`
	ExecutionID        string            `json:"executionId"`
	IntentFingerprint  string            `json:"intentFingerprint"`
	ScheduleKeyIndex   int               `json:"scheduleKeyIndex"`
	RecoveryKeyIndex   int               `json:"recoveryKeyIndex"`
	ReceiptKeyIndex    int               `json:"receiptKeyIndex"`
	GuardKeyIndex      int               `json:"guardKeyIndex"`
	ProtectionKeyIndex int               `json:"protectionKeyIndex"`
	ReceiptField       string            `json:"receiptField"`
	RetentionSeconds   int64             `json:"retentionSeconds"`
	Transactions       []wireTransaction `json:"transactions"`
	Balances           []wireBalance     `json:"balances"`
}

type wireTransaction struct {
	ID              string        `json:"id"`
	GuardField      string        `json:"guardField"`
	ExpectedGuard   string        `json:"expectedGuard"`
	NextGuard       string        `json:"nextGuard"`
	RecoveryField   string        `json:"recoveryField"`
	RecoveryPayload string        `json:"recoveryPayload"`
	Postings        []wirePosting `json:"postings"`
}

type wirePosting struct {
	Ref             string             `json:"ref"`
	BalanceRef      string             `json:"balanceRef"`
	Type            engine.PostingType `json:"type"`
	Amount          string             `json:"amount"`
	DrawPolicy      engine.DrawPolicy  `json:"drawPolicy"`
	OverdraftAmount string             `json:"overdraftAmount"`
}

type wireBalance struct {
	BalanceRef     string              `json:"balanceRef"`
	KeyIndex       int                 `json:"keyIndex"`
	DeleteKeyIndex int                 `json:"deleteKeyIndex"`
	Snapshot       wireBalanceSnapshot `json:"snapshot"`
}

type wireBalanceSnapshot struct {
	ID                    string `json:"id"`
	AccountID             string `json:"accountId"`
	AccountType           string `json:"accountType"`
	AssetCode             string `json:"assetCode"`
	Alias                 string `json:"alias"`
	Key                   string `json:"key"`
	Direction             string `json:"direction"`
	BalanceScope          string `json:"balanceScope"`
	Available             string `json:"available"`
	OnHold                string `json:"onHold"`
	OverdraftUsed         string `json:"overdraftUsed"`
	OverdraftLimit        string `json:"overdraftLimit"`
	Version               string `json:"version"`
	AllowSending          bool   `json:"allowSending"`
	AllowReceiving        bool   `json:"allowReceiving"`
	AllowOverdraft        bool   `json:"allowOverdraft"`
	OverdraftLimitEnabled bool   `json:"overdraftLimitEnabled"`
}

func prepareExecution(ctx context.Context, input command.EngineExecution, limits Limits, resolved resolvedExecutionKeys) (*preparedExecution, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("prepare accounting execution: %w", err)
	}

	if err := validateExecutionEnvelope(input, limits); err != nil {
		return nil, err
	}

	guards, recovery, err := prepareSidecars(input, limits)
	if err != nil {
		return nil, err
	}

	keys, err := prepareKeys(input.Request.Balances, resolved, limits.MaxRequestBytes)
	if err != nil {
		return nil, err
	}

	wireBalances, balances, err := prepareBalances(ctx, input.Request, limits)
	if err != nil {
		return nil, err
	}

	transactions, err := prepareTransactions(ctx, input.Request, limits, guards, recovery, balances)
	if err != nil {
		return nil, err
	}

	request := input.Request

	retentionSeconds, err := effectiveRetentionSeconds(input.RetentionSeconds)
	if err != nil {
		return nil, err
	}

	wire := wireRequest{
		ProtocolVersion: 1, TenantID: resolved.TenantID,
		OrganizationID: request.OrganizationID.String(), LedgerID: request.LedgerID.String(), ExecutionID: request.ExecutionID.String(),
		IntentFingerprint: input.IntentFingerprint, ScheduleKeyIndex: 1, RecoveryKeyIndex: 2, ReceiptKeyIndex: 3, GuardKeyIndex: 4, ProtectionKeyIndex: 5,
		ReceiptField: request.ExecutionID.String(), RetentionSeconds: retentionSeconds, Transactions: transactions, Balances: wireBalances,
	}

	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("encode accounting execution: %w", err)
	}

	if len(encoded) > limits.MaxRequestBytes {
		return nil, fmt.Errorf("accounting execution exceeds request byte limit")
	}

	return &preparedExecution{Keys: keys, Payload: encoded}, nil
}

const (
	defaultRetentionSeconds int64 = 300
	maximumRetentionSeconds int64 = 604800
)

func effectiveRetentionSeconds(requested int64) (int64, error) {
	if requested < 0 {
		return 0, fmt.Errorf("accounting execution retention must not be negative")
	}

	if requested == 0 {
		return defaultRetentionSeconds, nil
	}

	if requested > maximumRetentionSeconds {
		return 0, fmt.Errorf("accounting execution retention exceeds maximum")
	}

	return requested, nil
}

func validateExecutionEnvelope(input command.EngineExecution, limits Limits) error {
	if limits.MaxTransactions <= 0 || limits.MaxPostings <= 0 || limits.MaxBalances <= 0 || limits.MaxRecoveryBytes <= 0 || limits.MaxRequestBytes <= 0 || limits.MaxPreparedBytes <= 0 {
		return fmt.Errorf("accounting execution limits must be positive")
	}

	request := input.Request
	if request.OrganizationID == uuid.Nil || request.LedgerID == uuid.Nil || request.ExecutionID == uuid.Nil {
		return fmt.Errorf("accounting execution requires nonzero scope and execution UUIDs")
	}

	if strings.TrimSpace(input.IntentFingerprint) == "" {
		return fmt.Errorf("accounting execution requires an intent fingerprint")
	}

	if len(request.Transactions) == 0 || len(request.Transactions) > limits.MaxTransactions || len(request.Balances) > limits.MaxBalances {
		return fmt.Errorf("accounting execution exceeds transaction or balance limits")
	}

	if len(input.Guards) != len(request.Transactions) || len(input.Recovery) != len(request.Transactions) {
		return fmt.Errorf("accounting execution requires one guard and recovery payload per transaction")
	}

	return nil
}

func prepareSidecars(input command.EngineExecution, limits Limits) (map[uuid.UUID]command.ExecutionGuard, map[uuid.UUID]json.RawMessage, error) {
	guards := make(map[uuid.UUID]command.ExecutionGuard, len(input.Guards))
	for _, guard := range input.Guards {
		if guard.TransactionID == uuid.Nil || guard.NextToken == "" || guard.NextToken == guard.ExpectedToken {
			return nil, nil, fmt.Errorf("invalid accounting execution guard")
		}

		if _, duplicate := guards[guard.TransactionID]; duplicate {
			return nil, nil, fmt.Errorf("duplicate accounting execution guard")
		}

		guards[guard.TransactionID] = guard
	}

	recovery := make(map[uuid.UUID]json.RawMessage, len(input.Recovery))

	recoveryBytes := 0
	for _, intent := range input.Recovery {
		if len(intent.Payload) > limits.MaxRecoveryBytes-recoveryBytes {
			return nil, nil, fmt.Errorf("accounting recovery payload exceeds byte limit")
		}

		payload := strings.TrimSpace(string(intent.Payload))
		if intent.TransactionID == uuid.Nil || len(payload) == 0 || payload[0] != '{' || !json.Valid(intent.Payload) {
			return nil, nil, fmt.Errorf("invalid accounting recovery object")
		}

		if _, duplicate := recovery[intent.TransactionID]; duplicate {
			return nil, nil, fmt.Errorf("duplicate accounting recovery payload")
		}

		recoveryBytes += len(intent.Payload)
		recovery[intent.TransactionID] = intent.Payload
	}

	return guards, recovery, nil
}

func prepareBalances(ctx context.Context, request engine.Request, limits Limits) ([]wireBalance, map[string]engine.BalanceSnapshot, error) {
	prepared := make([]wireBalance, 0, len(request.Balances))
	balances := make(map[string]engine.BalanceSnapshot, len(request.Balances))
	identities := make(map[uuid.UUID]bool, len(request.Balances))
	accounts := make(map[uuid.UUID]engine.BalanceSnapshot, len(request.Balances))

	aliases := make(map[string]uuid.UUID, len(request.Balances))
	for i, balance := range request.Balances {
		if err := ctx.Err(); err != nil {
			return nil, nil, fmt.Errorf("prepare accounting snapshots: %w", err)
		}

		if !validSnapshotIdentity(balance) {
			return nil, nil, fmt.Errorf("invalid accounting balance identity")
		}

		if _, duplicate := balances[balance.BalanceRef]; duplicate || identities[balance.ID] {
			return nil, nil, fmt.Errorf("duplicate accounting balance identity")
		}

		if account, exists := accounts[balance.AccountID]; exists && (account.Alias != balance.Alias || account.AssetCode != balance.AssetCode || account.AccountType != balance.AccountType) {
			return nil, nil, fmt.Errorf("inconsistent accounting account identity")
		}

		if id, exists := aliases[balance.Alias]; exists && id != balance.AccountID {
			return nil, nil, fmt.Errorf("accounting alias identifies multiple accounts")
		}

		snapshot, err := prepareSnapshot(balance, limits.MaxRequestBytes)
		if err != nil {
			return nil, nil, err
		}

		balances[balance.BalanceRef], identities[balance.ID], accounts[balance.AccountID], aliases[balance.Alias] = balance, true, balance, balance.AccountID
		prepared = append(prepared, wireBalance{BalanceRef: balance.BalanceRef, KeyIndex: 6 + 2*i, DeleteKeyIndex: 7 + 2*i, Snapshot: snapshot})
	}

	return prepared, balances, nil
}

func validSnapshotIdentity(balance engine.BalanceSnapshot) bool {
	return balance.ID != uuid.Nil && balance.AccountID != uuid.Nil && balance.Alias != "" && balance.Key != "" && balance.AssetCode != "" && balance.AccountType != "" && balance.BalanceRef == balance.Alias+"#"+balance.Key && validLogicalReference(balance.BalanceRef)
}

func prepareTransactions(ctx context.Context, request engine.Request, limits Limits, guards map[uuid.UUID]command.ExecutionGuard, recovery map[uuid.UUID]json.RawMessage, balances map[string]engine.BalanceSnapshot) ([]wireTransaction, error) {
	preparedTransactions := make([]wireTransaction, 0, len(request.Transactions))
	transactionIDs := make(map[uuid.UUID]bool, len(request.Transactions))
	postingCount := 0

	for _, transaction := range request.Transactions {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("prepare accounting transactions: %w", err)
		}

		guard, hasGuard := guards[transaction.ID]

		payload, hasRecovery := recovery[transaction.ID]
		if transaction.ID == uuid.Nil || transactionIDs[transaction.ID] || !hasGuard || !hasRecovery {
			return nil, fmt.Errorf("invalid accounting transaction correlation")
		}

		transactionIDs[transaction.ID] = true
		if len(transaction.Postings) == 0 || len(transaction.Postings) > limits.MaxPostings-postingCount {
			return nil, fmt.Errorf("accounting execution exceeds posting limit or has an empty transaction")
		}

		postingCount += len(transaction.Postings)
		prepared := wireTransaction{
			ID: transaction.ID.String(), GuardField: transaction.ID.String(), ExpectedGuard: guard.ExpectedToken, NextGuard: guard.NextToken,
			RecoveryField: transaction.ID.String() + ":" + request.ExecutionID.String(), RecoveryPayload: string(payload),
			Postings: make([]wirePosting, 0, len(transaction.Postings)),
		}

		postings, err := preparePostings(transaction.Postings, balances, limits.MaxRequestBytes)
		if err != nil {
			return nil, err
		}

		prepared.Postings = postings
		preparedTransactions = append(preparedTransactions, prepared)
	}

	return preparedTransactions, nil
}

func preparePostings(postings []engine.Posting, balances map[string]engine.BalanceSnapshot, maxBytes int) ([]wirePosting, error) {
	prepared := make([]wirePosting, 0, len(postings))

	refs := make(map[string]bool, len(postings))
	for _, posting := range postings {
		if strings.TrimSpace(posting.Ref) == "" || refs[posting.Ref] {
			return nil, fmt.Errorf("empty or duplicate accounting posting reference")
		}

		if _, exists := balances[posting.BalanceRef]; !exists {
			return nil, fmt.Errorf("accounting posting references an unknown balance")
		}

		if posting.Amount.Sign() <= 0 || posting.OverdraftAmount.Sign() < 0 || !validPostingType(posting.Type) || !validDrawPolicy(posting.DrawPolicy) {
			return nil, fmt.Errorf("invalid accounting posting")
		}

		amount, err := boundedDecimal(posting.Amount, maxBytes)
		if err != nil {
			return nil, err
		}

		overdraftAmount, err := boundedDecimal(posting.OverdraftAmount, maxBytes)
		if err != nil {
			return nil, err
		}

		refs[posting.Ref] = true
		prepared = append(prepared, wirePosting{Ref: posting.Ref, BalanceRef: posting.BalanceRef, Type: posting.Type, Amount: amount, DrawPolicy: posting.DrawPolicy, OverdraftAmount: overdraftAmount})
	}

	return prepared, nil
}

func prepareKeys(balances []engine.BalanceSnapshot, resolved resolvedExecutionKeys, maxBytes int) ([]string, error) {
	if len(resolved.Balances) != len(balances) {
		return nil, fmt.Errorf("resolved accounting key inventory does not match snapshots")
	}

	if resolved.Protection == "" {
		return nil, fmt.Errorf("missing resolved accounting protection key")
	}

	keys := []string{resolved.Schedule, resolved.Recovery, resolved.Receipts, resolved.Guards, resolved.Protection}
	for _, balance := range balances {
		pair, exists := resolved.Balances[balance.BalanceRef]
		if !exists || pair.Deleted != pair.Balance+cachepolicy.DeletionMarkerSuffix {
			return nil, fmt.Errorf("invalid resolved accounting balance keys")
		}

		keys = append(keys, pair.Balance, pair.Deleted)
	}

	seen := make(map[string]bool, len(keys))

	total := 0
	for _, key := range keys {
		if !strings.Contains(key, cachepolicy.HashTag) || strings.Count(key, "{") != 1 || strings.Count(key, "}") != 1 || seen[key] || len(key) > maxBytes-total {
			return nil, fmt.Errorf("invalid or oversized resolved accounting key")
		}

		seen[key], total = true, total+len(key)
	}

	return keys, nil
}

func prepareSnapshot(balance engine.BalanceSnapshot, maxBytes int) (wireBalanceSnapshot, error) {
	if (balance.Direction != "" && balance.Direction != "credit" && balance.Direction != "debit") || (balance.BalanceScope != "transactional" && balance.BalanceScope != "internal") || balance.Version < 0 || balance.OnHold.Sign() < 0 || balance.OverdraftUsed.Sign() < 0 || balance.OverdraftLimit.Sign() < 0 || (balance.Available.Sign() < 0 && balance.AccountType != "external") {
		return wireBalanceSnapshot{}, fmt.Errorf("invalid accounting balance state")
	}

	values := [4]string{}

	for i, value := range []decimal.Decimal{balance.Available, balance.OnHold, balance.OverdraftUsed, balance.OverdraftLimit} {
		encoded, err := boundedDecimal(value, maxBytes)
		if err != nil {
			return wireBalanceSnapshot{}, err
		}

		values[i] = encoded
	}

	return wireBalanceSnapshot{
		ID: balance.ID.String(), AccountID: balance.AccountID.String(), AccountType: balance.AccountType, AssetCode: balance.AssetCode,
		Alias: balance.Alias, Key: balance.Key, Direction: balance.Direction, BalanceScope: balance.BalanceScope,
		Available: values[0], OnHold: values[1], OverdraftUsed: values[2], OverdraftLimit: values[3], Version: strconv.FormatInt(balance.Version, 10),
		AllowSending: balance.AllowSending, AllowReceiving: balance.AllowReceiving, AllowOverdraft: balance.AllowOverdraft, OverdraftLimitEnabled: balance.OverdraftLimitEnabled,
	}, nil
}

func boundedDecimal(value decimal.Decimal, maxBytes int) (string, error) {
	if value.IsZero() {
		return "0", nil
	}

	digits := int64(len(strings.TrimPrefix(value.Coefficient().String(), "-")))
	exponent := int64(value.Exponent())

	length := digits + exponent
	if exponent < 0 {
		length = digits + 1
		if digits+exponent <= 0 {
			length = 2 - exponent
		}
	}

	if value.Sign() < 0 {
		length++
	}

	if length > int64(maxBytes) {
		return "", fmt.Errorf("accounting decimal exceeds request byte limit")
	}

	return value.String(), nil
}

func validLogicalReference(ref string) bool {
	return strings.Count(ref, "#") == 1 && !strings.ContainsAny(ref, "{}") && !strings.HasPrefix(ref, "balance:") && !strings.HasPrefix(ref, "tenant:") && strings.IndexFunc(ref, unicode.IsControl) == -1
}

func validPostingType(posting engine.PostingType) bool {
	switch posting {
	case engine.PostingDebit, engine.PostingCredit, engine.PostingReserve, engine.PostingUnreserve, engine.PostingHold, engine.PostingRelease:
		return true
	default:
		return false
	}
}

func validDrawPolicy(policy engine.DrawPolicy) bool {
	return policy == engine.DrawForbidden || policy == engine.DrawAllowed || policy == engine.DrawRouteDenied
}
