// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"crypto/tls"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// IdentityUnaryInterceptor resolves the producer from gRPC's verified TLS peer,
// never from incoming metadata. Attach it before tenant resolution on the context
// reservation server when the durable decision flow is enabled. A nil resolver
// fails closed; it does not mean authentication is optional.
func IdentityUnaryInterceptor(resolver *seamidentity.Resolver) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

		ctx, span := tracer.Start(ctx, "middleware.reservations.resolve_identity")
		defer span.End()

		var state *tls.ConnectionState

		if remote, ok := peer.FromContext(ctx); ok && remote != nil {
			if info, ok := remote.AuthInfo.(credentials.TLSInfo); ok {
				state = &info.State
			}
		}

		identity, err := resolver.ResolveTLS(ctx, state)
		if err != nil {
			if errors.Is(err, constant.ErrInsufficientPrivileges) {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation producer rejected", err)
				return nil, status.Error(codes.PermissionDenied, constant.ErrInsufficientPrivileges.Error())
			}

			libOpentelemetry.HandleSpanError(span, "Reservation identity resolution failed", err)

			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, status.FromContextError(err).Err()
			}

			return nil, status.Error(codes.Unavailable, constant.ErrContextPolicyUnavailable.Error())
		}

		return handler(contextutil.WithIntegrationIdentity(ctx, identity), req)
	}
}
