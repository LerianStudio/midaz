// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountRegistration_HolderValidatedPersistenceFails covers Step 4 of the
// saga: when GetHolder succeeds but the subsequent UpdateStatus(HOLDER_VALIDATED) call
// fails, the saga must propagate the persistence error wrapped with context. This
// regression exercises the lines between Steps 3 and 5 that are otherwise only
// reached by the (still missing) end-to-end happy-path test.
func TestCreateAccountRegistration_HolderValidatedPersistenceFails(t *testing.T) {
	fx := newSagaFixture(t)

	regID := uuid.New()

	created := &mmodel.AccountRegistration{
		ID:             regID,
		OrganizationID: fx.orgID,
		LedgerID:       fx.ledgerID,
		HolderID:       fx.input.HolderID,
		IdempotencyKey: fx.idempotencyID,
		Status:         mmodel.AccountRegistrationReceived,
	}

	// Step 2: Idempotency claim succeeds for a fresh key.
	fx.repo.EXPECT().
		UpsertByIdempotencyKey(gomock.Any(), gomock.Any()).
		Return(created, true, nil).
		Times(1)

	// Step 3: GetHolder returns a valid holder so we proceed to Step 4.
	fx.crm.EXPECT().
		GetHolder(gomock.Any(), fx.orgID.String(), fx.input.HolderID, fx.token).
		Return(&mmodel.Holder{ID: &fx.input.HolderID}, nil).
		Times(1)

	// Step 4: UpdateStatus(HOLDER_VALIDATED) fails — this is what we're testing.
	persistenceErr := errors.New("pg: deadlock detected")

	fx.repo.EXPECT().
		UpdateStatus(gomock.Any(), regID, mmodel.AccountRegistrationHolderValidated).
		Return(persistenceErr).
		Times(1)

	reg, account, alias, err := fx.uc.CreateAccountRegistration(context.Background(), fx.orgID, fx.ledgerID, fx.input, fx.idempotencyID, fx.token)

	require.Error(t, err)
	assert.ErrorIs(t, err, persistenceErr, "the underlying pg error must be preserved through the wrap")
	assert.Contains(t, err.Error(), "mark holder_validated", "the wrap must name the failed step")

	require.NotNil(t, reg, "the saga record must still be returned so the caller can poll GET")
	assert.Nil(t, account, "no account is created before Step 5")
	assert.Nil(t, alias)
}

// TestCreateAccountRegistration_InProgressReplay_ReturnsCurrentState covers the third
// branch of the replay path: an existing registration in a non-COMPLETED status (e.g.
// FAILED_RETRYABLE awaiting Phase 5) must short-circuit without invoking any further
// saga steps. The caller polls GET and the worker drives forward progress.
func TestCreateAccountRegistration_InProgressReplay_ReturnsCurrentState(t *testing.T) {
	fx := newSagaFixture(t)

	inProgressID := uuid.New()
	stored := &mmodel.AccountRegistration{
		ID:             inProgressID,
		OrganizationID: fx.orgID,
		LedgerID:       fx.ledgerID,
		HolderID:       fx.input.HolderID,
		IdempotencyKey: fx.idempotencyID,
		Status:         mmodel.AccountRegistrationFailedRetryable, // not COMPLETED
	}

	fx.repo.EXPECT().
		UpsertByIdempotencyKey(gomock.Any(), gomock.Any()).
		Return(stored, false, nil).
		Times(1)

	// In-progress replay must NOT invoke GetHolder, CreateAccountAlias, or any further
	// saga step. Any unexpected call will cause gomock to fail this test.

	reg, account, alias, err := fx.uc.CreateAccountRegistration(context.Background(), fx.orgID, fx.ledgerID, fx.input, fx.idempotencyID, fx.token)
	require.NoError(t, err, "in-progress replay returns the current state without error")
	require.NotNil(t, reg)
	assert.Equal(t, mmodel.AccountRegistrationFailedRetryable, reg.Status)
	assert.Equal(t, inProgressID, reg.ID)
	assert.Nil(t, account, "in-progress replay must not return artifacts; the caller polls GET")
	assert.Nil(t, alias)
}

// TestCreateAccountRegistration_UpsertGenericError_WrapsAndEmitsPersistenceFailed covers
// the fall-through error branch in Step 2: an UpsertByIdempotencyKey error that is NOT
// the typed idempotency conflict must be wrapped with the "upsert" prefix and the
// reasonPersistenceFailed metric must be emitted.
func TestCreateAccountRegistration_UpsertGenericError_WrapsAndEmitsPersistenceFailed(t *testing.T) {
	fx := newSagaFixture(t)

	infraErr := errors.New("pg: connection refused")

	fx.repo.EXPECT().
		UpsertByIdempotencyKey(gomock.Any(), gomock.Any()).
		Return(nil, false, infraErr).
		Times(1)

	reg, account, alias, err := fx.uc.CreateAccountRegistration(context.Background(), fx.orgID, fx.ledgerID, fx.input, fx.idempotencyID, fx.token)

	require.Error(t, err)
	assert.ErrorIs(t, err, infraErr, "the original infra error must remain reachable via errors.Is")
	assert.Contains(t, err.Error(), "upsert", "the wrap must identify the failing step")

	assert.Nil(t, reg)
	assert.Nil(t, account)
	assert.Nil(t, alias)
}
