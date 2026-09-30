// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestGRPCServer_Stop_ReleasesListener proves Stop ends Serve and frees the
// listen address, so a restarted service can bind the same port.
func TestGRPCServer_Stop_ReleasesListener(t *testing.T) {
	t.Parallel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	address := lis.Addr().String()
	server := &GRPCServer{server: grpc.NewServer(), address: address}

	served := make(chan error, 1)

	go func() { served <- server.server.Serve(lis) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server.Stop(ctx)

	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Stop")
	}

	rebound, err := net.Listen("tcp", address)
	require.NoError(t, err, "the listen address must be free after Stop")
	assert.NoError(t, rebound.Close())
}

// TestGRPCServer_Stop_NilIsNoOp proves a Service without a gRPC server shuts
// down cleanly.
func TestGRPCServer_Stop_NilIsNoOp(t *testing.T) {
	t.Parallel()

	var server *GRPCServer

	assert.NotPanics(t, func() { server.Stop(context.Background()) })
}

// TestGRPCServer_Stop_BoundsAHangingRPC proves Stop cancels an RPC that never
// finishes once the shutdown budget runs out: through the caller's deadline
// when ctx carries one, through the fallback bound when it does not.
func TestGRPCServer_Stop_BoundsAHangingRPC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		stop func(s *GRPCServer)
	}{
		{
			name: "caller deadline",
			stop: func(s *GRPCServer) {
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				defer cancel()

				s.Stop(ctx)
			},
		},
		{
			name: "no deadline falls back to the bound",
			stop: func(s *GRPCServer) { s.stop(context.Background(), 200*time.Millisecond) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, started := startHangingGRPCServer(t)

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("the hanging RPC never reached the server")
			}

			stopped := make(chan struct{})

			go func() {
				tt.stop(server)
				close(stopped)
			}()

			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("Stop did not return while an RPC hung")
			}
		})
	}
}

// startHangingGRPCServer serves a handler that blocks until its RPC is
// cancelled, issues one call to it, and returns the server plus a channel
// closed once the call reached the handler.
func startHangingGRPCServer(t *testing.T) (*GRPCServer, <-chan struct{}) {
	t.Helper()

	started := make(chan struct{})

	var once sync.Once

	hang := func(_ any, stream grpc.ServerStream) error {
		once.Do(func() { close(started) })
		<-stream.Context().Done()

		return stream.Context().Err()
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &GRPCServer{server: grpc.NewServer(grpc.UnknownServiceHandler(hang)), address: lis.Addr().String()}

	go func() { _ = server.server.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		server.server.Stop()
	})

	go func() {
		_ = conn.Invoke(context.Background(), "/test.Hang/Forever", &emptypb.Empty{}, &emptypb.Empty{})
	}()

	return server, started
}
