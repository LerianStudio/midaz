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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

// Limits bound one execution. Production uses the engine-owned hard limits;
// tests may supply smaller limits to exercise boundary behavior.
type Limits struct {
	MaxTransactions        int
	MaxPostings            int
	MaxBalances            int
	MaxCompletionPlanBytes int
	MaxRequestBytes        int
	MaxPreparedBytes       int
}

const (
	maxTransactionsPerExecution = 1
	maxPostingsPerExecution     = 10_000
	maxBalancesPerExecution     = 20_000
	maxCompletionPlanBytes      = 32 * 1024 * 1024
	maxRequestBytes             = 64 * 1024 * 1024
	maxPreparedBytes            = 64 * 1024 * 1024
)

func hardLimits() Limits {
	return Limits{
		MaxTransactions:        maxTransactionsPerExecution,
		MaxPostings:            maxPostingsPerExecution,
		MaxBalances:            maxBalancesPerExecution,
		MaxCompletionPlanBytes: maxCompletionPlanBytes,
		MaxRequestBytes:        maxRequestBytes,
		MaxPreparedBytes:       maxPreparedBytes,
	}
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
	ID                  string                   `json:"id"`
	GuardField          string                   `json:"guardField"`
	ExpectedGuard       string                   `json:"expectedGuard"`
	NextGuard           string                   `json:"nextGuard"`
	RecoveryField       string                   `json:"recoveryField"`
	CompletionPlan      string                   `json:"recoveryPayload"`
	BalanceRequirements []wireBalanceRequirement `json:"balanceRequirements"`
	Postings            []wirePosting            `json:"postings"`
}

type wireBalanceRequirement struct {
	BalanceRef     string                       `json:"balanceRef"`
	AssetCode      string                       `json:"assetCode"`
	Permission     accounting.BalancePermission `json:"permission"`
	ForbidExternal bool                         `json:"forbidExternal"`
}

type wirePosting struct {
	Ref             string                 `json:"ref"`
	BalanceRef      string                 `json:"balanceRef"`
	Type            accounting.PostingType `json:"type"`
	Amount          string                 `json:"amount"`
	DrawPolicy      accounting.DrawPolicy  `json:"drawPolicy"`
	OverdraftAmount string                 `json:"overdraftAmount"`
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

	keys, err := prepareKeys(input.Execution.Balances, resolved, limits.MaxRequestBytes)
	if err != nil {
		return nil, err
	}

	wireBalances, balances, err := prepareBalances(ctx, input.Execution, limits)
	if err != nil {
		return nil, err
	}

	transactions, err := prepareTransactions(ctx, input.Execution, limits, guards, recovery, balances)
	if err != nil {
		return nil, err
	}

	request := input.Execution

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
	if limits.MaxTransactions <= 0 || limits.MaxPostings <= 0 || limits.MaxBalances <= 0 || limits.MaxCompletionPlanBytes <= 0 || limits.MaxRequestBytes <= 0 || limits.MaxPreparedBytes <= 0 {
		return fmt.Errorf("accounting execution limits must be positive")
	}

	request := input.Execution
	if request.OrganizationID == uuid.Nil || request.LedgerID == uuid.Nil || request.ExecutionID == uuid.Nil {
		return fmt.Errorf("accounting execution requires nonzero scope and execution UUIDs")
	}

	if strings.TrimSpace(input.IntentFingerprint) == "" {
		return fmt.Errorf("accounting execution requires an intent fingerprint")
	}

	if len(request.Transactions) == 0 || len(request.Transactions) > limits.MaxTransactions || len(request.Balances) > limits.MaxBalances {
		return fmt.Errorf("accounting execution exceeds transaction or balance limits")
	}

	if len(input.Guards) != len(request.Transactions) || len(input.CompletionPlans) != len(request.Transactions) {
		return fmt.Errorf("accounting execution requires one guard and completion plan per transaction")
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

	completionPlans := make(map[uuid.UUID]json.RawMessage, len(input.CompletionPlans))

	completionPlanBytes := 0
	for _, intent := range input.CompletionPlans {
		if len(intent.Payload) > limits.MaxCompletionPlanBytes-completionPlanBytes {
			return nil, nil, fmt.Errorf("accounting completion plan exceeds byte limit")
		}

		payload := strings.TrimSpace(string(intent.Payload))
		if intent.TransactionID == uuid.Nil || len(payload) == 0 || payload[0] != '{' || !json.Valid(intent.Payload) {
			return nil, nil, fmt.Errorf("invalid accounting completion plan")
		}

		if _, duplicate := completionPlans[intent.TransactionID]; duplicate {
			return nil, nil, fmt.Errorf("duplicate accounting completion plan")
		}

		completionPlanBytes += len(intent.Payload)
		completionPlans[intent.TransactionID] = intent.Payload
	}

	return guards, completionPlans, nil
}

func prepareBalances(ctx context.Context, request accounting.Execution, limits Limits) ([]wireBalance, map[string]accounting.BalanceSnapshot, error) {
	prepared := make([]wireBalance, 0, len(request.Balances))
	balances := make(map[string]accounting.BalanceSnapshot, len(request.Balances))
	identities := make(map[uuid.UUID]bool, len(request.Balances))
	accounts := make(map[uuid.UUID]accounting.BalanceSnapshot, len(request.Balances))

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

func validSnapshotIdentity(balance accounting.BalanceSnapshot) bool {
	return balance.ID != uuid.Nil && balance.AccountID != uuid.Nil && balance.Alias != "" && balance.Key != "" && balance.AssetCode != "" && balance.AccountType != "" && balance.BalanceRef == balance.Alias+"#"+balance.Key && validLogicalReference(balance.BalanceRef)
}

func prepareTransactions(ctx context.Context, request accounting.Execution, limits Limits, guards map[uuid.UUID]command.ExecutionGuard, completionPlans map[uuid.UUID]json.RawMessage, balances map[string]accounting.BalanceSnapshot) ([]wireTransaction, error) {
	preparedTransactions := make([]wireTransaction, 0, len(request.Transactions))
	transactionIDs := make(map[uuid.UUID]bool, len(request.Transactions))
	postingCount := 0

	for _, transaction := range request.Transactions {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("prepare accounting transactions: %w", err)
		}

		guard, hasGuard := guards[transaction.ID]

		completionPlan, hasCompletionPlan := completionPlans[transaction.ID]
		if transaction.ID == uuid.Nil || transactionIDs[transaction.ID] || !hasGuard || !hasCompletionPlan {
			return nil, fmt.Errorf("invalid accounting transaction correlation")
		}

		transactionIDs[transaction.ID] = true
		if len(transaction.Postings) == 0 || len(transaction.Postings) > limits.MaxPostings-postingCount {
			return nil, fmt.Errorf("accounting execution exceeds posting limit or has an empty transaction")
		}

		postingCount += len(transaction.Postings)
		prepared := wireTransaction{
			ID: transaction.ID.String(), GuardField: transaction.ID.String(), ExpectedGuard: guard.ExpectedToken, NextGuard: guard.NextToken,
			RecoveryField: transaction.ID.String() + ":" + request.ExecutionID.String(), CompletionPlan: string(completionPlan),
			BalanceRequirements: make([]wireBalanceRequirement, 0, len(transaction.BalanceRequirements)),
			Postings:            make([]wirePosting, 0, len(transaction.Postings)),
		}

		for _, requirement := range transaction.BalanceRequirements {
			if _, exists := balances[requirement.BalanceRef]; !exists || strings.TrimSpace(requirement.AssetCode) == "" ||
				(requirement.Permission != accounting.BalancePermissionSend && requirement.Permission != accounting.BalancePermissionReceive) {
				return nil, fmt.Errorf("invalid accounting balance requirement")
			}

			prepared.BalanceRequirements = append(prepared.BalanceRequirements, wireBalanceRequirement(requirement))
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

func preparePostings(postings []accounting.Posting, balances map[string]accounting.BalanceSnapshot, maxBytes int) ([]wirePosting, error) {
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

func prepareKeys(balances []accounting.BalanceSnapshot, resolved resolvedExecutionKeys, maxBytes int) ([]string, error) {
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

func prepareSnapshot(balance accounting.BalanceSnapshot, maxBytes int) (wireBalanceSnapshot, error) {
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

func validPostingType(posting accounting.PostingType) bool {
	switch posting {
	case accounting.PostingDebit, accounting.PostingCredit, accounting.PostingReserve, accounting.PostingUnreserve, accounting.PostingHold, accounting.PostingRelease:
		return true
	default:
		return false
	}
}

func validDrawPolicy(policy accounting.DrawPolicy) bool {
	return policy == accounting.DrawForbidden || policy == accounting.DrawAllowed || policy == accounting.DrawRouteDenied
}
