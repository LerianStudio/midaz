// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestDeleteAccountByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mocks
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:            mockAccountRepo,
		BalanceRepo:            mockBalanceRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()
	accountID := uuid.New()

	tests := []struct {
		name        string
		portfolioID *uuid.UUID
		setupMocks  func()
		expectedErr error
	}{
		{
			name:        "success - account deleted",
			portfolioID: &portfolioID,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
					Return(&mmodel.Account{ID: accountID.String()}, nil).
					Times(1)

				// DeleteAllBalancesByAccountID calls BalanceRepo.ListByAccountID internally
				mockBalanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
					Return([]*mmodel.Balance{}, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, accountID).
					Return(nil).
					Times(1)
				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAccount, accountID.String()).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name:        "success - metadata soft delete failure does not fail the delete",
			portfolioID: &portfolioID,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
					Return(&mmodel.Account{ID: accountID.String()}, nil).
					Times(1)

				mockBalanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
					Return([]*mmodel.Balance{}, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, accountID).
					Return(nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAccount, accountID.String()).
					Return(errors.New("mongo unavailable")).
					Times(3)
			},
			expectedErr: nil,
		},
		{
			name:        "failure - account not found",
			portfolioID: nil,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)

				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAccount, accountID.String()).
					Times(0)
			},
			expectedErr: errors.New("errDatabaseItemNotFound"),
		},
		{
			name:        "failure - forbidden external account manipulation",
			portfolioID: nil,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
					Return(&mmodel.Account{ID: accountID.String(), Type: "external"}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAccount, accountID.String()).
					Times(0)
			},
			expectedErr: errors.New("0074 - Accounts of type 'external' cannot be deleted or modified as they are used for traceability with external systems. Please review your request and ensure operations are only performed on internal accounts."),
		},
		{
			name:        "failure - delete operation error",
			portfolioID: &portfolioID,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
					Return(&mmodel.Account{ID: accountID.String()}, nil).
					Times(1)

				// DeleteAllBalancesByAccountID calls BalanceRepo.ListByAccountID internally
				mockBalanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
					Return([]*mmodel.Balance{}, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, accountID).
					Return(errors.New("delete error")).
					Times(1)

				mockMetadataRepo.EXPECT().
					Delete(gomock.Any(), constant.EntityAccount, accountID.String()).
					Times(0)
			},
			expectedErr: errors.New("delete error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Configuração dos mocks
			tt.setupMocks()

			// Executa a função
			err := uc.DeleteAccountByID(ctx, organizationID, ledgerID, tt.portfolioID, accountID, "token")

			// Validações
			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDeleteAccountByIDSuccess is responsible to test DeleteAccountByID with success
func TestDeleteAccountByIDSuccess(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	portfolioID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())
	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(nil).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.Nil(t, err)
}

// TestDeleteAccountByIDWithoutPortfolioSuccess is responsible to test DeleteAccountByID without portfolio with success
func TestDeleteAccountByIDWithoutPortfolioSuccess(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())
	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, nil, id).
		Return(nil).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, nil, id)

	assert.Nil(t, err)
}

// TestDeleteAccountByIDError is responsible to test DeleteAccountByID with error
func TestDeleteAccountByIDError(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	portfolioID := uuid.Must(libCommons.GenerateUUIDv7())
	id := uuid.Must(libCommons.GenerateUUIDv7())
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}

// stubInstrumentCascader is a hand-rolled InstrumentCascader stub that records the
// scope it was called with.
type stubInstrumentCascader struct {
	cascaded     int
	err          error
	onCall       func()
	calls        int
	gotOrgID     uuid.UUID
	gotLedgerID  uuid.UUID
	gotAccountID uuid.UUID
}

func (s *stubInstrumentCascader) SoftDeleteInstrumentsByAccount(_ context.Context, organizationID, ledgerID, accountID uuid.UUID) (int, error) {
	s.calls++
	s.gotOrgID = organizationID
	s.gotLedgerID = ledgerID
	s.gotAccountID = accountID

	if s.onCall != nil {
		s.onCall()
	}

	return s.cascaded, s.err
}

// stubFeeAliasDetacher is a hand-rolled FeeAliasDetacher stub that records the
// scope and alias it was called with.
type stubFeeAliasDetacher struct {
	result      model.FeeAliasDetachResult
	err         error
	onCall      func()
	calls       int
	gotOrgID    uuid.UUID
	gotLedgerID uuid.UUID
	gotAlias    string
}

func (s *stubFeeAliasDetacher) DetachAccountAlias(_ context.Context, organizationID, ledgerID uuid.UUID, alias string) (model.FeeAliasDetachResult, error) {
	s.calls++
	s.gotOrgID = organizationID
	s.gotLedgerID = ledgerID
	s.gotAlias = alias

	if s.onCall != nil {
		s.onCall()
	}

	return s.result, s.err
}

// TestDeleteAccountByID_Cascade pins the order and fail-closed posture of the
// account delete cascade: balances, instruments, fee alias detach, then the row.
func TestDeleteAccountByID_Cascade(t *testing.T) {
	organizationID := uuid.MustParse("0191a0b2-0000-7000-8000-000000000001")
	ledgerID := uuid.MustParse("0191a0b2-0000-7000-8000-000000000002")
	accountID := uuid.MustParse("0191a0b2-0000-7000-8000-000000000003")
	alias := "@cascade_account"
	emptyAlias := ""

	errInstruments := errors.New("crm store unavailable")
	errFees := errors.New("fees store unavailable")

	type findOutcome int

	const (
		findInternal findOutcome = iota
		findNotFound
		findExternal
	)

	tests := []struct {
		name            string
		withPorts       bool
		find            findOutcome
		accountAlias    *string
		listBalancesErr error
		cascaded        int
		cascadeErr      error
		feeResult       model.FeeAliasDetachResult
		feeErr          error
		wantInstrCalls  int
		wantFeeCalls    int
		wantRowDelete   bool
		wantErr         error
		wantBusinessErr error
	}{
		{
			name:          "nil ports keep the current flow",
			withPorts:     false,
			find:          findInternal,
			accountAlias:  &alias,
			wantRowDelete: true,
		},
		{
			name:           "ports without effects delete the row",
			withPorts:      true,
			find:           findInternal,
			accountAlias:   &alias,
			wantInstrCalls: 1,
			wantFeeCalls:   1,
			wantRowDelete:  true,
		},
		{
			name:           "one instrument cascaded and no fee package matched",
			withPorts:      true,
			find:           findInternal,
			accountAlias:   &alias,
			cascaded:       1,
			wantInstrCalls: 1,
			wantFeeCalls:   1,
			wantRowDelete:  true,
		},
		{
			name:           "fee packages updated and disabled",
			withPorts:      true,
			find:           findInternal,
			accountAlias:   &alias,
			feeResult:      model.FeeAliasDetachResult{PackagesUpdated: 2, PackagesDisabled: 1},
			wantInstrCalls: 1,
			wantFeeCalls:   1,
			wantRowDelete:  true,
		},
		{
			name:           "instrument cascade error aborts before fees and the row",
			withPorts:      true,
			find:           findInternal,
			accountAlias:   &alias,
			cascadeErr:     errInstruments,
			wantInstrCalls: 1,
			wantErr:        errInstruments,
		},
		{
			name:           "fee detach error aborts before the row",
			withPorts:      true,
			find:           findInternal,
			accountAlias:   &alias,
			cascaded:       1,
			feeErr:         errFees,
			wantInstrCalls: 1,
			wantFeeCalls:   1,
			wantErr:        errFees,
		},
		{
			name:            "account not found touches no port",
			withPorts:       true,
			find:            findNotFound,
			wantBusinessErr: pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount),
		},
		{
			name:            "external account touches no port",
			withPorts:       true,
			find:            findExternal,
			wantBusinessErr: pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, constant.EntityAccount),
		},
		{
			name:            "balance deletion failure touches no port",
			withPorts:       true,
			find:            findInternal,
			accountAlias:    &alias,
			listBalancesErr: errors.New("balance store unavailable"),
			wantBusinessErr: pkg.ValidateBusinessError(constant.ErrAccountBalanceDeletion, constant.EntityAccount),
		},
		{
			name:           "account without alias skips fees",
			withPorts:      true,
			find:           findInternal,
			accountAlias:   nil,
			wantInstrCalls: 1,
			wantRowDelete:  true,
		},
		{
			name:           "account with empty alias skips fees",
			withPorts:      true,
			find:           findInternal,
			accountAlias:   &emptyAlias,
			wantInstrCalls: 1,
			wantRowDelete:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAccountRepo := account.NewMockRepository(ctrl)
			mockBalanceRepo := balance.NewMockRepository(ctrl)

			switch tt.find {
			case findNotFound:
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
					Return(nil, nil).
					Times(1)
			case findExternal:
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
					Return(&mmodel.Account{ID: accountID.String(), Type: "external", Alias: &alias}, nil).
					Times(1)
			default:
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1).
					Return(&mmodel.Account{ID: accountID.String(), Type: "deposit", Alias: tt.accountAlias}, nil).
					Times(1)

				mockBalanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
					Return([]*mmodel.Balance{}, tt.listBalancesErr).
					Times(1)
			}

			mockMetadataRepo := onbMongo.NewMockRepository(ctrl)
			metadataDeletes := 0

			if tt.wantRowDelete {
				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, nil, accountID).
					Return(nil).
					Times(1)

				metadataDeletes = 1
			}

			// The metadata is soft-deleted only once the row is.
			mockMetadataRepo.EXPECT().
				Delete(gomock.Any(), constant.EntityAccount, accountID.String()).
				Return(nil).
				Times(metadataDeletes)

			uc := &UseCase{
				AccountRepo:            mockAccountRepo,
				BalanceRepo:            mockBalanceRepo,
				OnboardingMetadataRepo: mockMetadataRepo,
				metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
			}

			cascader := &stubInstrumentCascader{cascaded: tt.cascaded, err: tt.cascadeErr}
			detacher := &stubFeeAliasDetacher{result: tt.feeResult, err: tt.feeErr}

			if tt.withPorts {
				uc.InstrumentCascader = cascader
				uc.FeeAliasDetacher = detacher
			}

			err := uc.DeleteAccountByID(context.Background(), organizationID, ledgerID, nil, accountID, "token")

			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, tt.wantErr, err, "port errors must propagate unchanged")
			case tt.wantBusinessErr != nil:
				require.Error(t, err)
				assert.Equal(t, tt.wantBusinessErr, err)
			default:
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantInstrCalls, cascader.calls, "instrument cascader calls")
			assert.Equal(t, tt.wantFeeCalls, detacher.calls, "fee alias detacher calls")

			if tt.wantInstrCalls > 0 {
				assert.Equal(t, organizationID, cascader.gotOrgID)
				assert.Equal(t, ledgerID, cascader.gotLedgerID)
				assert.Equal(t, accountID, cascader.gotAccountID)
			}

			if tt.wantFeeCalls > 0 {
				assert.Equal(t, organizationID, detacher.gotOrgID)
				assert.Equal(t, ledgerID, detacher.gotLedgerID)
				assert.Equal(t, alias, detacher.gotAlias)
			}
		})
	}
}
