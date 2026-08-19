// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const ExpectedEconomicPlanVersion = 1

const (
	EconomicSideSource      = "SOURCE"
	EconomicSideDestination = "DESTINATION"
	EconomicSideUnspecified = "UNSPECIFIED"

	EconomicRolePrimary   = "PRIMARY"
	EconomicRoleFee       = "FEE"
	EconomicRoleCompanion = "COMPANION"
)

// ExpectedEconomicPlan is the immutable, versioned batch that the caller
// actually gives to the balance Lua. Unlike Validate's alias maps it preserves
// repeated aliases, fee legs, companion legs, and every resolved amount.
type ExpectedEconomicPlan struct {
	Version int                   `json:"version"`
	Digest  string                `json:"digest"`
	Legs    []ExpectedEconomicLeg `json:"legs"`
}

type ExpectedEconomicLeg struct {
	Identity              string `json:"identity"`
	Alias                 string `json:"alias"`
	BalanceID             string `json:"balance_id"`
	BalanceKey            string `json:"balance_key"`
	AccountID             string `json:"account_id"`
	InternalKey           string `json:"internal_key"`
	BalanceDirection      string `json:"balance_direction"`
	Operation             string `json:"operation"`
	Direction             string `json:"direction"`
	AssetCode             string `json:"asset_code"`
	Amount                string `json:"amount"`
	Role                  string `json:"role"`
	Side                  string `json:"side"`
	ExpectedOperationType string `json:"expected_operation_type"`
	PersistsOperation     bool   `json:"persists_operation"`
}

type economicPlanDigestPayload struct {
	Domain  string                `json:"domain"`
	Version int                   `json:"version"`
	Legs    []ExpectedEconomicLeg `json:"legs"`
}

//nolint:gocognit,gocyclo // Plan construction folds every operation effect variant; refactor candidate.
func BuildExpectedEconomicPlan(balanceOperations []BalanceOperation, transactionStatus string, pending bool, operationTypeOverride string) (*ExpectedEconomicPlan, error) {
	if len(balanceOperations) == 0 {
		return nil, fmt.Errorf("expected economic plan requires at least one balance leg")
	}

	if operationTypeOverride != "" && operationTypeOverride != constant.BLOCK && operationTypeOverride != constant.UNBLOCK {
		return nil, fmt.Errorf("unsupported expected operation type override %q", operationTypeOverride)
	}

	legs := make([]ExpectedEconomicLeg, 0, len(balanceOperations))
	for _, operation := range balanceOperations {
		if operation.Balance == nil || strings.TrimSpace(operation.Alias) == "" || strings.TrimSpace(operation.InternalKey) == "" {
			return nil, fmt.Errorf("expected economic plan leg requires balance, alias, and internal key")
		}

		amount := operation.Amount.Value
		if !amount.IsPositive() {
			return nil, fmt.Errorf("expected economic plan leg requires positive amount")
		}

		if operation.Balance.ID == "" || operation.Balance.Key == "" || operation.Balance.AccountID == "" ||
			operation.Balance.AssetCode == "" || operation.Amount.Operation == "" || operation.Amount.Direction == "" {
			return nil, fmt.Errorf("expected economic plan leg is incomplete")
		}

		role := operation.EconomicRole
		if role == "" {
			role = EconomicRolePrimary
		}

		if operation.Balance.Key == constant.OverdraftBalanceKey {
			role = EconomicRoleCompanion
		}

		if role != EconomicRolePrimary && role != EconomicRoleFee && role != EconomicRoleCompanion {
			return nil, fmt.Errorf("unsupported expected economic role %q", role)
		}

		side := operation.EconomicSide
		if side == "" {
			side = EconomicSideUnspecified
		}

		if side != EconomicSideSource && side != EconomicSideDestination && side != EconomicSideUnspecified {
			return nil, fmt.Errorf("unsupported expected economic side %q", side)
		}

		expectedOperationType := operation.Amount.Operation
		if role == EconomicRoleCompanion {
			expectedOperationType = constant.OVERDRAFT
		} else if operationTypeOverride != "" {
			expectedOperationType = operationTypeOverride
		}

		persistsOperation := true
		if pending && (transactionStatus == constant.PENDING || transactionStatus == constant.CANCELED) && side == EconomicSideDestination {
			persistsOperation = false
		}

		legs = append(legs, ExpectedEconomicLeg{
			Alias: operation.Alias, BalanceID: operation.Balance.ID, BalanceKey: operation.Balance.Key,
			AccountID: operation.Balance.AccountID, InternalKey: operation.InternalKey,
			BalanceDirection: operation.Balance.Direction, Operation: operation.Amount.Operation,
			Direction: operation.Amount.Direction, AssetCode: operation.Balance.AssetCode,
			Amount: amount.String(), Role: role, Side: side,
			ExpectedOperationType: expectedOperationType, PersistsOperation: persistsOperation,
		})
	}

	canonicalizeExpectedEconomicLegs(legs)

	occurrences := make(map[string]int, len(legs))
	for index := range legs {
		position := legs[index].Alias
		occurrence := occurrences[position]
		occurrences[position] = occurrence + 1
		legs[index].Identity = position + "/" + strconv.Itoa(occurrence)
	}

	canonicalizeExpectedEconomicLegs(legs)

	plan := &ExpectedEconomicPlan{Version: ExpectedEconomicPlanVersion, Legs: legs}

	digest, err := expectedEconomicPlanDigest(plan.Version, plan.Legs)
	if err != nil {
		return nil, err
	}

	plan.Digest = digest

	return plan, nil
}

//nolint:gocyclo // Validation checks every plan field invariant; refactor candidate.
func ValidateExpectedEconomicPlan(plan *ExpectedEconomicPlan) error {
	if plan == nil || plan.Version != ExpectedEconomicPlanVersion || len(plan.Legs) == 0 || plan.Digest == "" {
		return fmt.Errorf("complete expected economic plan version %d is required", ExpectedEconomicPlanVersion)
	}

	seen := make(map[string]struct{}, len(plan.Legs))

	canonical := append([]ExpectedEconomicLeg(nil), plan.Legs...)
	for index := range canonical {
		leg := &canonical[index]
		if leg.Identity == "" || leg.Alias == "" || leg.BalanceID == "" || leg.BalanceKey == "" ||
			leg.AccountID == "" || leg.InternalKey == "" || leg.Operation == "" || leg.Direction == "" ||
			leg.AssetCode == "" || leg.ExpectedOperationType == "" {
			return fmt.Errorf("expected economic plan leg is incomplete")
		}

		if _, duplicate := seen[leg.Identity]; duplicate {
			return fmt.Errorf("expected economic plan leg identity %q is duplicated", leg.Identity)
		}

		seen[leg.Identity] = struct{}{}

		amount, err := decimal.NewFromString(leg.Amount)
		if err != nil || !amount.IsPositive() {
			return fmt.Errorf("expected economic plan leg %q has invalid amount", leg.Identity)
		}

		leg.Amount = amount.String()
		if leg.Role != EconomicRolePrimary && leg.Role != EconomicRoleFee && leg.Role != EconomicRoleCompanion {
			return fmt.Errorf("expected economic plan leg %q has invalid role", leg.Identity)
		}

		if leg.Side != EconomicSideSource && leg.Side != EconomicSideDestination && leg.Side != EconomicSideUnspecified {
			return fmt.Errorf("expected economic plan leg %q has invalid side", leg.Identity)
		}
	}

	canonicalizeExpectedEconomicLegs(canonical)

	digest, err := expectedEconomicPlanDigest(plan.Version, canonical)
	if err != nil {
		return err
	}

	if digest != plan.Digest {
		return fmt.Errorf("expected economic plan digest mismatch")
	}

	return nil
}

func expectedEconomicPlanDigest(version int, legs []ExpectedEconomicLeg) (string, error) {
	payload := economicPlanDigestPayload{
		Domain: "midaz.expected-economic-plan", Version: version,
		Legs: append([]ExpectedEconomicLeg(nil), legs...),
	}
	canonicalizeExpectedEconomicLegs(payload.Legs)

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal expected economic plan: %w", err)
	}

	digest := sha256.Sum256(raw)

	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func canonicalizeExpectedEconomicLegs(legs []ExpectedEconomicLeg) {
	sort.Slice(legs, func(i, j int) bool {
		left, _ := json.Marshal(legs[i])
		right, _ := json.Marshal(legs[j])

		return string(left) < string(right)
	})
}
