package mgrpc

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestMapAuthGRPCError(t *testing.T) {
	t.Parallel()

	t.Run("nil error", func(t *testing.T) {
		require.NoError(t, MapAuthGRPCError(context.Background(), nil, "CODE", "Title", "op"))
	})

	t.Run("unauthenticated maps to unauthorized", func(t *testing.T) {
		err := MapAuthGRPCError(context.Background(), grpcstatus.Error(codes.Unauthenticated, "missing token"), "CODE", "Title", "op")

		var mapped pkg.UnauthorizedError
		require.ErrorAs(t, err, &mapped)
		assert.Equal(t, "CODE", mapped.Code)
		assert.Equal(t, "Title", mapped.Title)
		assert.Equal(t, "op: unauthorized", mapped.Message)
	})

	t.Run("permission denied maps to forbidden", func(t *testing.T) {
		err := MapAuthGRPCError(context.Background(), grpcstatus.Error(codes.PermissionDenied, "forbidden"), "CODE", "Title", "op")

		var mapped pkg.ForbiddenError
		require.ErrorAs(t, err, &mapped)
		assert.Equal(t, "CODE", mapped.Code)
		assert.Equal(t, "Title", mapped.Title)
		assert.Equal(t, "op: forbidden", mapped.Message)
	})

	t.Run("non auth error passes through", func(t *testing.T) {
		original := grpcstatus.Error(codes.Internal, "boom")
		err := MapAuthGRPCError(context.Background(), original, "CODE", "Title", "op")
		assert.ErrorIs(t, err, original)
	})
}
