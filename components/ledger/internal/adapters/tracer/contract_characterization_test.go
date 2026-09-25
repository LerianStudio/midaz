// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// This baseline intentionally records the current flat scope fields. The Tracer
// adapter characterization consumes this same fixture and proves the REST loss.
// The replacement gRPC contract rejects this legacy shape instead of inventing facts.
func TestReserveCharacterizationScopedLedgerWire(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/reserve_scoped.json")
	require.NoError(t, err)
	var input ReserveRequest
	require.NoError(t, json.Unmarshal(fixture, &input))
	captured := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/reservations" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		captured <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{TransactionID: input.TransactionID})
	}))
	defer server.Close()
	client, err := NewTracerClient(server.URL)
	require.NoError(t, err)
	result, err := client.Reserve(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, input.TransactionID, result.TransactionID)
	require.JSONEq(t, string(fixture), string(<-captured))
}
