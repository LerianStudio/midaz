// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import (
	"errors"
	"strings"
	"time"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

const bankAccountIndexCollection = "alias_bank_account_index"

type BankAccountIndexModel struct {
	ID             *uuid.UUID                      `bson:"_id,omitempty"`
	AliasID        *uuid.UUID                      `bson:"alias_id,omitempty"`
	OrganizationID *string                         `bson:"organization_id,omitempty"`
	LedgerID       *string                         `bson:"ledger_id,omitempty"`
	AccountID      *string                         `bson:"account_id,omitempty"`
	HolderID       *uuid.UUID                      `bson:"holder_id,omitempty"`
	Document       *string                         `bson:"document,omitempty"`
	Type           *string                         `bson:"type,omitempty"`
	Search         *BankAccountIndexSearchMongoDB  `bson:"search,omitempty"`
	BankingDetails *BankAccountIndexBankingMongoDB `bson:"banking_details,omitempty"`
	CreatedAt      *time.Time                      `bson:"created_at,omitempty"`
	UpdatedAt      *time.Time                      `bson:"updated_at,omitempty"`
	DeletedAt      *time.Time                      `bson:"deleted_at"`
}

type BankAccountIndexSearchMongoDB struct {
	Document              *string `bson:"document,omitempty"`
	BankingDetailsAccount *string `bson:"banking_details_account,omitempty"`
}

type BankAccountIndexBankingMongoDB struct {
	BankID          *string `bson:"bank_id,omitempty"`
	Branch          *string `bson:"branch,omitempty"`
	BranchCanonical *string `bson:"branch_canonical,omitempty"`
	Account         *string `bson:"account,omitempty"`
	Type            *string `bson:"type,omitempty"`
}

func canonicalizeBankAccountBranch(branch string) string {
	canonical := strings.TrimLeft(strings.TrimSpace(branch), "0")
	if canonical == "" {
		return "0"
	}

	return canonical
}

func bankAccountIndexModelFromAlias(organizationID string, alias *mmodel.Alias, ds *libCrypto.Crypto) (*BankAccountIndexModel, error) {
	if alias == nil || alias.ID == nil {
		return nil, nil
	}

	document, err := encryptOptional(ds, alias.Document)
	if err != nil {
		return nil, err
	}

	organization := organizationID

	createdAt := alias.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	updatedAt := alias.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	model := &BankAccountIndexModel{
		ID:             alias.ID,
		AliasID:        alias.ID,
		OrganizationID: &organization,
		LedgerID:       alias.LedgerID,
		AccountID:      alias.AccountID,
		HolderID:       alias.HolderID,
		Document:       document,
		Type:           alias.Type,
		Search:         &BankAccountIndexSearchMongoDB{},
		CreatedAt:      &createdAt,
		UpdatedAt:      &updatedAt,
		DeletedAt:      alias.DeletedAt,
	}

	if alias.Document != nil && strings.TrimSpace(*alias.Document) != "" {
		documentHash := ds.GenerateHash(alias.Document)
		model.Search.Document = &documentHash
	}

	if hasCompleteBankAccountIdentity(alias.BankingDetails) {
		account, err := encryptOptional(ds, alias.BankingDetails.Account)
		if err != nil {
			return nil, err
		}

		accountHash := ds.GenerateHash(alias.BankingDetails.Account)
		branchCanonical := canonicalizeBankAccountBranch(*alias.BankingDetails.Branch)
		model.Search.BankingDetailsAccount = &accountHash
		model.BankingDetails = &BankAccountIndexBankingMongoDB{
			BankID:          alias.BankingDetails.BankID,
			Branch:          alias.BankingDetails.Branch,
			BranchCanonical: &branchCanonical,
			Account:         account,
			Type:            alias.BankingDetails.Type,
		}
	}

	return model, nil
}

//nolint:gocyclo // Decoder validates the full resolver routing boundary before returning domain data.
func (model *BankAccountIndexModel) ToAlias(ds *libCrypto.Crypto) (*mmodel.Alias, error) {
	if model == nil {
		return nil, nil
	}

	if model.AliasID == nil || model.OrganizationID == nil || model.LedgerID == nil || model.AccountID == nil || model.HolderID == nil || model.CreatedAt == nil || model.UpdatedAt == nil {
		return nil, errors.New("malformed bank account index row: missing required routing fields")
	}

	if model.AliasID != nil && *model.AliasID == uuid.Nil || model.HolderID != nil && *model.HolderID == uuid.Nil || isZeroUUIDString(model.OrganizationID) || isZeroUUIDString(model.LedgerID) || isZeroUUIDString(model.AccountID) {
		return nil, errors.New("malformed bank account index row: zero UUID routing fields")
	}

	organizationID, err := parseRequiredUUIDString(model.OrganizationID)
	if err != nil {
		return nil, errors.New("malformed bank account index row: invalid organization_id")
	}

	document, err := decryptOptional(ds, model.Document)
	if err != nil {
		return nil, err
	}

	alias := &mmodel.Alias{
		ID:             model.AliasID,
		OrganizationID: &organizationID,
		Document:       document,
		Type:           model.Type,
		LedgerID:       model.LedgerID,
		AccountID:      model.AccountID,
		HolderID:       model.HolderID,
		CreatedAt:      *model.CreatedAt,
		UpdatedAt:      *model.UpdatedAt,
		DeletedAt:      model.DeletedAt,
	}

	if model.BankingDetails != nil {
		account, err := decryptOptional(ds, model.BankingDetails.Account)
		if err != nil {
			return nil, err
		}

		alias.BankingDetails = &mmodel.BankingDetails{
			BankID:  model.BankingDetails.BankID,
			Branch:  model.BankingDetails.Branch,
			Account: account,
			Type:    model.BankingDetails.Type,
		}
	}

	return alias, nil
}

func hasCompleteBankAccountIdentity(bankingDetails *mmodel.BankingDetails) bool {
	return bankingDetails != nil &&
		bankingDetails.BankID != nil && strings.TrimSpace(*bankingDetails.BankID) != "" &&
		bankingDetails.Branch != nil && strings.TrimSpace(*bankingDetails.Branch) != "" &&
		bankingDetails.Account != nil && strings.TrimSpace(*bankingDetails.Account) != "" &&
		bankingDetails.Type != nil && strings.TrimSpace(*bankingDetails.Type) != ""
}

func hasAnyBankAccountIdentity(bankingDetails *mmodel.BankingDetails) bool {
	return bankingDetails != nil &&
		((bankingDetails.BankID != nil && strings.TrimSpace(*bankingDetails.BankID) != "") ||
			(bankingDetails.Branch != nil && strings.TrimSpace(*bankingDetails.Branch) != "") ||
			(bankingDetails.Account != nil && strings.TrimSpace(*bankingDetails.Account) != "") ||
			(bankingDetails.Type != nil && strings.TrimSpace(*bankingDetails.Type) != ""))
}

func isZeroUUIDString(value *string) bool {
	if value == nil {
		return false
	}

	id, err := uuid.Parse(*value)

	return err == nil && id == uuid.Nil
}

func parseRequiredUUIDString(value *string) (uuid.UUID, error) {
	if value == nil {
		return uuid.Nil, errors.New("missing UUID")
	}

	id, err := uuid.Parse(*value)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, errors.New("invalid UUID")
	}

	return id, nil
}
