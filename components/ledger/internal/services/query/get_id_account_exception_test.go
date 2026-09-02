// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accountexception"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestGetAccountExceptionByIDSuccess: a live exception is returned.
func TestGetAccountExceptionByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	expected := &mmodel.AccountException{ID: id.String(), AccountID: accountID.String()}

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(expected, nil)

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.GetAccountExceptionByID(context.Background(), organizationID, ledgerID, accountID, id)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

// TestGetAccountExceptionByIDNotFound: FindByID returning 0503 propagates as 404.
func TestGetAccountExceptionByIDNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	businessErr := pkg.ValidateBusinessError(constant.ErrAccountExceptionNotFound, constant.EntityAccountException)

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(nil, businessErr)

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.GetAccountExceptionByID(context.Background(), organizationID, ledgerID, accountID, id)

	require.Error(t, err)
	assert.Nil(t, result)

	var notFound pkg.EntityNotFoundError
	require.True(t, errors.As(err, &notFound))
	assert.Equal(t, constant.ErrAccountExceptionNotFound.Error(), notFound.Code)
}

// TestGetAccountExceptionByIDTechnicalError: a non-business error is propagated as-is.
func TestGetAccountExceptionByIDTechnicalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(nil, errors.New("connection reset"))

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.GetAccountExceptionByID(context.Background(), organizationID, ledgerID, accountID, id)

	require.Error(t, err)
	assert.Nil(t, result)
}
