// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// Balance scope constants for BalanceSettings.BalanceScope.
const (
	// BalanceScopeTransactional identifies a balance that can participate in
	// user-initiated transactions. This is the default scope.
	BalanceScopeTransactional = "transactional"

	// BalanceScopeInternal identifies a balance managed exclusively by the
	// system (e.g. overdraft reserves). Direct operations on internal-scope
	// balances are rejected by the transaction engine.
	BalanceScopeInternal = "internal"
)

// BalanceSettings captures the optional per-balance configuration introduced by
// the overdraft feature. It is stored as a JSONB column on the balance row and
// propagated through the transaction engine via Balance.Settings.
//
// swagger:model BalanceSettings
// @Description Optional per-balance configuration controlling overdraft behavior and balance scope.
type BalanceSettings struct {
	// BalanceScope identifies how the balance participates in transactions.
	// Allowed values: "transactional" (default), "internal". Empty string is
	// treated as "transactional" for backwards compatibility.
	// example: transactional
	BalanceScope string `json:"balanceScope,omitempty" example:"transactional"`

	// AllowOverdraft enables overdraft behavior for the balance. When false,
	// transactions that would drive Available below zero are rejected.
	// example: false
	AllowOverdraft bool `json:"allowOverdraft" example:"false"`

	// OverdraftLimitEnabled gates the OverdraftLimit field. When true, a
	// non-empty, strictly positive OverdraftLimit MUST be supplied. When
	// false, OverdraftLimit MUST be absent (overdraft is unlimited if
	// AllowOverdraft is true).
	// example: false
	OverdraftLimitEnabled bool `json:"overdraftLimitEnabled" example:"false"`

	// OverdraftLimit is the maximum overdraft amount the balance may carry,
	// expressed as a decimal string (to preserve precision). Ignored when
	// OverdraftLimitEnabled is false.
	// example: 1000.00
	OverdraftLimit *string `json:"overdraftLimit,omitempty" example:"1000.00"`
}

// NewDefaultBalanceSettings returns a BalanceSettings initialized with the
// defaults mandated by PRD RF-1:
//   - BalanceScope  = "transactional"
//   - AllowOverdraft = false
//   - OverdraftLimitEnabled = false
//   - OverdraftLimit = nil
//
// The constructor centralizes default creation so call sites never end up
// with a zero-valued BalanceSettings that would leave BalanceScope empty.
func NewDefaultBalanceSettings() *BalanceSettings {
	return &BalanceSettings{
		BalanceScope:          BalanceScopeTransactional,
		AllowOverdraft:        false,
		OverdraftLimitEnabled: false,
		OverdraftLimit:        nil,
	}
}

// Validate enforces the balance settings contract from PRD §4 RF-1.
//
// Rules:
//   - BalanceScope MUST be "transactional", "internal", or empty (=>
//     defaults to "transactional"). Any other value (including case
//     variants) is rejected.
//   - When OverdraftLimitEnabled is true, OverdraftLimit MUST be present,
//     parse as a valid decimal via shopspring/decimal, and be strictly
//     greater than zero.
//   - When OverdraftLimitEnabled is false, OverdraftLimit MUST be absent
//     (nil). A present OverdraftLimit with the flag disabled is ambiguous
//     and therefore rejected.
func (s *BalanceSettings) Validate() error {
	if s == nil {
		return nil
	}

	if err := s.validateScope(); err != nil {
		return err
	}

	return s.validateOverdraftLimit()
}

// validateScope returns an error when BalanceScope is not one of the
// allowed values. Empty string is accepted and interpreted as the default
// "transactional" scope by downstream consumers.
func (s *BalanceSettings) validateScope() error {
	switch s.BalanceScope {
	case "", BalanceScopeTransactional, BalanceScopeInternal:
		return nil
	default:
		return fmt.Errorf(
			"invalid balanceScope %q: must be %q or %q",
			s.BalanceScope, BalanceScopeTransactional, BalanceScopeInternal,
		)
	}
}

// validateOverdraftLimit enforces the cross-field rule between
// OverdraftLimitEnabled and OverdraftLimit.
func (s *BalanceSettings) validateOverdraftLimit() error {
	if !s.OverdraftLimitEnabled {
		if s.OverdraftLimit != nil {
			return errors.New(
				"overdraftLimit must be absent when overdraftLimitEnabled is false",
			)
		}

		return nil
	}

	// OverdraftLimitEnabled == true: require a strictly positive decimal.
	if s.OverdraftLimit == nil {
		return errors.New(
			"overdraftLimit is required when overdraftLimitEnabled is true",
		)
	}

	if *s.OverdraftLimit == "" {
		return errors.New(
			"overdraftLimit must be a non-empty decimal string when overdraftLimitEnabled is true",
		)
	}

	limit, err := decimal.NewFromString(*s.OverdraftLimit)
	if err != nil {
		return fmt.Errorf(
			"overdraftLimit %q is not a valid decimal: %w",
			*s.OverdraftLimit, err,
		)
	}

	if !limit.IsPositive() {
		return fmt.Errorf(
			"overdraftLimit %q must be strictly greater than zero",
			*s.OverdraftLimit,
		)
	}

	return nil
}
