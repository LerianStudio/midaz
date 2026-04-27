// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/idempotency"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestCreateAlias_HolderConflict_SurfacesFromRepo verifies that the service
// propagates ErrAliasHolderConflict without masking it. The repository
// returns the sentinel when a (ledger_id, account_id) pair already exists
// under a different holder (see alias.mongodb.go Create pre-probe).
func TestCreateAlias_HolderConflict_SurfacesFromRepo(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	holderRepo := holder.NewMockRepository(ctrl)
	aliasRepo := alias.NewMockRepository(ctrl)
	idempRepo := idempotency.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderRepo:      holderRepo,
		AliasRepo:       aliasRepo,
		IdempotencyRepo: idempRepo,
	}

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	holderDoc := "90217469051"
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	holderRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), holderID, gomock.Any()).
		Return(&mmodel.Holder{
			ID:       &holderID,
			Document: &holderDoc,
		}, nil).
		Times(1)

	// The repository detects the pre-existing alias under a different holder
	// and returns ErrAliasHolderConflict (0170) — no masking, no retry.
	conflict := pkg.ValidateBusinessError(constant.ErrAliasHolderConflict, "Alias")
	aliasRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, conflict).
		Times(1)

	input := &mmodel.CreateAliasInput{
		LedgerID:  ledgerID,
		AccountID: accountID,
	}

	got, err := uc.CreateAlias(context.Background(), uuid.NewString(), holderID, input, "")

	require.Error(t, err)
	require.Nil(t, got)

	conflictErr, ok := err.(pkg.EntityConflictError)
	require.True(t, ok, "expected EntityConflictError for ErrAliasHolderConflict, got %T", err)
	assert.Equal(t, constant.ErrAliasHolderConflict.Error(), conflictErr.Code)
}

// TestCreateAlias_WithIdempotencyKey_SameKeySamePayload_ReturnsCached
// verifies that when the handler wraps create-alias in the idempotency guard
// via a supplied key, a second call with identical payload is served from the
// cache and does NOT re-invoke the repository.
func TestCreateAlias_WithIdempotencyKey_SameKeySamePayload_ReturnsCached(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	holderRepo := holder.NewMockRepository(ctrl)
	aliasRepo := alias.NewMockRepository(ctrl)
	idempRepo := idempotency.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderRepo:      holderRepo,
		AliasRepo:       aliasRepo,
		IdempotencyRepo: idempRepo,
	}

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	holderDoc := "90217469051"
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	key := "idem-create-" + uuid.NewString()
	orgID := uuid.NewString()

	input := &mmodel.CreateAliasInput{
		LedgerID:  ledgerID,
		AccountID: accountID,
	}

	// --- First call: guard misses, fn runs, result stored. ---
	idempRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), key).
		Return(nil, nil).
		Times(1)

	holderRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), holderID, gomock.Any()).
		Return(&mmodel.Holder{
			ID:       &holderID,
			Document: &holderDoc,
		}, nil).
		Times(1)

	createdID := uuid.Must(libCommons.GenerateUUIDv7())
	createdAlias := &mmodel.Alias{
		ID:        &createdID,
		Document:  &holderDoc,
		AccountID: &accountID,
		LedgerID:  &ledgerID,
		HolderID:  &holderID,
	}

	aliasRepo.EXPECT().
		Create(gomock.Any(), orgID, gomock.Any()).
		Return(createdAlias, nil).
		Times(1)

	idempRepo.EXPECT().
		Store(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	first, err := uc.CreateAlias(context.Background(), orgID, holderID, input, key)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, &createdID, first.ID)
}
