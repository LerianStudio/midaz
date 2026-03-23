// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetDB_RequiresTenantContext_WhenConfigured(t *testing.T) {
	t.Parallel()

	repo := NewAccountPostgreSQLRepository(nil, true)

	_, err := repo.getDB(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant postgres connection missing from context")
}
