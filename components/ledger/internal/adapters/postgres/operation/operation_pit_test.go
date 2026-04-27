// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperationPointInTimeModel_ToEntity covers the lightweight point-in-time → entity
// translation used by balance reconstruction queries. The model exists specifically to
// keep the index-only-scan path narrow, so any field added in production without a
// corresponding ToEntity update would silently drop balance state — this test surfaces
// that drift.
func TestOperationPointInTimeModel_ToEntity(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	available := decimal.NewFromInt(1000)
	onHold := decimal.NewFromInt(50)
	version := int64(7)

	model := &OperationPointInTimeModel{
		ID:                    "op-id-pit",
		BalanceID:             "bal-id-pit",
		AccountID:             "acc-id-pit",
		AssetCode:             "USD",
		BalanceKey:            "default",
		AvailableBalanceAfter: &available,
		OnHoldBalanceAfter:    &onHold,
		VersionBalanceAfter:   &version,
		CreatedAt:             createdAt,
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Equal(t, "op-id-pit", entity.ID)
	assert.Equal(t, "bal-id-pit", entity.BalanceID)
	assert.Equal(t, "acc-id-pit", entity.AccountID)
	assert.Equal(t, "USD", entity.AssetCode)
	assert.Equal(t, "default", entity.BalanceKey)
	assert.Equal(t, createdAt, entity.CreatedAt)

	require.NotNil(t, entity.BalanceAfter.Available)
	assert.True(t, available.Equal(*entity.BalanceAfter.Available))

	require.NotNil(t, entity.BalanceAfter.OnHold)
	assert.True(t, onHold.Equal(*entity.BalanceAfter.OnHold))

	require.NotNil(t, entity.BalanceAfter.Version)
	assert.Equal(t, int64(7), *entity.BalanceAfter.Version)
}

// TestOperationPointInTimeModel_ToEntity_NilDecimals exercises the nil-pointer
// preservation: when the row carries NULL balances (e.g., for an annotation operation),
// the entity must propagate nil rather than zero-value Decimals. Otherwise a zero would
// be indistinguishable from "not yet recorded" downstream.
func TestOperationPointInTimeModel_ToEntity_NilDecimals(t *testing.T) {
	t.Parallel()

	model := &OperationPointInTimeModel{
		ID:                    "op-id-nil",
		BalanceID:             "bal-id-nil",
		AccountID:             "acc-id-nil",
		AssetCode:             "EUR",
		BalanceKey:            "default",
		AvailableBalanceAfter: nil,
		OnHoldBalanceAfter:    nil,
		VersionBalanceAfter:   nil,
		CreatedAt:             time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	}

	entity := model.ToEntity()

	require.NotNil(t, entity)
	assert.Nil(t, entity.BalanceAfter.Available, "nil Available must propagate to the entity")
	assert.Nil(t, entity.BalanceAfter.OnHold, "nil OnHold must propagate to the entity")
	assert.Nil(t, entity.BalanceAfter.Version, "nil Version must propagate to the entity")
}
