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

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// newUpdateLedgerUseCase wires a UseCase over fresh mocks for one subtest.
func newUpdateLedgerUseCase(t *testing.T) (*UseCase, *ledger.MockRepository, *mongodb.MockRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ledgerRepo := ledger.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	return &UseCase{LedgerRepo: ledgerRepo, OnboardingMetadataRepo: metadataRepo}, ledgerRepo, metadataRepo
}

// Scenario update-rejeita-nome-duplicado-v1 at the command layer: the name is
// already held by another ledger, so the write must never be attempted.
func TestUpdateLedgerByID_RejectsNameTakenByAnotherLedger(t *testing.T) {
	uc, ledgerRepo, _ := newUpdateLedgerUseCase(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	conflict := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, constant.EntityLedger, "Alpha")

	ledgerRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID).
		Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Beta"}, nil)
	ledgerRepo.EXPECT().
		FindByNameExcludingID(gomock.Any(), organizationID, "Alpha", ledgerID).
		Return(true, conflict)

	result, err := uc.UpdateLedgerByID(context.Background(), organizationID, ledgerID, &mmodel.UpdateLedgerInput{Name: "Alpha"})

	assert.Nil(t, result)
	require.Error(t, err)

	var conflictErr pkg.EntityConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Equal(t, constant.ErrLedgerNameConflict.Error(), conflictErr.Code)
}

// Scenario update-rejeita-nome-duplicado-case-insensitive-v2 at the command
// layer. Case folding lives in the repository predicate; the command must pass
// the requested name through untouched so that folding can happen.
func TestUpdateLedgerByID_RejectsNameTakenCaseInsensitively(t *testing.T) {
	uc, ledgerRepo, _ := newUpdateLedgerUseCase(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	conflict := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, constant.EntityLedger, "alpha")

	ledgerRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID).
		Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Beta"}, nil)
	ledgerRepo.EXPECT().
		FindByNameExcludingID(gomock.Any(), organizationID, "alpha", ledgerID).
		Return(true, conflict)

	_, err := uc.UpdateLedgerByID(context.Background(), organizationID, ledgerID, &mmodel.UpdateLedgerInput{Name: "alpha"})

	var conflictErr pkg.EntityConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Equal(t, constant.ErrLedgerNameConflict.Error(), conflictErr.Code)
}

// Scenario update-sem-campo-name-nao-checa-unicidade: with no name in the
// payload the repository leaves the column alone, so there is nothing to check.
// gomock fails the test if FindByNameExcludingID is called at all.
func TestUpdateLedgerByID_EmptyNameSkipsUniquenessCheck(t *testing.T) {
	uc, ledgerRepo, metadataRepo := newUpdateLedgerUseCase(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()

	ledgerRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Alpha", Status: mmodel.Status{Code: "INACTIVE"}}, nil)
	metadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	result, err := uc.UpdateLedgerByID(context.Background(), organizationID, ledgerID, &mmodel.UpdateLedgerInput{
		Status: mmodel.Status{Code: "INACTIVE"},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "INACTIVE", result.Status.Code)
}

// Scenario update-reenvio-do-proprio-nome-continua-200: the ledger's own row is
// excluded from the candidate set, so the check reports no conflict and the
// update proceeds.
func TestUpdateLedgerByID_OwnNameResentProceeds(t *testing.T) {
	uc, ledgerRepo, metadataRepo := newUpdateLedgerUseCase(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()

	ledgerRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID).
		Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Alpha"}, nil)
	ledgerRepo.EXPECT().
		FindByNameExcludingID(gomock.Any(), organizationID, "Alpha", ledgerID).
		Return(false, nil)
	ledgerRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Alpha"}, nil)
	metadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	result, err := uc.UpdateLedgerByID(context.Background(), organizationID, ledgerID, &mmodel.UpdateLedgerInput{Name: "Alpha"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Alpha", result.Name)
}

// A technical failure of the uniqueness check propagates unchanged: it must not
// be reshaped into a conflict, and the write must not run on an unknown answer.
func TestUpdateLedgerByID_TechnicalFindFailurePropagates(t *testing.T) {
	uc, ledgerRepo, _ := newUpdateLedgerUseCase(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	boom := errors.New("connection reset by peer")

	ledgerRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID).
		Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Alpha"}, nil)
	ledgerRepo.EXPECT().
		FindByNameExcludingID(gomock.Any(), organizationID, "Alpha", ledgerID).
		Return(false, boom)

	result, err := uc.UpdateLedgerByID(context.Background(), organizationID, ledgerID, &mmodel.UpdateLedgerInput{Name: "Alpha"})

	assert.Nil(t, result)
	require.ErrorIs(t, err, boom)
	assert.False(t, pkg.IsBusinessError(err), "a driver failure must stay technical")
}

// Existence outranks uniqueness: a PATCH addressing a ledger that does not
// exist (wrong id, wrong organization, already deleted) must answer 404 even
// when the requested name is held by another ledger. Reaching the uniqueness
// check first would both break REST semantics and turn the endpoint into an
// oracle for which names an organization holds. gomock fails the test if
// FindByNameExcludingID or Update is called at all.
func TestUpdateLedgerByID_MissingLedgerIsNotFoundNotConflict(t *testing.T) {
	uc, ledgerRepo, _ := newUpdateLedgerUseCase(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	notFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)

	ledgerRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID).
		Return(nil, notFound)

	result, err := uc.UpdateLedgerByID(context.Background(), organizationID, ledgerID, &mmodel.UpdateLedgerInput{Name: "Alpha"})

	assert.Nil(t, result)
	require.Error(t, err)

	var notFoundErr pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFoundErr, "a PATCH on a missing ledger must be 404, never 409")
	assert.Equal(t, constant.ErrEntityNotFound.Error(), notFoundErr.Code)

	var conflictErr pkg.EntityConflictError
	assert.False(t, errors.As(err, &conflictErr), "the name conflict must never mask the missing ledger")
}

// A technical failure while resolving the ledger propagates unchanged and stops
// the flow before the uniqueness check.
func TestUpdateLedgerByID_TechnicalFindByIDFailurePropagates(t *testing.T) {
	uc, ledgerRepo, _ := newUpdateLedgerUseCase(t)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	boom := errors.New("connection reset by peer")

	ledgerRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID).
		Return(nil, boom)

	result, err := uc.UpdateLedgerByID(context.Background(), organizationID, ledgerID, &mmodel.UpdateLedgerInput{Name: "Alpha"})

	assert.Nil(t, result)
	require.ErrorIs(t, err, boom)
	assert.False(t, pkg.IsBusinessError(err), "a driver failure must stay technical")
}

// A cancelled caller is answered before any round-trip runs.
func TestUpdateLedgerByID_CancelledContextSkipsRoundTrips(t *testing.T) {
	uc, _, _ := newUpdateLedgerUseCase(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := uc.UpdateLedgerByID(ctx, uuid.New(), uuid.New(), &mmodel.UpdateLedgerInput{Name: "Alpha"})

	assert.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
}
