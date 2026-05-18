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
	now := time.Now()
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
