// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accountexception"
	onbRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/onboarding"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestCreateAccountException_InvalidatesCacheBeforeWrite proves invalidate-first
// ordering: Del(cacheKey) is called BEFORE AccountExceptionRepo.Create.
func TestCreateAccountException_InvalidatesCacheBeforeWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(organizationID, ledgerID, accountID)

	mockRedis := onbRedis.NewMockRedisRepository(ctrl)
	delCall := mockRedis.EXPECT().Del(gomock.Any(), cacheKey).Return(nil)

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit"}, nil)

	mockExc := accountexception.NewMockRepository(ctrl)
	createCall := mockExc.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, e *mmodel.AccountException) (*mmodel.AccountException, error) {
			return e, nil
		})

	gomock.InOrder(delCall, createCall)

	uc := &UseCase{AccountRepo: mockAccountRepo, AccountExceptionRepo: mockExc, OnboardingRedisRepo: mockRedis}

	result, err := uc.CreateAccountException(context.Background(), organizationID, ledgerID, accountID,
		&mmodel.CreateAccountExceptionInput{OperationalTypeCodes: []string{"PIX_IN"}, Context: "ctx"})

	require.NoError(t, err)
	require.NotNil(t, result)
}

// TestCreateAccountException_RedisDownRefusesWrite proves a failed invalidation
// refuses the write: the Postgres repo is NEVER called (neither the account
// lookup nor the exception insert).
func TestCreateAccountException_RedisDownRefusesWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(organizationID, ledgerID, accountID)

	mockRedis := onbRedis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Del(gomock.Any(), cacheKey).Return(errors.New("redis down"))

	// No expectations: a refused write must not touch Postgres at all.
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockExc := accountexception.NewMockRepository(ctrl)

	uc := &UseCase{AccountRepo: mockAccountRepo, AccountExceptionRepo: mockExc, OnboardingRedisRepo: mockRedis}

	result, err := uc.CreateAccountException(context.Background(), organizationID, ledgerID, accountID,
		&mmodel.CreateAccountExceptionInput{OperationalTypeCodes: []string{"PIX_IN"}, Context: "ctx"})

	require.Error(t, err)
	assert.Nil(t, result)
}

// TestUpdateAccountException_InvalidatesCacheBeforeWrite: Del precedes Update.
func TestUpdateAccountException_InvalidatesCacheBeforeWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(organizationID, ledgerID, accountID)

	mockRedis := onbRedis.NewMockRedisRepository(ctrl)
	delCall := mockRedis.EXPECT().Del(gomock.Any(), cacheKey).Return(nil)

	current := &mmodel.AccountException{ID: id.String(), AccountID: accountID.String()}
	updated := &mmodel.AccountException{ID: id.String(), AccountID: accountID.String(), Context: "new"}

	mockExc := accountexception.NewMockRepository(ctrl)
	mockExc.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(current, nil)
	updateCall := mockExc.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountID, id, gomock.Any()).
		Return(updated, nil)

	gomock.InOrder(delCall, updateCall)

	uc := &UseCase{AccountExceptionRepo: mockExc, OnboardingRedisRepo: mockRedis}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{Context: ptrString("new")})

	require.NoError(t, err)
	require.NotNil(t, result)
}

// TestUpdateAccountException_RedisDownRefusesWrite: a failed invalidation refuses
// the write; neither FindByID nor Update is reached.
func TestUpdateAccountException_RedisDownRefusesWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(organizationID, ledgerID, accountID)

	mockRedis := onbRedis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Del(gomock.Any(), cacheKey).Return(errors.New("redis down"))

	// No expectations: the write is refused before any Postgres call.
	mockExc := accountexception.NewMockRepository(ctrl)

	uc := &UseCase{AccountExceptionRepo: mockExc, OnboardingRedisRepo: mockRedis}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{Context: ptrString("new")})

	require.Error(t, err)
	assert.Nil(t, result)
}

// TestDeleteAccountException_InvalidatesCacheBeforeWrite: Del precedes Delete.
func TestDeleteAccountException_InvalidatesCacheBeforeWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(organizationID, ledgerID, accountID)

	mockRedis := onbRedis.NewMockRedisRepository(ctrl)
	delCall := mockRedis.EXPECT().Del(gomock.Any(), cacheKey).Return(nil)

	mockExc := accountexception.NewMockRepository(ctrl)
	deleteCall := mockExc.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(nil)

	gomock.InOrder(delCall, deleteCall)

	uc := &UseCase{AccountExceptionRepo: mockExc, OnboardingRedisRepo: mockRedis}

	err := uc.DeleteAccountException(context.Background(), organizationID, ledgerID, accountID, id)

	require.NoError(t, err)
}

// TestDeleteAccountException_RedisDownRefusesWrite: a failed invalidation refuses
// the delete; the Postgres repo is never called.
func TestDeleteAccountException_RedisDownRefusesWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()
	cacheKey := utils.AccountExceptionsInternalKey(organizationID, ledgerID, accountID)

	mockRedis := onbRedis.NewMockRedisRepository(ctrl)
	mockRedis.EXPECT().Del(gomock.Any(), cacheKey).Return(errors.New("redis down"))

	// No expectations: the delete is refused before any Postgres call.
	mockExc := accountexception.NewMockRepository(ctrl)

	uc := &UseCase{AccountExceptionRepo: mockExc, OnboardingRedisRepo: mockRedis}

	err := uc.DeleteAccountException(context.Background(), organizationID, ledgerID, accountID, id)

	require.Error(t, err)
}
