// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/tls"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

func TestNewGRPCServerRequiresClientCertificatesAndTheReservationInterceptor(t *testing.T) {
	t.Parallel()

	service := reservationv1.UnimplementedReservationServiceServer{}
	interceptor := grpc.UnaryServerInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(ctx, req)
	})
	mutual := &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert}
	telemetry := &libOtel.Telemetry{}

	for _, tc := range []struct {
		name        string
		tlsConfig   *tls.Config
		interceptor grpc.UnaryServerInterceptor
		reason      string
	}{
		{name: "no TLS config", interceptor: interceptor, reason: "TLS config must require and verify client certificates"},
		{name: "TLS without client certificates", tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.NoClientCert}, interceptor: interceptor, reason: "TLS config must require and verify client certificates"},
		{name: "TLS that only requests client certificates", tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.VerifyClientCertIfGiven}, interceptor: interceptor, reason: "TLS config must require and verify client certificates"},
		{name: "no interceptor", tlsConfig: mutual, reason: "reservation interceptor must not be nil"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server, err := NewGRPCServer(":0", service, tc.tlsConfig, tc.interceptor, libLog.NewNop(), telemetry)
			require.ErrorContains(t, err, tc.reason)
			require.Nil(t, server)
		})
	}

	server, err := NewGRPCServer(":0", service, mutual, interceptor, libLog.NewNop(), telemetry)
	require.NoError(t, err)
	require.NotNil(t, server)
	server.server.Stop()
}
