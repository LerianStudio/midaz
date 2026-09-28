// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package producerauth_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"

	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

// producerURI is the certificate URI SAN the fixtures map onto the ledger
// platform producer.
const producerURI = "spiffe://example.test/service/producer"

// Real loopback TLS handshakes prove that the gRPC identity interceptor reads
// the certificate actually verified by the server, rather than a manually
// injected context or forged metadata.
func TestProducerIdentityMTLS(t *testing.T) {
	for _, tc := range []struct {
		name    string
		uris    []string
		allowed bool
	}{
		{"registered producer", []string{producerURI}, true},
		{"trusted CA but unknown producer", []string{producerURI + "-other"}, false},
		{"trusted CA but no URI", nil, false},
		{"ambiguous certificate", []string{producerURI, producerURI + "-other"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serverTLS, clientTLS := identityTLSConfigs(t, tc.uris)
			checkGRPCIdentity(t, platformRegistry(t), serverTLS, clientTLS, tc.allowed)
		})
	}
}

func identityTLSConfigs(t *testing.T, uris []string) (*tls.Config, *tls.Config) {
	t.Helper()
	fixture := testutil.GenerateMTLSFixture(t, uris...)
	server, err := tls.X509KeyPair(fixture.ServerCertPEM, fixture.ServerKeyPEM)
	require.NoError(t, err)
	client, err := tls.X509KeyPair(fixture.ClientCertPEM, fixture.ClientKeyPEM)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(fixture.CACertPEM))
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{server}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool},
		&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{client}, RootCAs: pool, ServerName: "localhost"}
}

func identityListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

// platformRegistry maps producerURI onto the ledger platform producer.
func platformRegistry(t *testing.T) *producerauth.Registry {
	t.Helper()
	registry, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","certUri":"` + producerURI + `"}]`)
	require.NoError(t, err)
	return registry
}

func assertCapturedIdentity(t *testing.T, identities <-chan contextutil.IntegrationIdentity, allowed bool, wantID string) {
	t.Helper()
	if !allowed {
		require.Empty(t, identities)
		return
	}
	select {
	case identity := <-identities:
		require.Equal(t, contextutil.IntegrationIdentity{ID: wantID}, identity)
	default:
		t.Fatal("authorized handler did not receive the verified identity")
	}
}

func checkGRPCIdentity(t *testing.T, registry *producerauth.Registry, serverTLS, clientTLS *tls.Config, allowed bool) {
	t.Helper()
	identities := make(chan contextutil.IntegrationIdentity, 1)
	capture := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		identity, _ := contextutil.GetIntegrationIdentity(ctx)
		identities <- identity
		return next(ctx, req)
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)), grpc.ChainUnaryInterceptor(grpcin.IdentityUnaryInterceptor(registry), capture))
	healthv1.RegisterHealthServer(server, health.NewServer())
	listener := identityListener(t)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); require.NoError(t, <-done) })
	client, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-integration-id", "forged", "x-forwarded-client-cert", producerURI))
	_, err = healthv1.NewHealthClient(client).Check(ctx, &healthv1.HealthCheckRequest{})
	want := codes.PermissionDenied
	if allowed {
		want = codes.OK
	}
	require.Equal(t, want, status.Code(err))
	assertCapturedIdentity(t, identities, allowed, producerauth.ServiceLedger)
}

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
			wire, err := contractpb.EncodeReserve(t.Context(), request, bounds)
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
					require.Equal(t, contextutil.IntegrationIdentity{ID: producerauth.ServiceLedger}, identity)
					return &tracercontract.ReserveResult{ContractRevision: got.ContractRevision, TransactionID: got.TransactionID, EvaluationID: testutil.MustDeterministicUUID(89441), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonRuleAllow}, ReservationIDs: []uuid.UUID{}}, nil
				})
				completion.EXPECT().ExecuteReport(gomock.Any(), request.TransactionID, model.OperationConfirmed).DoAndReturn(func(ctx context.Context, _ uuid.UUID, _ model.ReserveOperationStatus) (*tracercontract.TransactionCompletionResult, error) {
					assertTenant(ctx)
					return &tracercontract.TransactionCompletionResult{ContractRevision: request.ContractRevision, TransactionID: request.TransactionID, Status: "CONFIRMED"}, nil
				})
			case "missing tenant":
			case "unavailable tenant":
				want = codes.Unavailable
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
			tenantAuthorizer := producerauth.NewTenantAuthorizer(func(_ context.Context, id, service string) error {
				require.Equal(t, tenantID, id)
				require.Equal(t, producerauth.ServiceLedger, service)
				return nil
			}, true)
			service, err := grpcin.NewContextReservationServer(admission, completion, mocks.NewMockContextReserveIDCompleter(ctrl), grpcin.ContextReservationConfig{Bounds: bounds, MaxBodyBytes: 65536, MaxReservations: 100})
			require.NoError(t, err)
			serverTLS, clientTLS := identityTLSConfigs(t, []string{uri})
			server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)), grpc.ChainUnaryInterceptor(grpcin.ContextReservationUnaryInterceptor(platformRegistry(t), tenantAuthorizer, tenantResolver)), grpc.MaxRecvMsgSize(65536))
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
			ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-integration-id", "forged", seamtenant.MetadataKey, tenantID))
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
