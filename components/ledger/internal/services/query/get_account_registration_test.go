// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accountregistration"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestGetAccountRegistration_Success confirms the happy path: the query delegates to
// FindByID with the (organizationID, ledgerID, id) tuple and returns the saga record
// untouched.
func TestGetAccountRegistration_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := accountregistration.NewMockRepository(ctrl)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	regID := uuid.New()
	holderID := uuid.New()

	stored := &mmodel.AccountRegistration{
		ID:             regID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		HolderID:       holderID,
		IdempotencyKey: "idem-stored",
		Status:         mmodel.AccountRegistrationCompleted,
	}

	repo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, regID).
		Return(stored, nil).
		Times(1)

	uc := &UseCase{AccountRegistrationRepo: repo}

	got, err := uc.GetAccountRegistration(context.Background(), organizationID, ledgerID, regID)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, stored, got, "the query is a thin pass-through; the stored record must round-trip unchanged")
}

// TestGetAccountRegistration_NotFound_PropagatesBusinessError verifies the not-found
// error returned by the repository (a typed pkg business error) flows back to the caller
// untouched. The query layer must not wrap business errors — they need to surface as 404
// at the HTTP boundary with their original code.
func TestGetAccountRegistration_NotFound_PropagatesBusinessError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := accountregistration.NewMockRepository(ctrl)

	notFound := pkg.ValidateBusinessError(constant.ErrAccountRegistrationNotFound, constant.EntityAccountRegistration)

	repo.EXPECT().
		FindByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, notFound).
		Times(1)

	uc := &UseCase{AccountRegistrationRepo: repo}

	got, err := uc.GetAccountRegistration(context.Background(), uuid.New(), uuid.New(), uuid.New())

	require.Error(t, err)
	assert.Nil(t, got)

	var notFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFoundErr, "expected EntityNotFoundError, got %T", err)
	assert.Equal(t, constant.ErrAccountRegistrationNotFound.Error(), notFoundErr.Code)
}

// TestGetAccountRegistration_RepositoryError_PropagatesAsIs covers the technical-error
// path: a generic repository failure (e.g. driver-level) is returned unwrapped, matching
// the behaviour of every sibling query (see GetAccount, GetLedger). Wrapping at this
// layer would obscure the call site for incident response.
func TestGetAccountRegistration_RepositoryError_PropagatesAsIs(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := accountregistration.NewMockRepository(ctrl)

	boom := errors.New("driver: connection reset")

	repo.EXPECT().
		FindByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, boom).
		Times(1)

	uc := &UseCase{AccountRegistrationRepo: repo}

	got, err := uc.GetAccountRegistration(context.Background(), uuid.New(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, boom, "technical errors must surface with the original cause intact")
	assert.Nil(t, got)
}
