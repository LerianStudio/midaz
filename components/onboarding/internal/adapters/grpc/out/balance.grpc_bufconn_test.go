// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package out

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/LerianStudio/midaz/v3/pkg/mgrpc"
	proto "github.com/LerianStudio/midaz/v3/pkg/mgrpc/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// fakeBalanceServer implements BalanceProtoServer with scripted responses.
type fakeBalanceServer struct {
	proto.UnimplementedBalanceProtoServer

	createResp *proto.BalanceResponse
	createErr  error

	deleteErr error
}

func (f *fakeBalanceServer) CreateBalance(_ context.Context, _ *proto.BalanceRequest) (*proto.BalanceResponse, error) {
	return f.createResp, f.createErr
}

func (f *fakeBalanceServer) DeleteAllBalancesByAccountID(_ context.Context, _ *proto.DeleteAllBalancesByAccountIDRequest) (*proto.Empty, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}

	return &proto.Empty{}, nil
}

// startBufconnServer returns a running bufconn-backed server and a pre-dialled
// client ClientConn. Caller is responsible for cleanup via t.Cleanup.
func startBufconnServer(t *testing.T, impl proto.BalanceProtoServer) (*grpc.ClientConn, *grpc.Server) {
	t.Helper()

	const bufSize = 1024 * 1024

	lis := bufconn.Listen(bufSize)

	srv := grpc.NewServer()
	proto.RegisterBalanceProtoServer(srv, impl)

	go func() {
		_ = srv.Serve(lis)
	}()

	dialer := func(_ context.Context, _ string) (net.Conn, error) {
		return lis.Dial()
	}

	clientConn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = clientConn.Close()

		srv.Stop()
	})

	return clientConn, srv
}

// repoForConn produces a BalanceGRPCRepository whose connection's GetNewClient
// will return the supplied pre-dialled conn without hitting the network.
func repoForConn(conn *grpc.ClientConn) *BalanceGRPCRepository {
	return &BalanceGRPCRepository{
		conn: &mgrpc.GRPCConnection{
			Addr:   "bufnet",
			Conn:   conn,
			Logger: grpcNoopLogger{},
		},
	}
}

func TestBalanceGRPC_CreateBalance_HappyPath(t *testing.T) {
	t.Parallel()

	fake := &fakeBalanceServer{
		createResp: &proto.BalanceResponse{
			Id:             uuid.NewString(),
			Alias:          "wallet",
			AssetCode:      "USD",
			Available:      "100.00",
			OnHold:         "0.00",
			AllowSending:   true,
			AllowReceiving: true,
		},
	}
	conn, _ := startBufconnServer(t, fake)
	r := repoForConn(conn)

	resp, err := r.CreateBalance(context.Background(), "tok", &proto.BalanceRequest{
		AccountId: uuid.NewString(),
	})
	require.NoError(t, err)
	assert.Equal(t, "wallet", resp.Alias)
}

var errFakeGRPC = errors.New("grpc boom")

func TestBalanceGRPC_CreateBalance_RemoteError(t *testing.T) {
	t.Parallel()

	fake := &fakeBalanceServer{createErr: errFakeGRPC}
	conn, _ := startBufconnServer(t, fake)
	r := repoForConn(conn)

	_, err := r.CreateBalance(context.Background(), "", &proto.BalanceRequest{})
	require.Error(t, err)
}

func TestBalanceGRPC_DeleteAllBalancesByAccountID_HappyPath(t *testing.T) {
	t.Parallel()

	fake := &fakeBalanceServer{}
	conn, _ := startBufconnServer(t, fake)
	r := repoForConn(conn)

	err := r.DeleteAllBalancesByAccountID(context.Background(), "tok",
		&proto.DeleteAllBalancesByAccountIDRequest{AccountId: uuid.NewString()})
	require.NoError(t, err)
}

func TestBalanceGRPC_DeleteAllBalancesByAccountID_Error(t *testing.T) {
	t.Parallel()

	fake := &fakeBalanceServer{deleteErr: errFakeGRPC}
	conn, _ := startBufconnServer(t, fake)
	r := repoForConn(conn)

	err := r.DeleteAllBalancesByAccountID(context.Background(), "",
		&proto.DeleteAllBalancesByAccountIDRequest{AccountId: uuid.NewString()})
	require.Error(t, err)
}

func TestBalanceAdapter_CreateBalanceSync_HappyPath(t *testing.T) {
	t.Parallel()

	fake := &fakeBalanceServer{
		createResp: &proto.BalanceResponse{
			Id:             uuid.NewString(),
			Alias:          "w",
			AssetCode:      "USD",
			Available:      "50.5",
			OnHold:         "10.25",
			AllowSending:   true,
			AllowReceiving: true,
		},
	}
	conn, _ := startBufconnServer(t, fake)
	adapter := &BalanceAdapter{grpcRepo: repoForConn(conn)}

	got, err := adapter.CreateBalanceSync(context.Background(), mmodel.CreateBalanceInput{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		AccountID:      uuid.New(),
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "w", got.Alias)
}

func TestBalanceAdapter_CreateBalanceSync_InvalidAvailableDecimal(t *testing.T) {
	t.Parallel()

	fake := &fakeBalanceServer{
		createResp: &proto.BalanceResponse{Id: "id", Available: "not-a-decimal", OnHold: "0"},
	}
	conn, _ := startBufconnServer(t, fake)
	adapter := &BalanceAdapter{grpcRepo: repoForConn(conn)}

	_, err := adapter.CreateBalanceSync(context.Background(), mmodel.CreateBalanceInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse Available")
}

func TestBalanceAdapter_CreateBalanceSync_InvalidOnHoldDecimal(t *testing.T) {
	t.Parallel()

	fake := &fakeBalanceServer{
		createResp: &proto.BalanceResponse{Id: "id", Available: "0", OnHold: "not-a-decimal"},
	}
	conn, _ := startBufconnServer(t, fake)
	adapter := &BalanceAdapter{grpcRepo: repoForConn(conn)}

	_, err := adapter.CreateBalanceSync(context.Background(), mmodel.CreateBalanceInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse OnHold")
}

func TestBalanceAdapter_DeleteAllBalancesByAccountID_HappyPath(t *testing.T) {
	t.Parallel()

	fake := &fakeBalanceServer{}
	conn, _ := startBufconnServer(t, fake)
	adapter := &BalanceAdapter{grpcRepo: repoForConn(conn)}

	err := adapter.DeleteAllBalancesByAccountID(context.Background(), uuid.New(), uuid.New(), uuid.New(), "req")
	require.NoError(t, err)
}
