// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accountexception"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestDeleteAccountExceptionSuccess: a live exception is soft-deleted.
func TestDeleteAccountExceptionSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(nil)

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	err := uc.DeleteAccountException(context.Background(), organizationID, ledgerID, accountID, id)

	require.NoError(t, err)
}

// TestDeleteAccountExceptionNotFound: a second delete (no live row) maps
// ErrDatabaseItemNotFound to 0503 — a safe retry, no state change.
func TestDeleteAccountExceptionNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(services.ErrDatabaseItemNotFound)

	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo}

	err := uc.DeleteAccountException(context.Background(), organizationID, ledgerID, accountID, id)

	require.Error(t, err)
	assert.Equal(t, constant.ErrAccountExceptionNotFound.Error(), extractCode(t, err))
}

// TestDeleteAccountException_EmitsDeletedEvent: a successful delete publishes one
// account_exception.deleted event carrying the exception identity.
func TestDeleteAccountException_EmitsDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	id := uuid.New()

	mockExceptionRepo := accountexception.NewMockRepository(ctrl)
	mockExceptionRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, accountID, id).
		Return(nil)

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := &UseCase{AccountExceptionRepo: mockExceptionRepo, Streaming: mockEmitter}

	err := uc.DeleteAccountException(context.Background(), organizationID, ledgerID, accountID, id)
	require.NoError(t, err)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account_exception", "deleted")
	assert.Equal(t, "account_exception.deleted", emitted[0].DefinitionKey)
	assert.Equal(t, id.String(), emitted[0].Subject)
}
