// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	crmhttp "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/crm/http"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accountregistration"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// sagaFixture bundles the tiny slice of UseCase dependencies the saga tests actually
// exercise. The saga calls createAccountWithOptions under the hood on the happy path,
// which needs half a dozen more repos — those branches are integration-tested against
// the real createAccountWithOptions path in activate_account_test.go / create_account_test.go
// and a future e2e test, not duplicated here. These tests cover the Phase 4-specific
// state-machine edges.
type sagaFixture struct {
	ctrl          *gomock.Controller
	repo          *accountregistration.MockRepository
	crm           *crmhttp.MockCRMAccountRelationshipPort
	uc            *UseCase
	input         *mmodel.CreateAccountRegistrationInput
	orgID         uuid.UUID
	ledgerID      uuid.UUID
	idempotencyID string
	token         string
}

func newSagaFixture(t *testing.T) *sagaFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	repo := accountregistration.NewMockRepository(ctrl)
	crm := crmhttp.NewMockCRMAccountRelationshipPort(ctrl)

	uc := &UseCase{
		AccountRegistrationRepo: repo,
		CRMClient:               crm,
	}

	return &sagaFixture{
		ctrl:          ctrl,
		repo:          repo,
		crm:           crm,
		uc:            uc,
		input:         newValidSagaInput(),
		orgID:         uuid.New(),
		ledgerID:      uuid.New(),
		idempotencyID: "idem-key-1",
		token:         "Bearer service-token",
	}
}

func newValidSagaInput() *mmodel.CreateAccountRegistrationInput {
	return &mmodel.CreateAccountRegistrationInput{
		HolderID: uuid.New(),
		Account: mmodel.CreateAccountInput{
			Name:      "John Doe Checking",
			Type:      "deposit",
			AssetCode: "USD",
		},
		CRMAlias: mmodel.CreateAliasInput{
			LedgerID:  "placeholder-overridden-by-saga",
			AccountID: "placeholder-overridden-by-saga",
		},
	}
}

// TestCreateAccountRegistration_MissingIdempotencyKey_ReturnsBusinessError ensures
// the saga rejects a call with an empty idempotency key *before* any repo or CRM
// side-effect runs.
func TestCreateAccountRegistration_MissingIdempotencyKey_ReturnsBusinessError(t *testing.T) {
	fx := newSagaFixture(t)

	reg, account, alias, err := fx.uc.CreateAccountRegistration(context.Background(), fx.orgID, fx.ledgerID, fx.input, "", fx.token)

	require.Error(t, err, "empty idempotency key must error")
	assert.True(t, errors.Is(err, constant.ErrIdempotencyKeyRequired), "expected ErrIdempotencyKeyRequired, got %v", err)
	assert.Nil(t, reg)
	assert.Nil(t, account)
	assert.Nil(t, alias)
}

// TestCreateAccountRegistration_NilInput_ReturnsError asserts defensive handling.
func TestCreateAccountRegistration_NilInput_ReturnsError(t *testing.T) {
	fx := newSagaFixture(t)

	reg, account, alias, err := fx.uc.CreateAccountRegistration(context.Background(), fx.orgID, fx.ledgerID, nil, fx.idempotencyID, fx.token)

	require.Error(t, err)
	assert.Nil(t, reg)
	assert.Nil(t, account)
	assert.Nil(t, alias)
}

// TestCreateAccountRegistration_Replay_ShortCircuits covers the idempotent-replay
// path: UpsertByIdempotencyKey returns wasCreated=false for a COMPLETED row, and the
// saga returns it without invoking CRM or ledger-side writes.
func TestCreateAccountRegistration_Replay_ShortCircuits(t *testing.T) {
	fx := newSagaFixture(t)

	completedID := uuid.New()
	accountID := uuid.New()
	aliasID := uuid.New()

	stored := &mmodel.AccountRegistration{
		ID:             completedID,
		OrganizationID: fx.orgID,
		LedgerID:       fx.ledgerID,
		HolderID:       fx.input.HolderID,
		IdempotencyKey: fx.idempotencyID,
		Status:         mmodel.AccountRegistrationCompleted,
		AccountID:      &accountID,
		CRMAliasID:     &aliasID,
	}

	// The repository returns wasCreated=false and the stored COMPLETED record.
	fx.repo.EXPECT().
		UpsertByIdempotencyKey(gomock.Any(), gomock.Any()).
		Return(stored, false, nil).
		Times(1)

	// Replay reloads the account from the AccountRepo and the alias from CRM. Both
	// are optional from the saga's perspective (logged-on-failure, not propagated).
	// Wire mocks so the replay path runs to completion.
	mockAccount := account.NewMockRepository(fx.ctrl)
	fx.uc.AccountRepo = mockAccount

	mockAccount.EXPECT().
		Find(gomock.Any(), fx.orgID, fx.ledgerID, (*uuid.UUID)(nil), accountID).
		Return(&mmodel.Account{ID: accountID.String()}, nil).
		Times(1)

	fx.crm.EXPECT().
		GetAliasByAccount(gomock.Any(), fx.orgID.String(), fx.ledgerID.String(), accountID.String(), fx.token).
		Return(&mmodel.Alias{ID: &aliasID}, nil).
		Times(1)

	// We deliberately do NOT expect GetHolder, CreateAccountAlias, UpdateStatus, etc.
	// The replay must not invoke any further side-effects.

	reg, _, _, err := fx.uc.CreateAccountRegistration(context.Background(), fx.orgID, fx.ledgerID, fx.input, fx.idempotencyID, fx.token)
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.Equal(t, mmodel.AccountRegistrationCompleted, reg.Status)
	assert.Equal(t, completedID, reg.ID)
}

// TestCreateAccountRegistration_HashMismatch_ReturnsIdempotencyError covers the
// case where UpsertByIdempotencyKey surfaces ErrAccountRegistrationIdempotencyConflict
// because the stored hash differs from the caller's current body.
func TestCreateAccountRegistration_HashMismatch_ReturnsIdempotencyError(t *testing.T) {
	fx := newSagaFixture(t)

	conflict := pkg.ValidateBusinessError(constant.ErrAccountRegistrationIdempotencyConflict, constant.EntityAccountRegistration)

	fx.repo.EXPECT().
		UpsertByIdempotencyKey(gomock.Any(), gomock.Any()).
		Return(nil, false, conflict).
		Times(1)

	reg, account, alias, err := fx.uc.CreateAccountRegistration(context.Background(), fx.orgID, fx.ledgerID, fx.input, fx.idempotencyID, fx.token)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrAccountRegistrationIdempotencyConflict), "expected idempotency conflict, got %v", err)
	assert.Nil(t, reg)
	assert.Nil(t, account)
	assert.Nil(t, alias)
}

// TestCreateAccountRegistration_HolderNotFound_MarksTerminal verifies a CRM 404 on
// GetHolder lands the saga in FAILED_TERMINAL via MarkFailed with HOLDER_NOT_FOUND.
func TestCreateAccountRegistration_HolderNotFound_MarksTerminal(t *testing.T) {
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

	fx.repo.EXPECT().
		UpsertByIdempotencyKey(gomock.Any(), gomock.Any()).
		Return(created, true, nil).
		Times(1)

	fx.crm.EXPECT().
		GetHolder(gomock.Any(), fx.orgID.String(), fx.input.HolderID, fx.token).
		Return(nil, pkg.ValidateBusinessError(constant.ErrHolderNotFound, constant.EntityAccountRegistration)).
		Times(1)

	// Persist the failure as FAILED_TERMINAL with reason HOLDER_NOT_FOUND.
	fx.repo.EXPECT().
		MarkFailed(gomock.Any(), regID, mmodel.AccountRegistrationFailedTerminal, "HOLDER_NOT_FOUND", gomock.Any()).
		Return(nil).
		Times(1)

	reg, account, alias, err := fx.uc.CreateAccountRegistration(context.Background(), fx.orgID, fx.ledgerID, fx.input, fx.idempotencyID, fx.token)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrHolderNotFound), "expected holder not found, got %v", err)
	require.NotNil(t, reg)
	assert.Equal(t, mmodel.AccountRegistrationFailedTerminal, reg.Status)
	assert.Nil(t, account)
	assert.Nil(t, alias)
}

// TestCreateAccountRegistration_HolderTransient_MarksRetryable verifies a transient
// CRM error on GetHolder lands the saga in FAILED_RETRYABLE with reason CRM_TRANSIENT.
// The test accepts either status-only persistence or the next_retry_at mutator path
// by using AnyTimes() on UpdateStatus.
func TestCreateAccountRegistration_HolderTransient_MarksRetryable(t *testing.T) {
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

	fx.repo.EXPECT().
		UpsertByIdempotencyKey(gomock.Any(), gomock.Any()).
		Return(created, true, nil).
		Times(1)

	fx.crm.EXPECT().
		GetHolder(gomock.Any(), fx.orgID.String(), fx.input.HolderID, fx.token).
		Return(nil, pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration)).
		Times(1)

	fx.repo.EXPECT().
		MarkFailed(gomock.Any(), regID, mmodel.AccountRegistrationFailedRetryable, "CRM_TRANSIENT", gomock.Any()).
		Return(nil).
		Times(1)

	// The retryable path additionally stamps next_retry_at through UpdateStatus with
	// a mutator. Allow zero or one calls; the important assertion is status + reason.
	fx.repo.EXPECT().
		UpdateStatus(gomock.Any(), regID, mmodel.AccountRegistrationFailedRetryable, gomock.Any()).
		Return(nil).
		AnyTimes()

	reg, account, alias, err := fx.uc.CreateAccountRegistration(context.Background(), fx.orgID, fx.ledgerID, fx.input, fx.idempotencyID, fx.token)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constant.ErrCRMTransient), "expected CRM transient, got %v", err)
	require.NotNil(t, reg)
	assert.Equal(t, mmodel.AccountRegistrationFailedRetryable, reg.Status)
	assert.Nil(t, account)
	assert.Nil(t, alias)
}

// TestClassifySagaError_Matrix covers every branch in the saga's error classifier so
// refactors don't silently change severity.
func TestClassifySagaError_Matrix(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReason string
		wantStatus mmodel.AccountRegistrationStatus
	}{
		{
			name:       "HolderNotFound is terminal",
			err:        pkg.ValidateBusinessError(constant.ErrHolderNotFound, constant.EntityAccountRegistration),
			wantReason: "HOLDER_NOT_FOUND",
			wantStatus: mmodel.AccountRegistrationFailedTerminal,
		},
		{
			name:       "CRM transient is retryable",
			err:        pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration),
			wantReason: "CRM_TRANSIENT",
			wantStatus: mmodel.AccountRegistrationFailedRetryable,
		},
		{
			name:       "Alias/holder conflict is terminal",
			err:        pkg.ValidateBusinessError(constant.ErrAliasHolderConflict, constant.EntityAccountRegistration),
			wantReason: "CRM_CONFLICT",
			wantStatus: mmodel.AccountRegistrationFailedTerminal,
		},
		{
			name:       "CRM bad request is terminal",
			err:        pkg.ValidateBusinessError(constant.ErrCRMBadRequest, constant.EntityAccountRegistration),
			wantReason: "CRM_BAD_REQUEST",
			wantStatus: mmodel.AccountRegistrationFailedTerminal,
		},
		{
			name:       "unknown error defaults to retryable",
			err:        errors.New("boom"),
			wantReason: "CRM_TRANSIENT",
			wantStatus: mmodel.AccountRegistrationFailedRetryable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, status := classifySagaError(tc.err)
			assert.Equal(t, tc.wantReason, reason)
			assert.Equal(t, tc.wantStatus, status)
		})
	}
}
