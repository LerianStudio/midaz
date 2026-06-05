// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedTransactionID and fixedReservationID are deterministic UUID literals so
// the tests carry no uuid.New() randomness.
var (
	fixedTransactionID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fixedReservationID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

// stubM2MProvider returns a fixed token and records that GetToken was called.
type stubM2MProvider struct {
	token  string
	called bool
}

func (s *stubM2MProvider) GetToken(_ context.Context) (string, error) {
	s.called = true
	return s.token, nil
}

func TestNewTracerClient_RejectsEmptyBaseURL(t *testing.T) {
	client, err := NewTracerClient("")

	require.Error(t, err)
	require.Nil(t, client)
}

func TestTracerClient_Reserve_201ParsesHandle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/reservations", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body ReserveRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, fixedTransactionID, body.TransactionID)

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{
			TransactionID:  fixedTransactionID,
			Denied:         false,
			ReservationIDs: []uuid.UUID{fixedReservationID},
		})
	}))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	result, err := client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Denied)
	require.Len(t, result.ReservationIDs, 1)
	assert.Equal(t, fixedReservationID, result.ReservationIDs[0])
}

func TestTracerClient_Reserve_DeniedIsSuccessfulReturn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{
			TransactionID:  fixedTransactionID,
			Denied:         true,
			ReservationIDs: []uuid.UUID{},
		})
	}))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	result, err := client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	// A DENIED decision is a successful Reserve return, NOT an error.
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Denied)
	assert.Empty(t, result.ReservationIDs)
}

func TestTracerClient_Reserve_TimeoutReturnsUnavailable(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release // hold the request open until the test deadline trips
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	defer close(release)

	client, err := NewTracerClient(srv.URL, WithOperationTimeout(20*time.Millisecond))
	require.NoError(t, err)

	result, err := client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	require.Error(t, err)
	require.Nil(t, result)
	assert.ErrorIs(t, err, ErrTracerUnavailable)
}

func TestTracerClient_Reserve_AttachesM2MHeaderWhenProviderSet(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{TransactionID: fixedTransactionID, ReservationIDs: []uuid.UUID{}})
	}))
	defer srv.Close()

	provider := &stubM2MProvider{token: "test-jwt"}
	client, err := NewTracerClient(srv.URL, WithM2MTokenProvider(provider))
	require.NoError(t, err)

	_, err = client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	require.NoError(t, err)
	assert.True(t, provider.called)
	assert.Equal(t, "Bearer test-jwt", gotAuth)
}

func TestTracerClient_Reserve_NoAuthHeaderWhenProviderNil(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{TransactionID: fixedTransactionID, ReservationIDs: []uuid.UUID{}})
	}))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	_, err = client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	require.NoError(t, err)
	assert.False(t, hadAuth)
}

func TestTracerClient_Reserve_NonCreatedStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	result, err := client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	require.Error(t, err)
	require.Nil(t, result)
	// A non-2xx status is NOT an availability failure — the anchor treats it
	// distinctly from ErrTracerUnavailable.
	assert.NotErrorIs(t, err, ErrTracerUnavailable)
}

func TestTracerClient_Confirm_200Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/reservations/"+fixedReservationID.String()+"/confirm", r.URL.Path)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"reservationId": fixedReservationID.String(), "status": "CONFIRMED"})
	}))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	require.NoError(t, client.Confirm(context.Background(), fixedReservationID))
}

func TestTracerClient_Release_200Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/reservations/"+fixedReservationID.String()+"/release", r.URL.Path)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"reservationId": fixedReservationID.String(), "status": "RELEASED"})
	}))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	require.NoError(t, client.Release(context.Background(), fixedReservationID))
}

func TestTracerClient_Confirm_TimeoutReturnsUnavailable(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(release)

	client, err := NewTracerClient(srv.URL, WithOperationTimeout(20*time.Millisecond))
	require.NoError(t, err)

	err = client.Confirm(context.Background(), fixedReservationID)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTracerUnavailable)
}

// recordingExecutor is a CircuitBreakerExecutor stub that either runs the
// operation or short-circuits with an open-breaker error.
type recordingExecutor struct {
	name     string
	openErr  error
	executed bool
}

func (e *recordingExecutor) Execute(name string, fn func() (any, error)) (any, error) {
	e.name = name

	if e.openErr != nil {
		return nil, e.openErr
	}

	e.executed = true

	return fn()
}

func TestTracerClient_Reserve_RoutesThroughCircuitBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{TransactionID: fixedTransactionID, ReservationIDs: []uuid.UUID{}})
	}))
	defer srv.Close()

	exec := &recordingExecutor{}
	client, err := NewTracerClient(srv.URL, WithCircuitBreaker(exec))
	require.NoError(t, err)

	_, err = client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	require.NoError(t, err)
	assert.True(t, exec.executed)
	assert.Equal(t, breakerName, exec.name)
}

func TestTracerClient_Reserve_OpenBreakerReturnsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be hit when the breaker is open")
	}))
	defer srv.Close()

	exec := &recordingExecutor{openErr: errors.New("circuit breaker is open")}
	client, err := NewTracerClient(srv.URL, WithCircuitBreaker(exec))
	require.NoError(t, err)

	result, err := client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	require.Error(t, err)
	require.Nil(t, result)
	assert.ErrorIs(t, err, ErrTracerUnavailable)
	assert.False(t, exec.executed)
}
