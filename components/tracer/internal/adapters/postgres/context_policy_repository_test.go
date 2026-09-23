// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestContextPolicyRepositoryRequiresTenantPool(t *testing.T) {
	t.Parallel()
	conn := &pgdb.PostgresConnectionAdapter{}
	conn.SetMultiTenantEnabled(true)
	repo, err := NewContextPolicyRepository(conn, 10)
	require.NoError(t, err)
	key := model.PolicyBindingKey{IntegrationID: "verified-producer", ContextID: "context"}
	_, err = repo.GetActive(context.Background(), key)
	require.ErrorIs(t, err, pgdb.ErrNoTenantInContext, "never fall back to a global policy database")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = repo.GetActive(ctx, key)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.GetActive(context.Background(), model.PolicyBindingKey{})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody, "empty keys never select an implicit binding")
}
