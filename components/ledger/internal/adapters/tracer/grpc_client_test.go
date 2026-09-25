// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

// stubReservationServer is an in-memory ReservationService used to exercise the
// gRPC client's wire mapping. Each handler is a swappable func so a test case
// can return a canned response or status error.
type stubReservationServer struct {
	reservationv1.UnimplementedReservationServiceServer

	reserveFn              func(*reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error)
	confirmByIDFn          func(*reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error)
	releaseByIDFn          func(*reservationv1.ReleaseByIdRequest) (*reservationv1.ReleaseByIdResponse, error)
	confirmByTransactionFn func(*reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error)
	releaseByTransactionFn func(*reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error)

	// captureMetadata, when set, receives the incoming metadata the Reserve RPC
	// arrived with so a test can assert on tenant propagation.
	captureMetadata func(metadata.MD)
}

func (s *stubReservationServer) Reserve(ctx context.Context, req *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
	if s.captureMetadata != nil {
		md, _ := metadata.FromIncomingContext(ctx)
		s.captureMetadata(md)
	}

	return s.reserveFn(req)
}

func (s *stubReservationServer) ConfirmById(_ context.Context, req *reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
	return s.confirmByIDFn(req)
}

func (s *stubReservationServer) ReleaseById(_ context.Context, req *reservationv1.ReleaseByIdRequest) (*reservationv1.ReleaseByIdResponse, error) {
	return s.releaseByIDFn(req)
}

func (s *stubReservationServer) ConfirmByTransaction(_ context.Context, req *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
	return s.confirmByTransactionFn(req)
}

func (s *stubReservationServer) ReleaseByTransaction(_ context.Context, req *reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
	return s.releaseByTransactionFn(req)
}

// newTestGRPCClient stands up the stub server on an in-memory bufconn listener
// and returns a client dialed to it. The server stops and the client closes via
// t.Cleanup.
func newTestGRPCClient(t *testing.T, stub *stubReservationServer) *TracerGRPCClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)

	srv := grpc.NewServer()
	reservationv1.RegisterReservationServiceServer(srv, stub)

	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(tenantUnaryInterceptor),
	)
	require.NoError(t, err)

	client := &TracerGRPCClient{
		conn:             conn,
		client:           reservationv1.NewReservationServiceClient(conn),
		operationTimeout: defaultOperationTimeout,
	}

	t.Cleanup(func() {
		_ = conn.Close()
		srv.Stop()
		_ = lis.Close()
	})

	return client
}

func TestNewTracerGRPCClient_EmptyTarget(t *testing.T) {
	t.Parallel()

	client, err := NewTracerGRPCClient("")
	require.Error(t, err)
	assert.Nil(t, client)
}

func TestNewTracerGRPCClient_ImplementsTracerReserver(t *testing.T) {
	t.Parallel()

	// grpc.NewClient is lazy (no dial at construction), so this never blocks on
	// reachability. The assignment proves the concrete type satisfies the port.
	client, err := NewTracerGRPCClient("passthrough:///tracer:4020")
	require.NoError(t, err)
	require.NotNil(t, client)

	t.Cleanup(func() { _ = client.Close() })

	var _ interface {
		Reserve(context.Context, ReserveRequest) (*ReserveResult, error)
		Confirm(context.Context, uuid.UUID) error
		Release(context.Context, uuid.UUID) error
		ConfirmByTransaction(context.Context, uuid.UUID) error
		ReleaseByTransaction(context.Context, uuid.UUID) error
	} = client
}

func contextClientFixture(t *testing.T) (tracercontract.ReserveRequest, ContextClientConfig) {
	t.Helper()
	config := ContextClientConfig{Namespace: "origin-a", Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxBodyBytes: 65536, MaxReservations: 100}
	raw, err := os.ReadFile("../../../../../pkg/tracercontract/testdata/reserve_request.json")
	require.NoError(t, err)
	request, err := tracercontract.DecodeReserveJSON(t.Context(), raw, config.MaxBodyBytes, config.Bounds)
	require.NoError(t, err)
	return request, config
}

func contextResultFixture(request tracercontract.ReserveRequest) *tracercontract.ReserveResult {
	return &tracercontract.ReserveResult{ContractRevision: request.ContractRevision, TransactionID: request.TransactionID, EvaluationID: uuid.MustParse("33333333-3333-4333-8333-333333333333"), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}
}

func TestContextGRPCClientReserve(t *testing.T) {
	for _, scenario := range []string{"allow", "deny", "review", "unavailable", "internal", "malformed reservation", "wrong transaction", "missing rules", "invalid request"} {
		t.Run(scenario, func(t *testing.T) {
			request, config := contextClientFixture(t)
			response := contextResultFixture(request)
			if scenario == "deny" {
				response.Decision = tracercontract.DecisionDeny
				response.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonLimitExceeded}
			}
			if scenario == "review" {
				response.Decision = tracercontract.DecisionReview
				response.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonRuleReview}
			}
			encoded, err := contractpb.EncodeResult(response, config.MaxReservations)
			require.NoError(t, err)
			calls := 0
			stub := &stubReservationServer{reserveFn: func(input *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
				calls++
				actual, err := contractpb.DecodeReserve(t.Context(), input, config.Namespace, config.Bounds, config.MaxBodyBytes)
				require.NoError(t, err)
				require.Equal(t, request, actual)
				switch scenario {
				case "unavailable":
					return nil, status.Error(codes.Unavailable, "unavailable")
				case "internal":
					return nil, status.Error(codes.Internal, "internal")
				case "malformed reservation":
					encoded.ReservationIds = []string{"invalid"}
				case "wrong transaction":
					encoded.TransactionId = "44444444-4444-4444-8444-444444444444"
				case "missing rules":
					encoded.Controls.Rules = string(tracercontract.RulesNotRequested)
				}
				return encoded, nil
			}}
			client := &ContextGRPCClient{transport: newTestGRPCClient(t, stub), config: config}
			if scenario == "invalid request" {
				request.LongLived = nil
			}
			result, err := client.Reserve(t.Context(), request)
			switch scenario {
			case "allow", "deny", "review":
				require.NoError(t, err)
				require.Equal(t, response, result)
			default:
				require.Error(t, err)
				require.Nil(t, result)
			}
			if scenario == "unavailable" || scenario == "internal" {
				require.ErrorIs(t, err, ErrTracerUnavailable)
			}
			if scenario == "invalid request" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestLegacyGRPCReserveCannotInventContext(t *testing.T) {
	client := newTestGRPCClient(t, &stubReservationServer{})
	result, err := client.Reserve(t.Context(), ReserveRequest{})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	require.Nil(t, result)
}

func TestTracerGRPCClient_Confirm(t *testing.T) {
	t.Parallel()

	reservationID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	t.Run("success passes reservation id", func(t *testing.T) {
		t.Parallel()

		var captured *reservationv1.ConfirmByIdRequest

		stub := &stubReservationServer{
			confirmByIDFn: func(req *reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
				captured = req

				return &reservationv1.ConfirmByIdResponse{}, nil
			},
		}
		client := newTestGRPCClient(t, stub)

		require.NoError(t, client.Confirm(context.Background(), reservationID))
		require.NotNil(t, captured)
		assert.Equal(t, reservationID.String(), captured.GetReservationId())
	})

	t.Run("unavailable maps to ErrTracerUnavailable", func(t *testing.T) {
		t.Parallel()

		stub := &stubReservationServer{
			confirmByIDFn: func(_ *reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
				return nil, status.Error(codes.Unavailable, "down")
			},
		}
		client := newTestGRPCClient(t, stub)

		err := client.Confirm(context.Background(), reservationID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTracerUnavailable)
	})

	t.Run("not found surfaces verbatim", func(t *testing.T) {
		t.Parallel()

		stub := &stubReservationServer{
			confirmByIDFn: func(_ *reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
				return nil, status.Error(codes.NotFound, "no reservation")
			},
		}
		client := newTestGRPCClient(t, stub)

		err := client.Confirm(context.Background(), reservationID)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrTracerUnavailable)
		assert.Equal(t, codes.NotFound, status.Code(err))
	})
}

func TestTracerGRPCClient_Release(t *testing.T) {
	t.Parallel()

	reservationID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	var captured *reservationv1.ReleaseByIdRequest

	stub := &stubReservationServer{
		releaseByIDFn: func(req *reservationv1.ReleaseByIdRequest) (*reservationv1.ReleaseByIdResponse, error) {
			captured = req

			return &reservationv1.ReleaseByIdResponse{}, nil
		},
	}
	client := newTestGRPCClient(t, stub)

	require.NoError(t, client.Release(context.Background(), reservationID))
	require.NotNil(t, captured)
	assert.Equal(t, reservationID.String(), captured.GetReservationId())
}

func TestTracerGRPCClient_ConfirmByTransaction(t *testing.T) {
	t.Parallel()

	transactionID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	t.Run("success passes transaction id", func(t *testing.T) {
		t.Parallel()

		var captured *reservationv1.ConfirmByTransactionRequest

		stub := &stubReservationServer{
			confirmByTransactionFn: func(req *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
				captured = req

				return &reservationv1.ConfirmByTransactionResponse{}, nil
			},
		}
		client := newTestGRPCClient(t, stub)

		require.NoError(t, client.ConfirmByTransaction(context.Background(), transactionID))
		require.NotNil(t, captured)
		assert.Equal(t, transactionID.String(), captured.GetTransactionId())
	})

	t.Run("unavailable maps to ErrTracerUnavailable", func(t *testing.T) {
		t.Parallel()

		stub := &stubReservationServer{
			confirmByTransactionFn: func(_ *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
				return nil, status.Error(codes.Unavailable, "down")
			},
		}
		client := newTestGRPCClient(t, stub)

		err := client.ConfirmByTransaction(context.Background(), transactionID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTracerUnavailable)
	})
}

func TestTracerGRPCClient_ReleaseByTransaction(t *testing.T) {
	t.Parallel()

	transactionID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	var captured *reservationv1.ReleaseByTransactionRequest

	stub := &stubReservationServer{
		releaseByTransactionFn: func(req *reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
			captured = req

			return &reservationv1.ReleaseByTransactionResponse{}, nil
		},
	}
	client := newTestGRPCClient(t, stub)

	require.NoError(t, client.ReleaseByTransaction(context.Background(), transactionID))
	require.NotNil(t, captured)
	assert.Equal(t, transactionID.String(), captured.GetTransactionId())
}

// TestTracerGRPCClient_PropagatesTenantMetadata pins trusted tenant propagation
// on the gRPC transport: when the request context carries a tenant, the client
// appends it to the outgoing metadata under the lower-cased TenantHeader key,
// and when the context carries none it appends nothing.
func TestTracerGRPCClient_PropagatesTenantMetadata(t *testing.T) {
	t.Parallel()

	request, config := contextClientFixture(t)
	wire, err := contractpb.EncodeResult(contextResultFixture(request), config.MaxReservations)
	require.NoError(t, err)

	// The gRPC metadata key MUST be the lower-cased REST TenantHeader so the two
	// transports cannot drift.
	assert.Equal(t, "x-tenant-id", tenantMetadataKey)

	t.Run("tenant in context lands on outgoing metadata", func(t *testing.T) {
		t.Parallel()

		var captured metadata.MD

		stub := &stubReservationServer{
			captureMetadata: func(md metadata.MD) { captured = md },
			reserveFn: func(_ *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
				return wire, nil
			},
		}
		client := &ContextGRPCClient{transport: newTestGRPCClient(t, stub), config: config}

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-007")

		_, err := client.Reserve(ctx, request)
		require.NoError(t, err)

		require.NotNil(t, captured)
		assert.Equal(t, []string{"tenant-007"}, captured.Get(tenantMetadataKey))
	})

	t.Run("no tenant in context appends no metadata", func(t *testing.T) {
		t.Parallel()

		var captured metadata.MD

		stub := &stubReservationServer{
			captureMetadata: func(md metadata.MD) { captured = md },
			reserveFn: func(_ *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
				return wire, nil
			},
		}
		client := &ContextGRPCClient{transport: newTestGRPCClient(t, stub), config: config}

		_, err := client.Reserve(context.Background(), request)
		require.NoError(t, err)

		assert.Empty(t, captured.Get(tenantMetadataKey))
	})
}

func TestMapGRPCError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		err             error
		wantUnavailable bool
	}{
		{"nil", nil, false},
		{"unavailable", status.Error(codes.Unavailable, "x"), true},
		{"deadline exceeded", status.Error(codes.DeadlineExceeded, "x"), true},
		{"canceled", status.Error(codes.Canceled, "x"), true},
		{"context deadline", context.DeadlineExceeded, true},
		{"context canceled", context.Canceled, true},
		{"not found", status.Error(codes.NotFound, "x"), false},
		{"internal", status.Error(codes.Internal, "x"), true},
		{"unknown", status.Error(codes.Unknown, "x"), true},
		{"resource exhausted", status.Error(codes.ResourceExhausted, "x"), true},
		{"deterministic resource exhausted", status.Error(codes.ResourceExhausted, constant.ErrInvalidRequestBody.Error()), false},
		{"invalid argument", status.Error(codes.InvalidArgument, "x"), false},
		{"plain error", errors.New("x"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapGRPCError(tt.err)
			if tt.err == nil {
				assert.NoError(t, got)
				return
			}

			assert.Equal(t, tt.wantUnavailable, errors.Is(got, ErrTracerUnavailable))
		})
	}
}

func TestContextGRPCClientCompletion(t *testing.T) {
	for _, scenario := range []string{"confirm", "release", "before admission", "wrong transaction", "wrong revision", "wrong status", "missing evaluation with movement", "malformed evaluation", "unknown field", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			request, config := contextClientFixture(t)
			revision := tracercontract.ReserveContractRevision
			evaluation := "33333333-3333-4333-8333-333333333333"
			makeResponse := func() (*reservationv1.ConfirmByTransactionResponse, error) {
				response := &reservationv1.ConfirmByTransactionResponse{ContractRevision: revision, TransactionId: request.TransactionID.String(), Status: "CONFIRMED", EvaluationId: &evaluation}
				switch scenario {
				case "before admission":
					response.EvaluationId = nil
				case "wrong transaction":
					response.TransactionId = evaluation
				case "wrong revision":
					response.ContractRevision = "legacy"
				case "wrong status":
					response.Status = "RELEASED"
				case "missing evaluation with movement":
					response.EvaluationId = nil
					response.Flipped = 1
				case "malformed evaluation":
					value := "invalid"
					response.EvaluationId = &value
				case "unknown field":
					response.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 99, protowire.VarintType), 1))
				case "unavailable":
					return nil, status.Error(codes.Unavailable, "unavailable")
				}
				return response, nil
			}
			stub := &stubReservationServer{
				confirmByTransactionFn: func(input *reservationv1.ConfirmByTransactionRequest) (*reservationv1.ConfirmByTransactionResponse, error) {
					require.Equal(t, revision, input.ContractRevision)
					require.Equal(t, request.TransactionID.String(), input.TransactionId)
					return makeResponse()
				},
				releaseByTransactionFn: func(input *reservationv1.ReleaseByTransactionRequest) (*reservationv1.ReleaseByTransactionResponse, error) {
					require.Equal(t, revision, input.ContractRevision)
					require.Equal(t, request.TransactionID.String(), input.TransactionId)
					return &reservationv1.ReleaseByTransactionResponse{ContractRevision: revision, TransactionId: input.TransactionId, Status: "RELEASED", EvaluationId: &evaluation}, nil
				},
			}
			client := &ContextGRPCClient{transport: newTestGRPCClient(t, stub), config: config}
			var result *tracercontract.TransactionCompletionResult
			var err error
			if scenario == "release" {
				result, err = client.ReleaseByTransaction(t.Context(), request.TransactionID)
			} else {
				result, err = client.ConfirmByTransaction(t.Context(), request.TransactionID)
			}
			switch scenario {
			case "confirm", "release", "before admission":
				require.NoError(t, err)
				require.NoError(t, result.Validate())
				require.Equal(t, request.TransactionID, result.TransactionID)
				if scenario == "before admission" {
					require.Nil(t, result.EvaluationID)
				}
			default:
				require.Error(t, err)
				require.Nil(t, result)
			}
			if scenario == "unavailable" {
				require.ErrorIs(t, err, ErrTracerUnavailable)
			}
		})
	}
}
