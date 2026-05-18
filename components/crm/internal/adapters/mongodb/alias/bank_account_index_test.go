// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBankAccountIndexModelFromAlias_StoresCanonicalBranchAndProtectsSensitiveFields(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	aliasID := uuid.New()
	holderID := uuid.New()
	document := "12345678901"
	account := "001234567"
	branch := "0000"
	bankID := "12345678"
	accountType := "CACC"
	organizationID := "00000000-0000-0000-0000-000000000001"
	ledgerID := "00000000-0000-0000-0000-000000000002"
	accountID := "00000000-0000-0000-0000-000000000003"

	model, err := bankAccountIndexModelFromAlias(organizationID, &mmodel.Alias{
		ID:        &aliasID,
		Document:  &document,
		LedgerID:  &ledgerID,
		AccountID: &accountID,
		HolderID:  &holderID,
		BankingDetails: &mmodel.BankingDetails{
			BankID:  &bankID,
			Branch:  &branch,
			Account: &account,
			Type:    &accountType,
		},
	}, crypto)

	require.NoError(t, err)
	require.NotNil(t, model)
	assert.Equal(t, "0", *model.BankingDetails.BranchCanonical)
	assert.Equal(t, branch, *model.BankingDetails.Branch)
	assert.Equal(t, crypto.GenerateHash(&document), *model.Search.Document)
	assert.Equal(t, crypto.GenerateHash(&account), *model.Search.BankingDetailsAccount)
	assert.NotEqual(t, document, *model.Document)
	assert.NotEqual(t, account, *model.BankingDetails.Account)
}

func TestCanonicalizeBankAccountBranch(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "1", canonicalizeBankAccountBranch("0001"))
	assert.Equal(t, "123", canonicalizeBankAccountBranch("00123"))
	assert.Equal(t, "0", canonicalizeBankAccountBranch("0000"))
	assert.Equal(t, "0", canonicalizeBankAccountBranch(""))
}

func TestBankAccountIndexModelToAliasRejectsMalformedRoutingFields(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	model := &BankAccountIndexModel{}

	result, err := model.ToAlias(crypto)

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestBankAccountIndexModelToAliasRejectsZeroUUIDRoutingFields(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	zeroID := uuid.Nil
	organizationID := uuid.Nil.String()
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	model := &BankAccountIndexModel{
		AliasID:        &zeroID,
		OrganizationID: &organizationID,
		LedgerID:       &ledgerID,
		AccountID:      &accountID,
		HolderID:       &zeroID,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	result, err := model.ToAlias(crypto)

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestBankAccountIndexModelToAliasReturnsTypedRoutingFields(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	aliasID := uuid.New()
	holderID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()
	document := "12345678901"
	createdAt := time.Date(2026, time.January, 2, 15, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)

	model, err := bankAccountIndexModelFromAlias(organizationID.String(), &mmodel.Alias{
		ID:        &aliasID,
		Document:  &document,
		LedgerID:  &ledgerID,
		AccountID: &accountID,
		HolderID:  &holderID,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, crypto)
	require.NoError(t, err)

	result, err := model.ToAlias(crypto)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, aliasID, *result.ID)
	assert.Equal(t, organizationID, *result.OrganizationID)
	assert.Equal(t, ledgerID, *result.LedgerID)
	assert.Equal(t, accountID, *result.AccountID)
	assert.Equal(t, holderID, *result.HolderID)
	assert.Equal(t, document, *result.Document)
	assert.Equal(t, createdAt, result.CreatedAt)
	assert.Equal(t, updatedAt, result.UpdatedAt)
}

func TestBankAccountIndexModelToAliasRejectsInvalidOrganizationID(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	aliasID := uuid.New()
	holderID := uuid.New()
	organizationID := "not-a-uuid"
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	model := &BankAccountIndexModel{
		AliasID:        &aliasID,
		OrganizationID: &organizationID,
		LedgerID:       &ledgerID,
		AccountID:      &accountID,
		HolderID:       &holderID,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	result, err := model.ToAlias(crypto)

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestBankAccountIndexModelToAliasRejectsInvalidLedgerAndAccountIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*BankAccountIndexModel)
		wantErr string
	}{
		{
			name: "invalid ledger id",
			mutate: func(model *BankAccountIndexModel) {
				invalid := "not-a-uuid"
				model.LedgerID = &invalid
			},
			wantErr: "invalid ledger_id",
		},
		{
			name: "invalid account id",
			mutate: func(model *BankAccountIndexModel) {
				invalid := "not-a-uuid"
				model.AccountID = &invalid
			},
			wantErr: "invalid account_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			crypto := testutils.SetupCrypto(t)
			aliasID := uuid.New()
			holderID := uuid.New()
			organizationID := uuid.New().String()
			ledgerID := uuid.New().String()
			accountID := uuid.New().String()
			now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
			model := &BankAccountIndexModel{
				AliasID:        &aliasID,
				OrganizationID: &organizationID,
				LedgerID:       &ledgerID,
				AccountID:      &accountID,
				HolderID:       &holderID,
				CreatedAt:      &now,
				UpdatedAt:      &now,
			}

			tt.mutate(model)

			result, err := model.ToAlias(crypto)

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Nil(t, result)
		})
	}
}

func TestBankAccountIndexRoutingUUIDHelpers(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	idText := id.String()
	zeroText := uuid.Nil.String()
	invalidText := "not-a-uuid"

	parsed, err := parseRequiredUUIDString(&idText)
	require.NoError(t, err)
	assert.Equal(t, id, parsed)
	assert.True(t, isZeroUUIDString(&zeroText))
	assert.False(t, isZeroUUIDString(&invalidText))
	assert.False(t, isZeroUUIDString(nil))

	_, err = parseRequiredUUIDString(nil)
	require.Error(t, err)
	_, err = parseRequiredUUIDString(&zeroText)
	require.Error(t, err)
}

func TestHasAnyBankAccountIdentity(t *testing.T) {
	t.Parallel()

	bankID := "12345678"
	blank := " "

	assert.False(t, hasAnyBankAccountIdentity(nil))
	assert.False(t, hasAnyBankAccountIdentity(&mmodel.BankingDetails{BankID: &blank}))
	assert.True(t, hasAnyBankAccountIdentity(&mmodel.BankingDetails{BankID: &bankID}))
}

func TestBankAccountIndexModelFromAlias_StoresBaseRowWithoutBankingDetails(t *testing.T) {
	t.Parallel()

	crypto := testutils.SetupCrypto(t)
	aliasID := uuid.New()
	holderID := uuid.New()
	document := "12345678901"
	organizationID := "00000000-0000-0000-0000-000000000001"
	ledgerID := "00000000-0000-0000-0000-000000000002"
	accountID := "00000000-0000-0000-0000-000000000003"

	model, err := bankAccountIndexModelFromAlias(organizationID, &mmodel.Alias{
		ID:        &aliasID,
		Document:  &document,
		LedgerID:  &ledgerID,
		AccountID: &accountID,
		HolderID:  &holderID,
	}, crypto)

	require.NoError(t, err)
	require.NotNil(t, model)
	assert.Nil(t, model.BankingDetails)
	require.NotNil(t, model.Search.Document)
	assert.Equal(t, crypto.GenerateHash(&document), *model.Search.Document)
}
