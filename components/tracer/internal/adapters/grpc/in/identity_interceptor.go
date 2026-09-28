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
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// IdentityUnaryInterceptor resolves the platform producer from gRPC's verified
// TLS peer, never from incoming metadata, and binds the Producer and its
// contextutil.IntegrationIdentity into the context. Attach it before tenant
// authorization. A rejected certificate is codes.PermissionDenied; a nil
// registry, or one without certificate mappings, is codes.Unavailable.
func IdentityUnaryInterceptor(reg *producerauth.Registry) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

		_, span := tracer.Start(ctx, "middleware.reservations.authenticate_producer")
		defer span.End()

		var state *tls.ConnectionState

		if remote, ok := peer.FromContext(ctx); ok && remote != nil {
			if info, ok := remote.AuthInfo.(credentials.TLSInfo); ok {
				state = &info.State
			}
		}

		producer, err := reg.ByTLS(state)
		if err != nil {
			if errors.Is(err, constant.ErrInsufficientPrivileges) {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reservation producer rejected", err)

				return nil, status.Error(codes.PermissionDenied, constant.ErrInsufficientPrivileges.Error())
			}

			libOpentelemetry.HandleSpanError(span, "Reservation producer resolution unavailable", err)

			return nil, status.Error(codes.Unavailable, constant.ErrContextPolicyUnavailable.Error())
		}

		span.SetAttributes(
			attribute.String("app.auth.producer_service", producer.Service),
			attribute.String("app.auth.producer_via", producer.Via),
		)

		ctx = producerauth.WithProducer(ctx, producer)
		ctx = contextutil.WithIntegrationIdentity(ctx, contextutil.IntegrationIdentity{ID: producer.Service})

		return handler(ctx, req)
	}
}
