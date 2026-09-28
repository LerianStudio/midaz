// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedTransactionID is a deterministic UUID literal so the tests carry no
// uuid.New() randomness.
var fixedTransactionID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func TestNewTracerClient_RejectsEmptyBaseURL(t *testing.T) {
	client, err := NewTracerClient("")

	require.Error(t, err)
	require.Nil(t, client)
}

func TestTracerClient_PostTimeoutReturnsUnavailable(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release // hold the request open until the operation deadline trips
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	defer close(release)

	client, err := NewTracerClient(srv.URL, WithOperationTimeout(20*time.Millisecond))
	require.NoError(t, err)

	resp, err := client.post(context.Background(), "/v1/reservations", []byte(`{}`))
	if resp != nil {
		_ = resp.Body.Close()
	}

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTracerUnavailable)
}
