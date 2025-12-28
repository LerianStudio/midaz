package query

import (
	"context"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/stretchr/testify/require"
)

func TestWaitForOperations_ReturnContract(t *testing.T) {
	// Return contract: either ops has items OR err is not nil OR timeout error
	// This test documents the expected behavior

	t.Run("returns operations when found", func(t *testing.T) {
		ctx := context.Background()
		expectedOps := []*operation.Operation{{ID: "op-1"}}

		fetch := func(ctx context.Context) ([]*operation.Operation, libHTTP.CursorPagination, error) {
			return expectedOps, libHTTP.CursorPagination{}, nil
		}

		ops, _, err := waitForOperations(ctx, fetch)

		// Contract: when fetch returns ops, we get them back
		require.NoError(t, err)
		require.NotEmpty(t, ops)
	})

	t.Run("returns error when fetch fails", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := ErrOperationsWaitTimeout

		fetch := func(ctx context.Context) ([]*operation.Operation, libHTTP.CursorPagination, error) {
			return nil, libHTTP.CursorPagination{}, expectedErr
		}

		ops, _, err := waitForOperations(ctx, fetch)

		// Contract: when fetch returns error, we propagate it
		require.Error(t, err)
		require.Empty(t, ops)
	})
}
