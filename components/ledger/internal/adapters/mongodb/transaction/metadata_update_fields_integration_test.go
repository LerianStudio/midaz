//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

func TestIntegration_MetadataRepository_UpdateFields_WritesOnlyNamedKeys(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	repo := createRepository(t, container)
	ctx := context.Background()
	collection, id := "Transaction", "update-fields-txn"
	date := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	require.NoError(t, repo.Create(ctx, collection, &Metadata{
		EntityID: id, EntityName: collection, Data: JSON{"purpose": "client", "gone": "x"}, CreatedAt: date, UpdatedAt: date,
	}))

	settled, err := repo.UpdateFields(ctx, collection, id, map[string]any{"feeDebtSettlements": `[{"debtId":"d"}]`})
	require.NoError(t, err)
	assert.Equal(t, JSON{"purpose": "client", "gone": "x", "feeDebtSettlements": `[{"debtId":"d"}]`}, settled.Data)

	want := JSON{"purpose": "edited", "feeDebtSettlements": `[{"debtId":"d"}]`, "a.b": "dotted", "$price": "$literal"}
	patched, err := repo.UpdateFields(ctx, collection, id, map[string]any{"purpose": "edited", "gone": nil, "a.b": "dotted", "$price": "$literal"})
	require.NoError(t, err)
	assert.Equal(t, want, patched.Data, "a dotted or dollar key and a dollar value stay literal and flat")

	stored, err := repo.FindByEntity(ctx, collection, id)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, want, stored.Data)
	assert.Equal(t, collection, stored.EntityName)
	assert.True(t, stored.CreatedAt.Equal(date), "the creation time is kept")

	created, err := repo.UpdateFields(ctx, collection, id+"-missing", map[string]any{"purpose": "new", "none": nil})
	require.NoError(t, err)
	assert.Equal(t, JSON{"purpose": "new"}, created.Data, "a missing document is upserted like Update")
}
