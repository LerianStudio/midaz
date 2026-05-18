// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// streamingFailingEmitter is a tiny Emitter that returns a publish error
// without recording any state. Used to verify that IMPORTANT-posture emit
// failures do not fail the underlying request.
type streamingFailingEmitter struct{}

func (streamingFailingEmitter) Emit(_ context.Context, _ libStreaming.EmitRequest) error {
	return errors.New("simulated streaming failure")
}

func (streamingFailingEmitter) Close() error                    { return nil }
func (streamingFailingEmitter) Healthy(_ context.Context) error { return nil }

// newStreamingTestUseCase wires a happy-path UseCase suitable for exercising
// the account.created emission. All repositories are gomock-backed and
// preconfigured to accept any call.
func newStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockAssetRepo := asset.NewMockRepository(ctrl)
	mockPortfolioRepo := portfolio.NewMockRepository(ctrl)
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)

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
			out.ID = uuid.New().String()
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

	return &UseCase{
		AssetRepo:              mockAssetRepo,
		PortfolioRepo:          mockPortfolioRepo,
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		AccountTypeRepo:        mockAccountTypeRepo,
		BalanceRepo:            mockBalanceRepo,
		LedgerRepo:             mockLedgerRepo,
		Streaming:              emitter,
	}
}

// TestCreateAccount_EmitsAccountCreatedEvent verifies that a successful
// CreateAccount call publishes exactly one account.created event with the
// expected resource/event types, tenant ID, subject and payload fields.
func TestCreateAccount_EmitsAccountCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newStreamingTestUseCase(t, ctrl, mockEmitter)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreateAccountInput{
		Name:      "Streaming Test Account",
		Type:      "deposit",
		AssetCode: "USD",
	}

	acc, err := uc.CreateAccount(ctx, orgID, ledgerID, input, "Bearer test")
	require.NoError(t, err)
	require.NotNil(t, acc)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "account", "created")

	evt := events[0]
	assert.Equal(t, "account.created", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, acc.ID, evt.Subject, "Subject must be the new account ID")

	// Payload is json.RawMessage — decode and inspect required fields.
	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, acc.ID, payload["id"])
	assert.Equal(t, acc.OrganizationID, payload["organizationId"])
	assert.Equal(t, acc.LedgerID, payload["ledgerId"])
	assert.Equal(t, acc.AssetCode, payload["assetCode"])
	assert.Equal(t, acc.Type, payload["type"])
	assert.NotEmpty(t, payload["createdAt"], "createdAt must be set (RFC3339)")
	assert.NotEmpty(t, payload["updatedAt"], "updatedAt must be set (RFC3339)")
	assert.Contains(t, payload, "status")
	assert.Contains(t, payload, "blocked")
}

// TestCreateAccount_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, CreateAccount
// succeeds without error and no panic.
func TestCreateAccount_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	input := &mmodel.CreateAccountInput{
		Name:      "Noop Test Account",
		Type:      "deposit",
		AssetCode: "USD",
	}

	acc, err := uc.CreateAccount(context.Background(), uuid.New(), uuid.New(), input, "Bearer test")
	require.NoError(t, err)
	require.NotNil(t, acc)
}

// TestCreateAccount_EmitFailureDoesNotFailRequest verifies the IMPORTANT
// posture: when Emit returns an error, CreateAccount must still return the
// successfully-persisted account because durability is owned by PG +
// future DLQ/outbox, not by the synchronous Emit call.
func TestCreateAccount_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	input := &mmodel.CreateAccountInput{
		Name:      "Emit Fail Account",
		Type:      "deposit",
		AssetCode: "USD",
	}

	acc, err := uc.CreateAccount(context.Background(), uuid.New(), uuid.New(), input, "Bearer test")
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, acc)
}

// TestCreateAccount_NilStreamingDoesNotPanic confirms that a UseCase with a
// nil Streaming field (legacy / partial wiring) still completes the request
// — the emit block must be guarded.
func TestCreateAccount_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newStreamingTestUseCase(t, ctrl, nil)

	input := &mmodel.CreateAccountInput{
		Name:      "Nil Streaming Account",
		Type:      "deposit",
		AssetCode: "USD",
	}

	acc, err := uc.CreateAccount(context.Background(), uuid.New(), uuid.New(), input, "Bearer test")
	require.NoError(t, err)
	require.NotNil(t, acc)
}
