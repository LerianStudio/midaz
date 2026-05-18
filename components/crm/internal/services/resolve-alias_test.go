// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestResolveBankAccountReturnsDeterministicProofFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := alias.NewMockRepository(ctrl)
	holderRepo := holder.NewMockRepository(ctrl)
	uc := &UseCase{AliasRepo: repo, HolderRepo: holderRepo}

	aliasID := uuid.New()
	holderID := uuid.New()
	document := "12345678901"
	bankID := "12345678"
	branch := "0001"
	account := "001234567"
	accountType := "CACC"
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()
	organizationID := uuid.New().String()

	input := &mmodel.ResolveBankAccountInput{
		Document: document,
		BankingDetails: mmodel.ResolveBankAccountBankingDetailsInput{
			BankID:  bankID,
			Branch:  branch,
			Account: account,
			Type:    accountType,
		},
	}

	repo.EXPECT().ResolveBankAccount(gomock.Any(), input).Return([]*mmodel.Alias{{
		ID:             &aliasID,
		OrganizationID: &organizationID,
		Document:       &document,
		LedgerID:       &ledgerID,
		AccountID:      &accountID,
		HolderID:       &holderID,
		BankingDetails: &mmodel.BankingDetails{
			BankID:  &bankID,
			Branch:  &branch,
			Account: &account,
			Type:    &accountType,
		},
	}}, nil)

	result, err := uc.ResolveBankAccount(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, aliasID.String(), result.ID)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, accountID, result.AccountID)
	assert.Equal(t, holderID.String(), result.HolderID)
	assert.Equal(t, document, result.HolderDocument)
	assert.Equal(t, bankID, result.BankingDetails.BankID)
	assert.Equal(t, branch, result.BankingDetails.Branch)
	assert.Equal(t, account, result.BankingDetails.Account)
	assert.Equal(t, accountType, result.BankingDetails.Type)
}

func TestResolveBankAccountNoMatchReturnsAliasNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := alias.NewMockRepository(ctrl)
	uc := &UseCase{AliasRepo: repo}

	input := &mmodel.ResolveBankAccountInput{
		Document: "12345678901",
		BankingDetails: mmodel.ResolveBankAccountBankingDetailsInput{
			BankID: "12345678", Branch: "0001", Account: "1234567", Type: "CACC",
		},
	}

	repo.EXPECT().ResolveBankAccount(gomock.Any(), input).Return(nil, nil)

	result, err := uc.ResolveBankAccount(context.Background(), input)

	require.Error(t, err)
	assert.Nil(t, result)
	var notFound pkg.EntityNotFoundError
	require.True(t, errors.As(err, &notFound))
	assert.Equal(t, cn.ErrAliasNotFound.Error(), notFound.Code)
}

func TestResolveBankAccountDuplicateMatchesReturnConflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := alias.NewMockRepository(ctrl)
	uc := &UseCase{AliasRepo: repo}
	input := &mmodel.ResolveBankAccountInput{
		Document: "12345678901",
		BankingDetails: mmodel.ResolveBankAccountBankingDetailsInput{
			BankID: "12345678", Branch: "0001", Account: "1234567", Type: "CACC",
		},
	}
	repo.EXPECT().ResolveBankAccount(gomock.Any(), input).Return([]*mmodel.Alias{{ID: ptrUUID(uuid.New())}, {ID: ptrUUID(uuid.New())}}, nil)

	result, err := uc.ResolveBankAccount(context.Background(), input)

	require.Error(t, err)
	assert.Nil(t, result)
	var conflictErr pkg.EntityConflictError
	assert.True(t, errors.As(err, &conflictErr))
}

func TestResolveAccountReturnsOrganizationID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := alias.NewMockRepository(ctrl)
	uc := &UseCase{AliasRepo: repo}

	aliasID := uuid.New()
	holderID := uuid.New()
	accountUUID := uuid.New()
	organizationID := uuid.New().String()
	ledgerID := uuid.New().String()
	document := "12345678901"
	bankID := "12345678"
	branch := "1"
	account := "1234567"
	accountType := "CACC"
	accountID := accountUUID.String()

	repo.EXPECT().ResolveAccount(gomock.Any(), accountUUID).Return([]*mmodel.Alias{{
		ID:             &aliasID,
		OrganizationID: &organizationID,
		Document:       &document,
		LedgerID:       &ledgerID,
		AccountID:      &accountID,
		HolderID:       &holderID,
		BankingDetails: &mmodel.BankingDetails{BankID: &bankID, Branch: &branch, Account: &account, Type: &accountType},
	}}, nil)

	result, err := uc.ResolveAccount(context.Background(), &mmodel.ResolveAccountInput{AccountID: accountUUID.String()})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, organizationID, result.OrganizationID)
	assert.Equal(t, accountUUID.String(), result.AccountID)
}

func TestResolveAccountNoMatchReturnsAliasNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := alias.NewMockRepository(ctrl)
	uc := &UseCase{AliasRepo: repo}
	accountID := uuid.New()
	repo.EXPECT().ResolveAccount(gomock.Any(), accountID).Return(nil, nil)

	result, err := uc.ResolveAccount(context.Background(), &mmodel.ResolveAccountInput{AccountID: accountID.String()})

	require.Error(t, err)
	assert.Nil(t, result)
	var notFound pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &notFound))
}

func TestResolveAccountDuplicateMatchesReturnConflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := alias.NewMockRepository(ctrl)
	uc := &UseCase{AliasRepo: repo}
	accountID := uuid.New()
	repo.EXPECT().ResolveAccount(gomock.Any(), accountID).Return([]*mmodel.Alias{{ID: ptrUUID(uuid.New())}, {ID: ptrUUID(uuid.New())}}, nil)

	result, err := uc.ResolveAccount(context.Background(), &mmodel.ResolveAccountInput{AccountID: accountID.String()})

	require.Error(t, err)
	assert.Nil(t, result)
	var conflictErr pkg.EntityConflictError
	assert.True(t, errors.As(err, &conflictErr))
}

func TestResolveAccountRejectsZeroUUID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := alias.NewMockRepository(ctrl)
	uc := &UseCase{AliasRepo: repo}

	result, err := uc.ResolveAccount(context.Background(), &mmodel.ResolveAccountInput{AccountID: uuid.Nil.String()})

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestResolveBankAccountRejectsNilInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := alias.NewMockRepository(ctrl)
	uc := &UseCase{AliasRepo: repo}

	result, err := uc.ResolveBankAccount(context.Background(), nil)

	require.Error(t, err)
	assert.Nil(t, result)
}

func ptrUUID(id uuid.UUID) *uuid.UUID {
	return &id
}
