// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

const economicEffectDigestVersion = 1
const economicEffectDigestDomain = "midaz:transaction-economic-effect:v1\x00"
const annotationEffectDigestDomain = "midaz:transaction-annotation-effect:v1\x00"

type canonicalRedisOperationEffect struct {
	ID                    string `json:"id"`
	TransactionID         string `json:"transactionId"`
	BalanceID             string `json:"balanceId"`
	BalanceKey            string `json:"balanceKey"`
	AccountID             string `json:"accountId"`
	OrganizationID        string `json:"organizationId"`
	LedgerID              string `json:"ledgerId"`
	Type                  string `json:"type"`
	Direction             string `json:"direction"`
	AssetCode             string `json:"assetCode"`
	BalanceAffected       bool   `json:"balanceAffected"`
	AmountValue           string `json:"amountValue"`
	BalanceAvailable      string `json:"balanceAvailable"`
	BalanceOnHold         string `json:"balanceOnHold"`
	BalanceVersion        int64  `json:"balanceVersion"`
	BalanceAfterAvailable string `json:"balanceAfterAvailable"`
	BalanceAfterOnHold    string `json:"balanceAfterOnHold"`
	BalanceAfterVersion   int64  `json:"balanceAfterVersion"`
	OverdraftUsedBefore   string `json:"overdraftUsedBefore"`
	OverdraftUsedAfter    string `json:"overdraftUsedAfter"`
}

type canonicalRedisBalanceEffect struct {
	ID                    string `json:"id"`
	Alias                 string `json:"alias"`
	Key                   string `json:"key"`
	AccountID             string `json:"accountId"`
	AssetCode             string `json:"assetCode"`
	Available             string `json:"available"`
	OnHold                string `json:"onHold"`
	Version               int64  `json:"version"`
	AccountType           string `json:"accountType"`
	AllowSending          int    `json:"allowSending"`
	AllowReceiving        int    `json:"allowReceiving"`
	Direction             string `json:"direction"`
	OverdraftUsed         string `json:"overdraftUsed"`
	AllowOverdraft        int    `json:"allowOverdraft"`
	OverdraftLimitEnabled int    `json:"overdraftLimitEnabled"`
	OverdraftLimit        string `json:"overdraftLimit"`
	BalanceScope          string `json:"balanceScope"`
}

// RedisEconomicEffectDigest returns an order-independent, duplicate-preserving
// digest of the complete terminal money effect. The transaction amount and
// asset come from the immutable input, never from a persistence candidate.
// Decimal values are normalized with shopspring/decimal before hashing, so
// equivalent spellings share one digest without ever passing through float64
// or Lua numbers. Attempt owner, terminal outcome, and dataset generation
// remain separate envelope fields and are compared alongside this digest by
// the same Redis finalization command.
func RedisEconomicEffectDigest(
	transactionAmount, transactionAssetCode string,
	operations []OperationRedis,
	balances []BalanceRedis,
) (string, error) {
	canonicalAmount, err := canonicalTransactionIdentity(transactionAmount, transactionAssetCode)
	if err != nil {
		return "", err
	}
	if len(operations) == 0 || len(balances) == 0 {
		return "", fmt.Errorf("complete economic operations and balances are required")
	}

	canonicalOperations, err := canonicalRedisOperationEffects(operations)
	if err != nil {
		return "", err
	}

	if !RedisBalanceSetEconomicComplete(balances) {
		return "", fmt.Errorf("economic balance snapshot is incomplete")
	}
	canonicalBalances := make([]json.RawMessage, 0, len(balances))
	for _, balance := range balances {
		overdraftUsed, err := canonicalEconomicDecimal(balance.OverdraftUsed)
		if err != nil {
			return "", fmt.Errorf("canonicalize balance %q overdraft used: %w", balance.ID, err)
		}
		overdraftLimit, err := canonicalEconomicDecimal(balance.OverdraftLimit)
		if err != nil {
			return "", fmt.Errorf("canonicalize balance %q overdraft limit: %w", balance.ID, err)
		}
		encoded, err := json.Marshal(canonicalRedisBalanceEffect{
			ID: balance.ID, Alias: balance.Alias, Key: balance.Key, AccountID: balance.AccountID,
			AssetCode: balance.AssetCode, Available: balance.Available.String(), OnHold: balance.OnHold.String(),
			Version: balance.Version, AccountType: balance.AccountType, AllowSending: balance.AllowSending,
			AllowReceiving: balance.AllowReceiving, Direction: balance.Direction, OverdraftUsed: overdraftUsed,
			AllowOverdraft: balance.AllowOverdraft, OverdraftLimitEnabled: balance.OverdraftLimitEnabled,
			OverdraftLimit: overdraftLimit, BalanceScope: balance.BalanceScope,
		})
		if err != nil {
			return "", fmt.Errorf("encode canonical economic balance: %w", err)
		}
		canonicalBalances = append(canonicalBalances, encoded)
	}

	sort.Slice(canonicalOperations, func(i, j int) bool {
		return bytes.Compare(canonicalOperations[i], canonicalOperations[j]) < 0
	})
	sort.Slice(canonicalBalances, func(i, j int) bool {
		return bytes.Compare(canonicalBalances[i], canonicalBalances[j]) < 0
	})
	canonical, err := json.Marshal(struct {
		Version              int               `json:"version"`
		TransactionAmount    string            `json:"transactionAmount"`
		TransactionAssetCode string            `json:"transactionAssetCode"`
		Operations           []json.RawMessage `json:"operations"`
		Balances             []json.RawMessage `json:"balances"`
	}{
		Version: economicEffectDigestVersion, TransactionAmount: canonicalAmount,
		TransactionAssetCode: transactionAssetCode, Operations: canonicalOperations, Balances: canonicalBalances,
	})
	if err != nil {
		return "", fmt.Errorf("encode canonical economic effect: %w", err)
	}
	return digestCanonicalEffect(economicEffectDigestDomain, canonical), nil
}

// RedisAnnotationEffectDigest binds the exact, duplicate-preserving annotation
// rows chosen by CAS. It has a separate domain from a money effect and carries
// no balance set; callers must first prove ANNOTATION_ONLY semantics.
func RedisAnnotationEffectDigest(
	transactionAmount, transactionAssetCode string,
	operations []OperationRedis,
) (string, error) {
	canonicalAmount, err := canonicalTransactionIdentity(transactionAmount, transactionAssetCode)
	if err != nil {
		return "", err
	}
	canonicalOperations, err := canonicalRedisOperationEffects(operations)
	if err != nil {
		return "", err
	}
	canonical, err := json.Marshal(struct {
		Version              int               `json:"version"`
		TransactionAmount    string            `json:"transactionAmount"`
		TransactionAssetCode string            `json:"transactionAssetCode"`
		Operations           []json.RawMessage `json:"operations"`
	}{
		Version: economicEffectDigestVersion, TransactionAmount: canonicalAmount,
		TransactionAssetCode: transactionAssetCode, Operations: canonicalOperations,
	})
	if err != nil {
		return "", fmt.Errorf("encode canonical annotation effect: %w", err)
	}

	return digestCanonicalEffect(annotationEffectDigestDomain, canonical), nil
}

func canonicalTransactionIdentity(transactionAmount, transactionAssetCode string) (string, error) {
	if transactionAssetCode == "" {
		return "", fmt.Errorf("transaction asset code is required")
	}
	canonicalAmount, err := canonicalEconomicDecimal(transactionAmount)
	if err != nil {
		return "", fmt.Errorf("canonicalize transaction amount: %w", err)
	}
	parsed, err := decimal.NewFromString(canonicalAmount)
	if err != nil || !parsed.IsPositive() {
		return "", fmt.Errorf("transaction amount must be positive")
	}

	return canonicalAmount, nil
}

func canonicalRedisOperationEffects(operations []OperationRedis) ([]json.RawMessage, error) {
	if len(operations) == 0 {
		return nil, fmt.Errorf("complete transaction operations are required")
	}
	canonicalOperations := make([]json.RawMessage, 0, len(operations))
	for _, operation := range operations {
		if !RedisOperationEconomicComplete(operation) {
			return nil, fmt.Errorf("economic operation %q is incomplete", operation.ID)
		}
		beforeOverdraft, err := canonicalEconomicDecimal(operation.Snapshot.OverdraftUsedBefore)
		if err != nil {
			return nil, fmt.Errorf("canonicalize operation %q before overdraft: %w", operation.ID, err)
		}
		afterOverdraft, err := canonicalEconomicDecimal(operation.Snapshot.OverdraftUsedAfter)
		if err != nil {
			return nil, fmt.Errorf("canonicalize operation %q after overdraft: %w", operation.ID, err)
		}
		encoded, err := json.Marshal(canonicalRedisOperationEffect{
			ID: operation.ID, TransactionID: operation.TransactionID, BalanceID: operation.BalanceID,
			BalanceKey: operation.BalanceKey, AccountID: operation.AccountID,
			OrganizationID: operation.OrganizationID, LedgerID: operation.LedgerID,
			Type: operation.Type, Direction: operation.Direction, AssetCode: operation.AssetCode,
			BalanceAffected: operation.BalanceAffected, AmountValue: operation.AmountValue.String(),
			BalanceAvailable: operation.BalanceAvailable.String(), BalanceOnHold: operation.BalanceOnHold.String(),
			BalanceVersion: operation.BalanceVersion, BalanceAfterAvailable: operation.BalanceAfterAvailable.String(),
			BalanceAfterOnHold: operation.BalanceAfterOnHold.String(), BalanceAfterVersion: operation.BalanceAfterVersion,
			OverdraftUsedBefore: beforeOverdraft, OverdraftUsedAfter: afterOverdraft,
		})
		if err != nil {
			return nil, fmt.Errorf("encode canonical economic operation: %w", err)
		}
		canonicalOperations = append(canonicalOperations, encoded)
	}
	sort.Slice(canonicalOperations, func(i, j int) bool {
		return bytes.Compare(canonicalOperations[i], canonicalOperations[j]) < 0
	})

	return canonicalOperations, nil
}

func digestCanonicalEffect(domain string, canonical []byte) string {
	digestInput := make([]byte, 0, len(domain)+len(canonical))
	digestInput = append(digestInput, domain...)
	digestInput = append(digestInput, canonical...)
	sum := sha256.Sum256(digestInput)

	return hex.EncodeToString(sum[:])
}

func canonicalEconomicDecimal(value string) (string, error) {
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return "", fmt.Errorf("invalid decimal %q: %w", value, err)
	}

	return parsed.String(), nil
}
