// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// DefaultGRPCAddress is the tracer's reservation gRPC seam on a local stack.
const DefaultGRPCAddress = "localhost:4021"

// GetGRPCAddress returns the reservation gRPC seam address the tests dial:
// TRACER_GRPC_ADDRESS when set, otherwise DefaultGRPCAddress.
func GetGRPCAddress() string {
	if addr := os.Getenv("TRACER_GRPC_ADDRESS"); addr != "" {
		return addr
	}

	return DefaultGRPCAddress
}

// seamAPIKeyMetadataKey is the gRPC metadata key the seam reads the API key
// from under API-key identity.
// #nosec G101 -- metadata key name, not a credential value.
const seamAPIKeyMetadataKey = "x-api-key"

// DialReservationClient returns a plaintext reservation gRPC client for the
// seam at GetGRPCAddress. Every RPC carries GetAPIKey as x-api-key, the seam
// credential when the service runs with API_KEY_ENABLED=true; the seam ignores
// it under any other identity. The connection closes when the test ends.
func DialReservationClient(tb testing.TB) reservationv1.ReservationServiceClient {
	tb.Helper()

	withAPIKey := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(metadata.AppendToOutgoingContext(ctx, seamAPIKeyMetadataKey, GetAPIKey()), method, req, reply, cc, opts...)
	}

	conn, err := grpc.NewClient(
		GetGRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(withAPIKey),
	)
	require.NoError(tb, err, "dial the reservation gRPC seam")

	tb.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			tb.Logf("close reservation gRPC connection: %v", closeErr)
		}
	})

	return reservationv1.NewReservationServiceClient(conn)
}
