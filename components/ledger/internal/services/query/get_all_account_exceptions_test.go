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
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestGetAllAccountExceptionsSuccess: a non-empty page is returned with the envelope
// carrying the requested limit/page.
func TestGetAllAccountExceptionsSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	items := []*mmodel.AccountException{
		{ID: uuid.New().String(), AccountID: accountID.String()},
		{ID: uuid.New().String(), AccountID: accountID.String()},
	}

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, filter http.Pagination) ([]*mmodel.AccountException, error) {
			assert.Equal(t, 10, filter.Limit)
			assert.Equal(t, 2, filter.Page)
			return items, nil
		})

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, pagination, err := uc.GetAllAccountExceptions(context.Background(), organizationID, ledgerID, accountID,
		http.QueryHeader{Limit: 10, Page: 2})

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, 10, pagination.Limit)
	assert.Equal(t, 2, pagination.Page)
}

// TestGetAllAccountExceptionsEmpty: an empty page raises 0504 (no exceptions found).
func TestGetAllAccountExceptionsEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
		Return([]*mmodel.AccountException{}, nil)

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, _, err := uc.GetAllAccountExceptions(context.Background(), organizationID, ledgerID, accountID,
		http.QueryHeader{Limit: 10, Page: 1})

	require.Error(t, err)
	assert.Nil(t, result)

	var notFound pkg.EntityNotFoundError
	require.True(t, errors.As(err, &notFound))
	assert.Equal(t, constant.ErrNoAccountExceptionsFound.Error(), notFound.Code)
}

// TestGetAllAccountExceptionsRepoError: a technical repository failure is propagated.
func TestGetAllAccountExceptionsRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
		Return(nil, errors.New("connection reset"))

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, _, err := uc.GetAllAccountExceptions(context.Background(), organizationID, ledgerID, accountID,
		http.QueryHeader{Limit: 10, Page: 1})

	require.Error(t, err)
	assert.Nil(t, result)
}
