// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accountexception"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// pkgValidateNotFound mirrors the business error the account-exception repository's
// FindByID returns on a missing row (0503), so update/query tests can drive that path.
func pkgValidateNotFound() error {
	return pkg.ValidateBusinessError(constant.ErrAccountExceptionNotFound, constant.EntityAccountException)
}

// TestUpdateAccountExceptionSuccess: a PATCH merges onto the stored state, validates
// the final window, and persists.
func TestUpdateAccountExceptionSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	stored := &mmodel.AccountException{
		ID:                   id.String(),
		OrganizationID:       organizationID.String(),
		LedgerID:             ledgerID.String(),
		AccountID:            accountID.String(),
		OperationalTypeCodes: []string{"PIX_IN"},
		Context:              "old",
	}

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(stored, nil)
	mockExceptionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountID, id, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _, _ uuid.UUID, patch *mmodel.AccountException) (*mmodel.AccountException, error) {
			assert.Equal(t, "new", patch.Context)
			out := *stored
			out.Context = "new"
			return &out, nil
		})

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{Context: ptrString("new")})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "new", result.Context)
}

// TestUpdateAccountExceptionAllFields: a PATCH carrying every field populates the whole
// partial entity and validates the merged (fully-supplied) window.
func TestUpdateAccountExceptionAllFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	effectiveAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	stored := &mmodel.AccountException{ID: id.String(), AccountID: accountID.String()}

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(stored, nil)
	mockExceptionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountID, id, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _, _ uuid.UUID, patch *mmodel.AccountException) (*mmodel.AccountException, error) {
			assert.Equal(t, []string{"PIX_IN", "TED_OUT"}, patch.OperationalTypeCodes)
			require.NotNil(t, patch.BalanceKey)
			assert.Equal(t, "asset-freeze", *patch.BalanceKey)
			assert.Equal(t, "reason", patch.Context)
			assert.Equal(t, &effectiveAt, patch.EffectiveAt)
			assert.Equal(t, &expiresAt, patch.ExpiresAt)
			out := *stored
			return &out, nil
		})

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{
			OperationalTypeCodes: []string{"PIX_IN", "TED_OUT"},
			BalanceKey:           ptrString("asset-freeze"),
			Context:              ptrString("reason"),
			EffectiveAt:          &effectiveAt,
			ExpiresAt:            &expiresAt,
		})

	require.NoError(t, err)
	require.NotNil(t, result)
}

// TestUpdateAccountExceptionRepoError: a technical Update failure is propagated as-is.
func TestUpdateAccountExceptionRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	stored := &mmodel.AccountException{ID: id.String(), AccountID: accountID.String()}

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(stored, nil)
	mockExceptionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountID, id, gomock.Any()).
		Return(nil, errors.New("connection reset"))

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{Context: ptrString("new")})

	require.Error(t, err)
	assert.Nil(t, result)
}

// TestUpdateAccountExceptionClearBalanceKey: a non-nil empty balanceKey pointer is passed
// through so the repository maps it to NULL (clear semantics).
func TestUpdateAccountExceptionClearBalanceKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	stored := &mmodel.AccountException{
		ID:             id.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		BalanceKey:     ptrString("asset-freeze"),
	}

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(stored, nil)
	mockExceptionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountID, id, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _, _ uuid.UUID, patch *mmodel.AccountException) (*mmodel.AccountException, error) {
			require.NotNil(t, patch.BalanceKey, "the clear sentinel is a non-nil pointer to empty string")
			assert.Equal(t, "", *patch.BalanceKey)
			out := *stored
			out.BalanceKey = nil
			return &out, nil
		})

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{BalanceKey: ptrString("")})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.BalanceKey)
}

// TestUpdateAccountExceptionNotFound: FindByID returning 0503 propagates as 404.
func TestUpdateAccountExceptionNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(nil, pkgValidateNotFound())

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{Context: ptrString("new")})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, constant.ErrAccountExceptionNotFound.Error(), extractCode(t, err))
}

// TestUpdateAccountExceptionUpdateNotFound: Update returning ErrDatabaseItemNotFound
// (concurrent delete between load and persist) maps to 0503.
func TestUpdateAccountExceptionUpdateNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	stored := &mmodel.AccountException{ID: id.String(), AccountID: accountID.String()}

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(stored, nil)
	mockExceptionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountID, id, gomock.Any()).
		Return(nil, services.ErrDatabaseItemNotFound)

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{Context: ptrString("new")})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, constant.ErrAccountExceptionNotFound.Error(), extractCode(t, err))
}

// TestUpdateAccountExceptionInvalidMergedWindow: a PATCH moving only one bound inverts
// a previously valid window and is rejected as 0505.
func TestUpdateAccountExceptionInvalidMergedWindow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	effectiveAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	storedExpires := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	stored := &mmodel.AccountException{
		ID:          id.String(),
		AccountID:   accountID.String(),
		EffectiveAt: &effectiveAt,
		ExpiresAt:   &storedExpires,
	}

	newExpires := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(stored, nil)

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{ExpiresAt: &newExpires})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, constant.ErrInvalidAccountExceptionValidityWindow.Error(), extractCode(t, err))
}

// TestUpdateAccountException_EmitsUpdatedEvent: a successful update publishes one
// account_exception.updated event.
func TestUpdateAccountException_EmitsUpdatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	stored := &mmodel.AccountException{ID: id.String(), AccountID: accountID.String(), Context: "old"}

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(stored, nil)
	mockExceptionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountID, id, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _, _ uuid.UUID, patch *mmodel.AccountException) (*mmodel.AccountException, error) {
			out := *stored
			out.Context = patch.Context
			return &out, nil
		})

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo, Streaming: mockEmitter}

	result, err := uc.UpdateAccountException(context.Background(), organizationID, ledgerID, accountID, id,
		&mmodel.UpdateAccountExceptionInput{Context: ptrString("new")})

	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account_exception", "updated")
	assert.Equal(t, "account_exception.updated", emitted[0].DefinitionKey)
}
