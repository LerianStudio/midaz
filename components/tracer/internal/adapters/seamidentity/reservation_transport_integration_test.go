// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package seamidentity_test

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/bxcodec/dbresolver/v2"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"

	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

func TestContextReserveNativeGRPC(t *testing.T) {
	for _, scenario := range []string{"registered producer", "second tenant", "missing tenant", "unavailable tenant", "unknown producer", "legacy wire", "missing presence", "unsupported revision"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			admission := mocks.NewMockContextReserveAdmitter(ctrl)
			completion := mocks.NewMockContextReserveCompleter(ctrl)
			bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
			raw, err := os.ReadFile("../../../../../pkg/tracercontract/testdata/reserve_request.json")
			require.NoError(t, err)
			request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, 65536, bounds)
			require.NoError(t, err)
			wire, err := contractpb.EncodeReserve(t.Context(), request, "origin-a", bounds)
			require.NoError(t, err)
			uri := producerURI

			tenantID := "tenant-a"
			if scenario == "second tenant" {
				tenantID = "tenant-b"
			}
			if scenario == "missing tenant" {
				tenantID = ""
			}
			sqlDB, _, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			pool := dbresolver.New(dbresolver.WithPrimaryDBs(sqlDB))
			var resolutions atomic.Int32
			tenantResolver := seamtenant.NewResolverWithPool(func(ctx context.Context, id string) (dbresolver.DB, error) {
				resolutions.Add(1)
				require.Equal(t, tenantID, id)
				_, verified := contextutil.GetIntegrationIdentity(ctx)
				require.True(t, verified, "identity must precede tenant lookup")
				if scenario == "unavailable tenant" {
					return nil, errors.New("tenant pool unavailable")
				}
				return pool, nil
			}, true)
			assertTenant := func(ctx context.Context) {
				require.Equal(t, tenantID, tmcore.GetTenantIDContext(ctx))
				require.Equal(t, pool, tmcore.GetPGContext(ctx))
			}
			want := codes.InvalidArgument
			switch scenario {
			case "registered producer", "second tenant":
				want = codes.OK
				admission.EXPECT().Execute(gomock.Any(), request).DoAndReturn(func(ctx context.Context, got tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
					assertTenant(ctx)
					identity, ok := contextutil.GetIntegrationIdentity(ctx)
					require.True(t, ok)
					require.Equal(t, contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "origin-a"}, identity)
					return &tracercontract.ReserveResult{ContractRevision: got.ContractRevision, TransactionID: got.TransactionID, EvaluationID: testutil.MustDeterministicUUID(89441), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonRuleAllow}, ReservationIDs: []uuid.UUID{}}, nil
				})
				completion.EXPECT().ExecuteReport(gomock.Any(), request.TransactionID, model.OperationConfirmed).DoAndReturn(func(ctx context.Context, _ uuid.UUID, _ model.ReserveOperationStatus) (*tracercontract.TransactionCompletionResult, error) {
					assertTenant(ctx)
					return &tracercontract.TransactionCompletionResult{ContractRevision: request.ContractRevision, TransactionID: request.TransactionID, Status: "CONFIRMED"}, nil
				})
			case "missing tenant":
			case "unavailable tenant":
				want = codes.Internal
			case "unknown producer":
				uri += "-unknown"
				want = codes.PermissionDenied
			case "legacy wire":
				wire = &reservationv1.ReserveRequest{}
				wire.ProtoReflect().SetUnknown(protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), request.TransactionID.String()))
			case "missing presence":
				wire.LongLived = nil
			case "unsupported revision":
				wire.ContractRevision = "unsupported"
			}
			resolver, err := seamidentity.NewResolver([]seamidentity.Binding{{URI: producerURI, IntegrationID: "producer", AssetNamespace: "origin-a", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}}}, 256)
			require.NoError(t, err)
			service, err := grpcin.NewContextReservationServer(mocks.NewMockReservationService(ctrl), testutil.NewDefaultMockClock(), admission, completion, mocks.NewMockContextReserveIDCompleter(ctrl), grpcin.ContextReservationConfig{Bounds: bounds, MaxBodyBytes: 65536, MaxReservations: 100})
			require.NoError(t, err)
			serverTLS, clientTLS := identityTLSConfigs(t, []string{uri})
			server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)), grpc.ChainUnaryInterceptor(grpcin.ContextReservationUnaryInterceptor(resolver, tenantResolver)), grpc.MaxRecvMsgSize(65536))
			reservationv1.RegisterReservationServiceServer(server, service)
			listener := identityListener(t)
			done := make(chan error, 1)
			go func() { done <- server.Serve(listener) }()
			t.Cleanup(func() { server.Stop(); require.NoError(t, <-done) })
			conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, conn.Close()) })
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-integration-id", "forged", "x-asset-namespace", "forged", seamtenant.MetadataKey, tenantID))
			client := reservationv1.NewReservationServiceClient(conn)
			response, err := client.Reserve(ctx, wire)
			require.Equal(t, want, status.Code(err))
			if scenario == "unknown producer" || scenario == "missing tenant" {
				require.Zero(t, resolutions.Load())
			}
			if want == codes.OK {
				require.Equal(t, request.ContractRevision, response.GetContractRevision())
				require.Equal(t, string(tracercontract.DecisionAllow), response.GetDecision())
				report, err := client.ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{TransactionId: request.TransactionID.String(), ContractRevision: request.ContractRevision})
				require.NoError(t, err)
				require.Equal(t, "CONFIRMED", report.GetStatus())
			}
		})
	}
}
