//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// laggingReplicaRepo serves the package read from a snapshot taken before the last
// write: the answer a secondary still behind the primary gives.
type laggingReplicaRepo struct {
	pack.Repository
	snapshot *model.AmountData
}

func (r *laggingReplicaRepo) FindFeesAndAmountDataByPackageID(context.Context, uuid.UUID, uuid.UUID) (*model.AmountData, error) {
	return r.snapshot, nil
}

// A PATCH judged on a lagging replica's read, which shows one fee where the package now
// holds two, would disable a package that keeps a fee. It answers the retryable race
// error instead, and the stored package keeps both fees, enabled and unstamped.
func TestIntegration_UpdatePackage_StaleReadWritesNothing(t *testing.T) {
	_, repo := newLivePackageUseCase(t)
	ctx := context.Background()
	orgID := uuid.New()

	stored := &pack.Package{
		ID:             uuid.New(),
		FeeGroupLabel:  "Pacote Padrao",
		LedgerID:       uuid.New(),
		MinimumAmount:  decimal.RequireFromString("100"),
		MaximumAmount:  decimal.RequireFromString("1000"),
		Fees:           storedDeductibleFlatFee("25"),
		Enable:         boolPtr(true),
		WaivedAccounts: &[]string{},
		CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	_, err := repo.Create(ctx, stored, orgID)
	require.NoError(t, err)

	snapshot, err := repo.FindFeesAndAmountDataByPackageID(ctx, orgID, stored.ID)
	require.NoError(t, err)

	secondFee := storedDeductibleFlatFee("10")["fee1"]
	secondFee.Priority = 2
	added, err := pack.FromEntityFeeMap(map[string]model.Fee{"fee2": secondFee})
	require.NoError(t, err)

	moved := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)
	_, err = repo.Update(ctx, stored.ID, orgID, uuid.Nil, snapshot.UpdatedAt,
		&bson.M{"$set": bson.M{"fees.fee2": added["fee2"], "updated_at": moved}})
	require.NoError(t, err)

	lagging := &UseCase{packageRepo: &laggingReplicaRepo{Repository: repo, snapshot: snapshot}}
	err = lagging.UpdatePackageByID(ctx, stored.ID, orgID, uuid.Nil,
		&model.UpdatePackageInput{Fee: map[string]model.Fee{"fee1": {}}})

	var race pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &race)
	require.Equal(t, constant.ErrLockVersionAccountBalance.Error(), race.Code)

	after, err := repo.FindByID(ctx, stored.ID, orgID, uuid.Nil)
	require.NoError(t, err)
	require.Len(t, after.Fees, 2, "the refused PATCH must leave both fees")
	require.True(t, *after.Enable, "the refused PATCH must leave the package enabled")
	require.True(t, moved.Equal(after.UpdatedAt), "the refused PATCH must not stamp the document")
}
