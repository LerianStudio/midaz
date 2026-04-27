// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetAliasByAccount_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	aliasRepo := alias.NewMockRepository(ctrl)

	orgID := uuid.NewString()
	ledgerID := uuid.NewString()
	accountID := uuid.NewString()
	aliasID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	expected := &mmodel.Alias{
		ID:        &aliasID,
		LedgerID:  &ledgerID,
		AccountID: &accountID,
		HolderID:  &holderID,
	}

	aliasRepo.EXPECT().
		FindByLedgerAndAccount(gomock.Any(), orgID, ledgerID, accountID).
		Return(expected, nil).
		Times(1)

	uc := &UseCase{AliasRepo: aliasRepo}

	got, err := uc.GetAliasByAccount(context.Background(), orgID, ledgerID, accountID)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, &aliasID, got.ID)
}

func TestGetAliasByAccount_NotFound_PropagatesBusinessError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	aliasRepo := alias.NewMockRepository(ctrl)

	orgID := uuid.NewString()
	ledgerID := uuid.NewString()
	accountID := uuid.NewString()

	notFound := pkg.ValidateBusinessError(constant.ErrAliasNotFound, "Alias")

	aliasRepo.EXPECT().
		FindByLedgerAndAccount(gomock.Any(), orgID, ledgerID, accountID).
		Return(nil, notFound).
		Times(1)

	uc := &UseCase{AliasRepo: aliasRepo}

	got, err := uc.GetAliasByAccount(context.Background(), orgID, ledgerID, accountID)

	require.Error(t, err)
	require.Nil(t, got)

	notFoundErr, ok := err.(pkg.EntityNotFoundError)
	require.True(t, ok, "expected EntityNotFoundError, got %T", err)
	assert.Equal(t, constant.ErrAliasNotFound.Error(), notFoundErr.Code)
}
