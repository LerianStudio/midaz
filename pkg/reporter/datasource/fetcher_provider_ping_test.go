// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/fetcher"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pingableFetcherClient is a test stub that implements FetcherManagementClient
// (including the new Ping method introduced in Gate 2).
type pingableFetcherClient struct {
	pingFn func(ctx context.Context) error
}

func (p *pingableFetcherClient) ListConnections(_ context.Context) ([]fetcher.ConnectionResponse, error) {
	return nil, nil
}

func (p *pingableFetcherClient) GetConnectionSchema(_ context.Context, _ string) (*fetcher.ConnectionSchemaResponse, error) {
	return nil, nil
}

func (p *pingableFetcherClient) ValidateSchema(_ context.Context, _ map[string]map[string][]string) (*fetcher.ValidateSchemaResponse, error) {
	return nil, nil
}

func (p *pingableFetcherClient) Ping(ctx context.Context) error {
	if p.pingFn != nil {
		return p.pingFn(ctx)
	}

	return nil
}

func TestFetcherProvider_Ping_DelegatesToClient(t *testing.T) {
	t.Parallel()

	called := false
	mock := &pingableFetcherClient{
		pingFn: func(_ context.Context) error {
			called = true
			return nil
		},
	}

	p := NewFetcherProvider(mock)
	err := p.Ping(context.Background())

	require.NoError(t, err)
	assert.True(t, called, "Ping must delegate to the underlying client")
}

func TestFetcherProvider_Ping_PropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("upstream down")
	mock := &pingableFetcherClient{
		pingFn: func(_ context.Context) error { return wantErr },
	}

	p := NewFetcherProvider(mock)
	err := p.Ping(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}
