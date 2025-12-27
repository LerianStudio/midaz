package mgrpc

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// MapAuthGRPCError maps gRPC auth errors to domain errors and logs raw details.
// Returns the original error when it isn't an auth error.
//
// Parameters code, title, and operation are used in error responses and must be non-empty.
// Empty values indicate a programming error in the caller.
func MapAuthGRPCError(ctx context.Context, err error, code, title, operation string) error {
	// Parameter validation - these are programmer errors, not runtime conditions
	assert.NotEmpty(code, "error code must not be empty",
		"function", "MapAuthGRPCError")
	assert.NotEmpty(title, "error title must not be empty",
		"function", "MapAuthGRPCError",
		"code", code)
	assert.NotEmpty(operation, "operation description must not be empty",
		"function", "MapAuthGRPCError",
		"code", code)

	if err == nil {
		return nil
	}

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "mgrpc.map_auth_grpc_error")
	defer span.End()

	switch grpcstatus.Code(err) {
	case codes.Unauthenticated:
		logger.Errorf("gRPC %s unauthorized: %v", operation, err)

		mapped := pkg.UnauthorizedError{
			Code:    code,
			Title:   title,
			Message: operation + ": unauthorized",
			Err:     err,
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "grpc unauthorized", mapped)

		return mapped
	case codes.PermissionDenied:
		logger.Errorf("gRPC %s forbidden: %v", operation, err)

		mapped := pkg.ForbiddenError{
			Code:    code,
			Title:   title,
			Message: operation + ": forbidden",
			Err:     err,
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "grpc forbidden", mapped)

		return mapped
	default:
		logger.Errorf("gRPC %s error: %v", operation, err)
		return err
	}
}
