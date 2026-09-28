// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestGetCrossLedgerPolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	enabledLedgerID := uuid.New()
	disabledLedgerID := uuid.New()
	enabledRef := LedgerRef{OrganizationID: organizationID, LedgerID: enabledLedgerID}
	disabledRef := LedgerRef{OrganizationID: organizationID, LedgerID: disabledLedgerID}

	t.Run("returns every enabled ledger policy and deduplicates references", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		repo := ledger.NewMockRepository(ctrl)
		repo.EXPECT().GetSettings(gomock.Any(), organizationID, enabledLedgerID).Return(map[string]any{
			"crossLedger": map[string]any{"enabled": true},
		}, nil).Times(1)

		policies, err := (&UseCase{LedgerRepo: repo}).GetCrossLedgerPolicy(ctx, []LedgerRef{enabledRef, enabledRef})
		require.NoError(t, err)
		assert.Equal(t, map[LedgerRef]bool{enabledRef: true}, map[LedgerRef]bool{enabledRef: policies[enabledRef].Enabled})
	})

	t.Run("rejects the first disabled ledger", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		repo := ledger.NewMockRepository(ctrl)
		repo.EXPECT().GetSettings(gomock.Any(), organizationID, enabledLedgerID).Return(map[string]any{
			"crossLedger": map[string]any{"enabled": true},
		}, nil)
		repo.EXPECT().GetSettings(gomock.Any(), organizationID, disabledLedgerID).Return(map[string]any{}, nil)

		_, err := (&UseCase{LedgerRepo: repo}).GetCrossLedgerPolicy(ctx, []LedgerRef{enabledRef, disabledRef})
		require.Error(t, err)
		assert.Contains(t, err.Error(), constant.ErrCrossLedgerNotEnabled.Error())
		assert.Contains(t, err.Error(), disabledLedgerID.String())
	})

	t.Run("propagates a missing ledger without changing it into a policy error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		repo := ledger.NewMockRepository(ctrl)
		notFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)
		repo.EXPECT().GetSettings(gomock.Any(), organizationID, enabledLedgerID).Return(nil, notFound)

		_, err := (&UseCase{LedgerRepo: repo}).GetCrossLedgerPolicy(ctx, []LedgerRef{enabledRef})
		require.Error(t, err)
		assert.Equal(t, notFound, err)
	})
}
