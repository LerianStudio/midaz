// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mgrpc

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// MapAuthGRPCError maps gRPC auth errors to domain errors and logs raw details.
// Returns the original error when it isn't an auth error.
func MapAuthGRPCError(ctx context.Context, err error, code, title, operation string) error {
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
			Message: fmt.Sprintf("%s: unauthorized", operation),
			Err:     err,
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "grpc unauthorized", mapped)

		return mapped
	case codes.PermissionDenied:
		logger.Errorf("gRPC %s forbidden: %v", operation, err)

		mapped := pkg.ForbiddenError{
			Code:    code,
			Title:   title,
			Message: fmt.Sprintf("%s: forbidden", operation),
			Err:     err,
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "grpc forbidden", mapped)

		return mapped
	default:
		logger.Errorf("gRPC %s error: %v", operation, err)
		return err
	}
}
