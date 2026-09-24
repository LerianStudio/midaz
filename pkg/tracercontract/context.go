// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tracercontract defines facts exchanged with Tracer. It does not own
// account classifications, policy decisions, limit consumption, or accounting.
package tracercontract

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// AssetIdentity identifies an asset within the authenticated tenant. Namespace
// must be authorized by the receiving integration, never trusted from the body.
// ID is opaque so producers need not share an asset registry or UUID scheme.
type AssetIdentity struct {
	Namespace string
	ID        string
}

// AssetRef carries an identity and its human-readable code. Equal codes do not
// imply equivalent assets. Conflicting codes for one identity are invalid.
type AssetRef struct {
	Namespace string `json:"namespace"`
	ID        string `json:"id"`
	Code      string `json:"code"`
}

// Identity excludes Code, which is descriptive rather than an identity key.
func (a AssetRef) Identity() AssetIdentity {
	return AssetIdentity{Namespace: a.Namespace, ID: a.ID}
}

// Account contains official facts about one participating internal account.
// Type and Status retain the producer's vocabulary. Blocked must be present;
// false is a fact, whereas nil is incomplete context. These facts do not decide
// whether an operation is allowed: that remains a policy concern in Tracer.
type Account struct {
	ID      uuid.UUID `json:"id"`
	Type    string    `json:"type"`
	Status  string    `json:"status"`
	Blocked *bool     `json:"blocked"`
	Asset   AssetRef  `json:"asset"`
}

// Direction describes a posting, independently of payment rail or account type.
type Direction string

const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)

// Entry refers to an internal account or explicitly represents an external
// participant. External entries never invent an internal account ID or facts.
// Amount is a positive magnitude; Direction conveys the posting direction.
type Entry struct {
	AccountID uuid.UUID `json:"accountId,omitzero"`
	External  bool      `json:"external,omitempty"`
	Direction Direction `json:"direction"`
	Amount    Amount    `json:"amount"`
	Asset     AssetRef  `json:"asset"`
}

// Context describes the participating accounts once and their prepared entries,
// including fee entries. Tenant and authenticated integration identity do not
// belong in this caller-controlled body. No aggregate consumption is supplied.
type Context struct {
	Accounts []Account `json:"accounts"`
	Entries  []Entry   `json:"entries"`
}

// Limits bounds work before decimal parsing and evaluation. Callers must supply
// measured/configured limits explicitly; this package imposes no hidden business
// precision, currency scale, account-count default, or rounding policy.
type Limits struct {
	MaxAccounts       int
	MaxEntries        int
	MaxTextBytes      int
	MaxIntegerDigits  int
	MaxFractionDigits int
}

// Validate requires explicit positive collection/text bounds and a valid decimal range.
func (l Limits) Validate() error {
	if l.MaxAccounts <= 0 || l.MaxEntries <= 0 || l.MaxTextBytes <= 0 ||
		l.MaxIntegerDigits <= 0 || l.MaxFractionDigits < 0 {
		return invalid("resource limits")
	}

	return nil
}

func invalid(field string) error {
	return fmt.Errorf("%s: %w", field, constant.ErrInvalidRequestBody)
}

func validText(s string, limit int) bool {
	return len(s) > 0 && len(s) <= limit && utf8.ValidString(s) &&
		!strings.ContainsRune(s, '\x00') && strings.TrimSpace(s) == s
}

// Validate checks structure and completeness, not policy or accounting balance.
// authorizedNamespace must come from trusted integration configuration. Tenant
// isolation and authorization of the context remain adapter responsibilities.
// The context is not mutated, so official producer facts cannot be rewritten.
func (c Context) Validate(ctx context.Context, authorizedNamespace string, limits Limits) error {
	return c.validate(ctx, authorizedNamespace, limits, nil)
}

// validate accepts previously validated asset identities from the envelope so
// contradictory codes are rejected across both header and participating facts.
func (c Context) validate(ctx context.Context, authorizedNamespace string, limits Limits, assets map[AssetIdentity]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := limits.Validate(); err != nil {
		return err
	}

	if !validText(authorizedNamespace, limits.MaxTextBytes) {
		return invalid("authorized namespace")
	}

	if len(c.Accounts) > limits.MaxAccounts || len(c.Entries) == 0 || len(c.Entries) > limits.MaxEntries {
		return invalid("context size")
	}

	accounts := make(map[uuid.UUID]Account, len(c.Accounts))
	if assets == nil {
		assets = make(map[AssetIdentity]string)
	}

	for i, account := range c.Accounts {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := validateAccount(account, authorizedNamespace, limits, accounts, assets); err != nil {
			return fmt.Errorf("accounts[%d]: %w", i, err)
		}

		accounts[account.ID] = account
	}

	used := make(map[uuid.UUID]bool, len(accounts))

	for i, entry := range c.Entries {
		if err := validateEntry(ctx, entry, authorizedNamespace, limits, accounts, assets); err != nil {
			return fmt.Errorf("entries[%d]: %w", i, err)
		}

		if !entry.External {
			used[entry.AccountID] = true
		}
	}

	if len(used) != len(accounts) {
		return invalid("unreferenced account")
	}

	return nil
}

func validateAccount(account Account, namespace string, limits Limits, accounts map[uuid.UUID]Account, assets map[AssetIdentity]string) error {
	if account.ID == uuid.Nil || !validText(account.Type, limits.MaxTextBytes) ||
		!validText(account.Status, limits.MaxTextBytes) || account.Blocked == nil {
		return invalid("account facts")
	}

	if _, exists := accounts[account.ID]; exists {
		return invalid("duplicate account")
	}

	return validateAsset(account.Asset, namespace, limits, assets)
}

func validateAsset(asset AssetRef, namespace string, limits Limits, seen map[AssetIdentity]string) error {
	if asset.Namespace != namespace || !validText(asset.ID, limits.MaxTextBytes) || !validText(asset.Code, limits.MaxTextBytes) {
		return invalid("asset reference")
	}

	if code, exists := seen[asset.Identity()]; exists && code != asset.Code {
		return invalid("conflicting asset code")
	}

	seen[asset.Identity()] = asset.Code

	return nil
}

func validateEntry(ctx context.Context, entry Entry, namespace string, limits Limits, accounts map[uuid.UUID]Account, assets map[AssetIdentity]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if entry.Direction != Debit && entry.Direction != Credit {
		return invalid("direction")
	}

	if entry.External != (entry.AccountID == uuid.Nil) {
		return invalid("participant reference")
	}

	if err := validateAsset(entry.Asset, namespace, limits, assets); err != nil {
		return err
	}

	if !entry.External {
		account, exists := accounts[entry.AccountID]
		if !exists || account.Asset != entry.Asset {
			return invalid("account asset reference")
		}
	}

	amount, err := entry.Amount.Decimal(ctx, limits)
	if err != nil {
		return err
	}

	if !amount.IsPositive() {
		return invalid("positive amount required")
	}

	return nil
}
