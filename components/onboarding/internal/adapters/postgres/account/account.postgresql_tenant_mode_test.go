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
