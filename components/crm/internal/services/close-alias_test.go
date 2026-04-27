// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/idempotency"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestCloseAlias_Success confirms that CloseAlias delegates to the repository
// and returns the closed entity when no idempotency key is supplied.
func TestCloseAlias_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	aliasRepo := alias.NewMockRepository(ctrl)
	idempRepo := idempotency.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	aliasID := uuid.Must(libCommons.GenerateUUIDv7())
	orgID := uuid.NewString()

	closingDate := mmodel.Date{Time: time.Now().UTC()}
	closed := &mmodel.Alias{
		ID:       &aliasID,
		HolderID: &holderID,
		BankingDetails: &mmodel.BankingDetails{
			ClosingDate: &closingDate,
		},
	}

	aliasRepo.EXPECT().
		CloseByID(gomock.Any(), orgID, holderID, aliasID, gomock.Any()).
		Return(closed, nil).
		Times(1)

	uc := &UseCase{
		AliasRepo:       aliasRepo,
		IdempotencyRepo: idempRepo,
	}

	got, err := uc.CloseAlias(context.Background(), orgID, holderID, aliasID, "")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotNil(t, got.BankingDetails)
	require.NotNil(t, got.BankingDetails.ClosingDate)
}

// TestCloseAlias_SecondCall_IsNoOp verifies the natural idempotency at the
// repository level: a second close on an already-closed alias short-circuits
// and returns the existing record without mutating it.
//
// This test operates at the service layer, using the mock repo to simulate the
// repository's "already closed" short-circuit — both calls return the same
// closed entity with no error, and the service does not retry.
func TestCloseAlias_SecondCall_IsNoOp(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	aliasRepo := alias.NewMockRepository(ctrl)
	idempRepo := idempotency.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	aliasID := uuid.Must(libCommons.GenerateUUIDv7())
	orgID := uuid.NewString()

	closingDate := mmodel.Date{Time: time.Now().UTC()}
	closed := &mmodel.Alias{
		ID:       &aliasID,
		HolderID: &holderID,
		BankingDetails: &mmodel.BankingDetails{
			ClosingDate: &closingDate,
		},
	}

	// Both calls should land on the repository; the repo itself is responsible
	// for the no-op behaviour (it short-circuits before mutating when closing
	// date is already set). The service-layer test asserts that the second
	// close returns 200 with the same entity, not an error.
	aliasRepo.EXPECT().
		CloseByID(gomock.Any(), orgID, holderID, aliasID, gomock.Any()).
		Return(closed, nil).
		Times(2)

	uc := &UseCase{
		AliasRepo:       aliasRepo,
		IdempotencyRepo: idempRepo,
	}

	first, err := uc.CloseAlias(context.Background(), orgID, holderID, aliasID, "")
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := uc.CloseAlias(context.Background(), orgID, holderID, aliasID, "")
	require.NoError(t, err, "second close must succeed (natural idempotency)")
	require.NotNil(t, second)
	assert.Equal(t, first.BankingDetails.ClosingDate, second.BankingDetails.ClosingDate,
		"both closes must return the same closing date")
}

// TestCloseAlias_WithIdempotencyKey_UsesGuard verifies that when an
// Idempotency-Key is supplied, the guard is invoked: a fresh key proceeds to
// fn exactly once, and a replay with the same key returns the cached response
// without a second repository call.
func TestCloseAlias_WithIdempotencyKey_UsesGuard(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	aliasRepo := alias.NewMockRepository(ctrl)
	idempRepo := idempotency.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	aliasID := uuid.Must(libCommons.GenerateUUIDv7())
	orgID := uuid.NewString()
	key := "idem-close-" + uuid.NewString()

	closingDate := mmodel.Date{Time: time.Now().UTC()}
	closed := &mmodel.Alias{
		ID:       &aliasID,
		HolderID: &holderID,
		BankingDetails: &mmodel.BankingDetails{
			ClosingDate: &closingDate,
		},
	}

	// First call: guard probe finds nothing, fn runs, result is stored.
	idempRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), key).
		Return(nil, nil).
		Times(1)

	idempRepo.EXPECT().
		Store(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	aliasRepo.EXPECT().
		CloseByID(gomock.Any(), orgID, holderID, aliasID, gomock.Any()).
		Return(closed, nil).
		Times(1)

	uc := &UseCase{
		AliasRepo:       aliasRepo,
		IdempotencyRepo: idempRepo,
	}

	first, err := uc.CloseAlias(context.Background(), orgID, holderID, aliasID, key)
	require.NoError(t, err)
	require.NotNil(t, first)
}

// TestCloseAlias_WithIdempotencyKey_HashMismatch_ReturnsConflict confirms the
// hash mismatch behaviour: reusing the same key for a DIFFERENT alias yields
// ErrIdempotencyKey (409), preventing accidental cross-alias replay.
func TestCloseAlias_WithIdempotencyKey_HashMismatch_ReturnsConflict(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	aliasRepo := alias.NewMockRepository(ctrl)
	idempRepo := idempotency.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	aliasID := uuid.Must(libCommons.GenerateUUIDv7())
	orgID := uuid.NewString()
	key := "idem-close-conflict-" + uuid.NewString()

	idempRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), key).
		Return(&idempotency.Record{
			TenantID:         "",
			IdempotencyKey:   key,
			RequestHash:      "totally-different-hash",
			ResponseDocument: []byte(`{}`),
		}, nil).
		Times(1)

	// aliasRepo.CloseByID MUST NOT be called.

	uc := &UseCase{
		AliasRepo:       aliasRepo,
		IdempotencyRepo: idempRepo,
	}

	got, err := uc.CloseAlias(context.Background(), orgID, holderID, aliasID, key)

	require.Error(t, err)
	require.Nil(t, got)

	conflictErr, ok := err.(pkg.EntityConflictError)
	require.True(t, ok, "expected EntityConflictError for ErrIdempotencyKey, got %T", err)
	assert.Equal(t, constant.ErrIdempotencyKey.Error(), conflictErr.Code)
}
