// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
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
}

func (s *stubReservationServer) Reserve(_ context.Context, req *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
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

func TestTracerGRPCClient_Reserve(t *testing.T) {
	t.Parallel()

	transactionID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	reservationID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	accountID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	t.Run("allow maps request and result field-for-field", func(t *testing.T) {
		t.Parallel()

		var captured *reservationv1.ReserveRequest

		stub := &stubReservationServer{
			reserveFn: func(req *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
				captured = req

				return &reservationv1.ReserveResult{
					TransactionId:  transactionID.String(),
					Denied:         false,
					ReservationIds: []string{reservationID.String()},
				}, nil
			},
		}
		client := newTestGRPCClient(t, stub)

		req := ReserveRequest{
			TransactionID:        transactionID,
			RequestID:            "req-1",
			Amount:               "100.50",
			Currency:             "USD",
			Account:              ReserveAccount{AccountID: accountID.String()},
			TransactionTimestamp: "2026-06-11T00:00:00Z",
		}

		result, err := client.Reserve(context.Background(), req)
		require.NoError(t, err)

		require.NotNil(t, captured)
		assert.Equal(t, transactionID.String(), captured.GetTransactionId())
		assert.Equal(t, "req-1", captured.GetRequestId())
		assert.Equal(t, "100.50", captured.GetAmount())
		assert.Equal(t, "USD", captured.GetCurrency())
		assert.Equal(t, accountID.String(), captured.GetAccount().GetAccountId())
		assert.Equal(t, "2026-06-11T00:00:00Z", captured.GetTransactionTimestamp())

		assert.False(t, result.Denied)
		assert.Equal(t, transactionID, result.TransactionID)
		assert.Equal(t, []uuid.UUID{reservationID}, result.ReservationIDs)
	})

	t.Run("denied is a successful result, not an error", func(t *testing.T) {
		t.Parallel()

		stub := &stubReservationServer{
			reserveFn: func(_ *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
				return &reservationv1.ReserveResult{
					TransactionId:  transactionID.String(),
					Denied:         true,
					ReservationIds: nil,
				}, nil
			},
		}
		client := newTestGRPCClient(t, stub)

		result, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: transactionID})
		require.NoError(t, err)
		assert.True(t, result.Denied)
		assert.Empty(t, result.ReservationIDs)
	})

	t.Run("unavailable status maps to ErrTracerUnavailable", func(t *testing.T) {
		t.Parallel()

		stub := &stubReservationServer{
			reserveFn: func(_ *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
				return nil, status.Error(codes.Unavailable, "tracer down")
			},
		}
		client := newTestGRPCClient(t, stub)

		result, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: transactionID})
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrTracerUnavailable)
	})

	t.Run("internal status surfaces verbatim, not as unavailable", func(t *testing.T) {
		t.Parallel()

		stub := &stubReservationServer{
			reserveFn: func(_ *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
				return nil, status.Error(codes.Internal, "boom")
			},
		}
		client := newTestGRPCClient(t, stub)

		result, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: transactionID})
		require.Error(t, err)
		assert.Nil(t, result)
		assert.NotErrorIs(t, err, ErrTracerUnavailable)
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("malformed reservation id from tracer is a contract error", func(t *testing.T) {
		t.Parallel()

		stub := &stubReservationServer{
			reserveFn: func(_ *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
				return &reservationv1.ReserveResult{
					TransactionId:  transactionID.String(),
					ReservationIds: []string{"not-a-uuid"},
				}, nil
			},
		}
		client := newTestGRPCClient(t, stub)

		result, err := client.Reserve(context.Background(), ReserveRequest{TransactionID: transactionID})
		require.Error(t, err)
		assert.Nil(t, result)
		assert.NotErrorIs(t, err, ErrTracerUnavailable)
	})
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
		{"internal", status.Error(codes.Internal, "x"), false},
		{"invalid argument", status.Error(codes.InvalidArgument, "x"), false},
		{"plain error", errors.New("x"), false},
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
