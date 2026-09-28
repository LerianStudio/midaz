// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

// TestContextGRPCClientSendsTenantWithoutAuthorization pins that every gRPC
// seam call carries the trusted tenant and no bearer credential: the gRPC
// identity is the client certificate.
func TestContextGRPCClientSendsTenantWithoutAuthorization(t *testing.T) {
	t.Parallel()

	request, config := contextClientFixture(t)
	wire, err := contractpb.EncodeResult(contextResultFixture(request), config.MaxReservations)
	require.NoError(t, err)

	evaluation := "33333333-3333-4333-8333-333333333333"

	var captured []metadata.MD

	stub := &stubReservationServer{
		captureMetadata: func(md metadata.MD) { captured = append(captured, md) },
		reserveFn: func(*reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
			return wire, nil
		},
		confirmByTransactionFn: func(input *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
			return &reservationv1.ConfirmByTransactionResponse{ContractRevision: input.ContractRevision, TransactionId: input.TransactionId, Status: "CONFIRMED", EvaluationId: &evaluation}, nil
		},
		releaseByTransactionFn: func(input *reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
			return &reservationv1.ReleaseByTransactionResponse{ContractRevision: input.ContractRevision, TransactionId: input.TransactionId, Status: "RELEASED", EvaluationId: &evaluation}, nil
		},
	}
	client := &ContextGRPCClient{transport: newTestGRPCClient(t, stub), config: config}
	ctx := tmcore.ContextWithTenantID(t.Context(), "tenant-007")

	_, err = client.Reserve(ctx, request)
	require.NoError(t, err)

	_, err = client.ConfirmByTransaction(ctx, request.TransactionID)
	require.NoError(t, err)

	_, err = client.ReleaseByTransaction(ctx, request.TransactionID)
	require.NoError(t, err)

	require.Len(t, captured, 3, "reserve, confirm and release each reach the server")

	for i, md := range captured {
		require.Equal(t, []string{"tenant-007"}, md.Get(tenantMetadataKey), "call %d", i)
		require.Empty(t, md.Get(strings.ToLower(AuthorizationHeader)), "call %d", i)
	}
}
