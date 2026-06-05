// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// stubSettingsReader is a hand-rolled SettingsReader stub for the RequireHolder gate.
type stubSettingsReader struct {
	requireHolder bool
	err           error
}

func (s stubSettingsReader) GetParsedLedgerSettings(_ context.Context, _, _ uuid.UUID) (mmodel.LedgerSettings, error) {
	if s.err != nil {
		return mmodel.LedgerSettings{}, s.err
	}

	settings := mmodel.DefaultLedgerSettings()
	settings.Accounting.RequireHolder = s.requireHolder

	return settings, nil
}

// stubHolderReader is a hand-rolled HolderReader stub that records the org ID it
// was called with (R41: prove the org ID flows through the port).
type stubHolderReader struct {
	exists      bool
	err         error
	calls       int
	gotOrgID    string
	gotHolderID uuid.UUID
}

func (s *stubHolderReader) Exists(_ context.Context, organizationID string, id uuid.UUID) (bool, error) {
	s.calls++
	s.gotOrgID = organizationID
	s.gotHolderID = id

	return s.exists, s.err
}

// setupHolderAccountTest wires the create-account dependencies that the holder
// branches exercise. The returned capture pointer receives the account passed to
// AccountRepo.Create so the materialised HolderID can be asserted.
func setupHolderAccountTest(ctrl *gomock.Controller, captured **mmodel.Account) (*UseCase, *stubHolderReader, *stubSettingsReader) {
	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	// applyAccountingValidations reads settings via the uncached LedgerRepo; nil
	// settings parse to defaults (ValidateAccountType false), so the account-type
	// branch is skipped and the holder branches under test run unobstructed.
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	mockAssetRepo.EXPECT().
		FindByNameOrCode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).AnyTimes()

	mockAccountRepo.EXPECT().
		FindByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	mockAccountRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Account) (*mmodel.Account, error) {
			out := *in
			*captured = &out

			return &out, nil
		}).AnyTimes()

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	mockBalanceRepo.EXPECT().
		ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	holderReader := &stubHolderReader{}
	settingsReader := &stubSettingsReader{}

	uc := &UseCase{
		AssetRepo:              mockAssetRepo,
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		BalanceRepo:            mockBalanceRepo,
		LedgerRepo:             mockLedgerRepo,
		HolderReader:           holderReader,
		SettingsReader:         settingsReader,
	}

	return uc, holderReader, settingsReader
}

// TestCreateAccountSelfHolderDefault covers the T08 default-materialisation split:
// non-external accounts without a HolderID default to the org self-holder; external
// accounts stay unowned; an explicit HolderID is preserved verbatim.
func TestCreateAccountSelfHolderDefault(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	expectedSelf := deriveSelfHolderID(organizationID).String()
	explicitHolder := uuid.New().String()

	tests := []struct {
		name           string
		input          *mmodel.CreateAccountInput
		expectedHolder *string
	}{
		{
			name:           "non-external defaults to org self-holder",
			input:          &mmodel.CreateAccountInput{Name: "A", Type: "deposit", AssetCode: "USD"},
			expectedHolder: &expectedSelf,
		},
		{
			name:           "external stays unowned",
			input:          &mmodel.CreateAccountInput{Name: "B", Type: "external", AssetCode: "USD"},
			expectedHolder: nil,
		},
		{
			name:           "EXTERNAL is case-insensitive and stays unowned",
			input:          &mmodel.CreateAccountInput{Name: "C", Type: "EXTERNAL", AssetCode: "USD"},
			expectedHolder: nil,
		},
		{
			name:           "explicit holder id is preserved",
			input:          &mmodel.CreateAccountInput{Name: "D", Type: "deposit", AssetCode: "USD", HolderID: &explicitHolder},
			expectedHolder: &explicitHolder,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var captured *mmodel.Account
			uc, _, _ := setupHolderAccountTest(ctrl, &captured)

			acc, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input, "Bearer test")
			require.NoError(t, err)
			require.NotNil(t, acc)
			require.NotNil(t, captured)

			if tt.expectedHolder == nil {
				assert.Nil(t, captured.HolderID)
			} else {
				require.NotNil(t, captured.HolderID)
				assert.Equal(t, *tt.expectedHolder, *captured.HolderID)
			}
		})
	}
}

// TestCreateAccountRequireHolderGate covers the T05 gate: it only fires when
// RequireHolder is true AND an explicit HolderID is supplied; a missing holder maps
// to ErrHolderNotFound; RequireHolder=false skips the existence check entirely.
func TestCreateAccountRequireHolderGate(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	explicitHolder := uuid.New().String()

	tests := []struct {
		name                 string
		requireHolder        bool
		input                *mmodel.CreateAccountInput
		holderExists         bool
		expectHolderNotFound bool
		expectChecked        bool
	}{
		{
			name:                 "require true, holder set, holder missing -> ErrHolderNotFound",
			requireHolder:        true,
			input:                &mmodel.CreateAccountInput{Name: "A", Type: "deposit", AssetCode: "USD", HolderID: &explicitHolder},
			holderExists:         false,
			expectHolderNotFound: true,
			expectChecked:        true,
		},
		{
			name:          "require true, holder set, holder exists -> success",
			requireHolder: true,
			input:         &mmodel.CreateAccountInput{Name: "B", Type: "deposit", AssetCode: "USD", HolderID: &explicitHolder},
			holderExists:  true,
			expectChecked: true,
		},
		{
			name:          "require false skips the existence check",
			requireHolder: false,
			input:         &mmodel.CreateAccountInput{Name: "C", Type: "deposit", AssetCode: "USD", HolderID: &explicitHolder},
			holderExists:  false,
			expectChecked: false,
		},
		{
			name:          "require true but no input holder -> default path, no check",
			requireHolder: true,
			input:         &mmodel.CreateAccountInput{Name: "D", Type: "deposit", AssetCode: "USD"},
			holderExists:  false,
			expectChecked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var captured *mmodel.Account
			uc, holderReader, settingsReader := setupHolderAccountTest(ctrl, &captured)
			settingsReader.requireHolder = tt.requireHolder
			holderReader.exists = tt.holderExists

			_, err := uc.CreateAccount(ctx, organizationID, ledgerID, tt.input, "Bearer test")

			if tt.expectHolderNotFound {
				require.Error(t, err)

				var notFound pkg.EntityNotFoundError
				require.ErrorAs(t, err, &notFound)
				assert.Equal(t, constant.ErrHolderNotFound.Error(), notFound.Code)
				assert.Equal(t, constant.EntityHolder, notFound.EntityType)
			} else {
				require.NoError(t, err)
			}

			if tt.expectChecked {
				assert.Equal(t, 1, holderReader.calls)
				// R41: the org ID must flow through the port unchanged.
				assert.Equal(t, organizationID.String(), holderReader.gotOrgID)
				assert.Equal(t, explicitHolder, holderReader.gotHolderID.String())
			} else {
				assert.Equal(t, 0, holderReader.calls)
			}
		})
	}
}

// TestCreateAccountRequireHolderReaderError ensures a HolderReader transport error
// propagates as a request failure (the gate does not silently pass it).
func TestCreateAccountRequireHolderReaderError(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	explicitHolder := uuid.New().String()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var captured *mmodel.Account
	uc, holderReader, settingsReader := setupHolderAccountTest(ctrl, &captured)
	settingsReader.requireHolder = true
	holderReader.err = errors.New("mongo down")

	_, err := uc.CreateAccount(ctx, organizationID, ledgerID,
		&mmodel.CreateAccountInput{Name: "A", Type: "deposit", AssetCode: "USD", HolderID: &explicitHolder},
		"Bearer test")

	require.Error(t, err)
	assert.Equal(t, 1, holderReader.calls)
}

// TestRequireHolderEnabledFallback proves the cached read degrades to permissive
// (false) when the settings reader is unwired or errors.
func TestRequireHolderEnabledFallback(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	t.Run("nil reader -> false", func(t *testing.T) {
		uc := &UseCase{}
		assert.False(t, uc.requireHolderEnabled(ctx, organizationID, ledgerID))
	})

	t.Run("reader error -> false", func(t *testing.T) {
		uc := &UseCase{SettingsReader: &stubSettingsReader{err: errors.New("boom")}}
		assert.False(t, uc.requireHolderEnabled(ctx, organizationID, ledgerID))
	})

	t.Run("reader true -> true", func(t *testing.T) {
		uc := &UseCase{SettingsReader: &stubSettingsReader{requireHolder: true}}
		assert.True(t, uc.requireHolderEnabled(ctx, organizationID, ledgerID))
	})
}

// TestSelfHolderIDDeterministic locks the derivation: same org ID -> same self-holder
// ID, different org IDs -> different IDs, and the result is a valid UUIDv5.
func TestSelfHolderIDDeterministic(t *testing.T) {
	orgA := uuid.New()
	orgB := uuid.New()

	first := deriveSelfHolderID(orgA)
	second := deriveSelfHolderID(orgA)

	assert.Equal(t, first, second, "derivation must be deterministic for the same org")
	assert.NotEqual(t, first, deriveSelfHolderID(orgB), "different orgs derive different self-holders")
	assert.Equal(t, uuid.Version(5), first.Version(), "self-holder ID must be a UUIDv5")
}
