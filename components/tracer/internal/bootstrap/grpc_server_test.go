// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
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
