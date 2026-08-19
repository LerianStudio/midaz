// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
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

func TestNewTracerClient_RejectsEmptyBaseURL(t *testing.T) {
	client, err := NewTracerClient("")

	require.Error(t, err)
	require.Nil(t, client)
}

func TestTracerClient_Reserve_201ParsesHandle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/reservations/ledger-outcome-v2", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body ReserveRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, fixedTransactionID, body.TransactionID)

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{
			TransactionID:  fixedTransactionID,
			Denied:         false,
			ReservationIDs: []uuid.UUID{fixedReservationID},
			DeliveryMode:   DeliveryModeLedgerOutcomeV2,
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
		DeliveryMode:  DeliveryModeLedgerOutcomeV2,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Denied)
	assert.Equal(t, DeliveryModeLedgerOutcomeV2, result.DeliveryMode)
	require.Len(t, result.ReservationIDs, 1)
	assert.Equal(t, fixedReservationID, result.ReservationIDs[0])
}

func TestTracerClient_ReserveV2NeverFallsBackToLegacyEndpoint(t *testing.T) {
	legacySideEffects := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/reservations" {
			legacySideEffects++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(ReserveResult{TransactionID: fixedTransactionID})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	result, err := client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		DeliveryMode:  DeliveryModeLedgerOutcomeV2,
	})
	require.Error(t, err)
	require.Nil(t, result)
	assert.Zero(t, legacySideEffects, "a V2 caller must fail before an old server can create a legacy hold")
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

func TestTracerClient_ApplyOutcomePreservesReceiptAndTenant(t *testing.T) {
	outcomeID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	var gotTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/reservations/transaction/"+fixedTransactionID.String()+"/outcome", r.URL.Path)
		gotTenant = r.Header.Get(TenantHeader)
		var body ApplyOutcomeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, outcomeID, body.OutcomeID)
		assert.Equal(t, ReservationOutcomeCommitted, body.Outcome)
		_ = json.NewEncoder(w).Encode(ApplyOutcomeResult{
			TransactionID: fixedTransactionID, OutcomeID: outcomeID, Outcome: ReservationOutcomeCommitted,
			ReservationCount: 2, Replayed: true,
		})
	}))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-007")
	result, err := client.ApplyOutcome(ctx, ApplyOutcomeRequest{
		TransactionID: fixedTransactionID, OutcomeID: outcomeID, Outcome: ReservationOutcomeCommitted,
	})
	require.NoError(t, err)
	assert.Equal(t, "tenant-007", gotTenant)
	assert.Equal(t, 2, result.ReservationCount)
	assert.True(t, result.Replayed)
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

// TestTracerClient_Reserve_SendsConfiguredAPIKey pins the REST seam's
// application credential in addition to mTLS. The key is sent only in the
// dedicated X-API-Key header; it is never reused as a Bearer token.
func TestTracerClient_Reserve_SendsConfiguredAPIKey(t *testing.T) {
	const apiKey = "ledger-to-tracer-secret"
	var gotAPIKey string
	var hadAuthorization bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get(APIKeyHeader)
		_, hadAuthorization = r.Header["Authorization"]

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{TransactionID: fixedTransactionID, ReservationIDs: []uuid.UUID{}})
	}))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL, WithAPIKey(apiKey))
	require.NoError(t, err)

	_, err = client.Reserve(context.Background(), ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	require.NoError(t, err)
	assert.Equal(t, apiKey, gotAPIKey)
	assert.False(t, hadAuthorization)
}

func TestTracerClient_Reserve_AuthenticatedGuardRejectsThenAcceptsConfiguredKey(t *testing.T) {
	const apiKey = "ledger-to-tracer-secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(APIKeyHeader) != apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{TransactionID: fixedTransactionID})
	}))
	defer srv.Close()

	unauthenticated, err := NewTracerClient(srv.URL)
	require.NoError(t, err)
	_, err = unauthenticated.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
	require.ErrorContains(t, err, "status 401")
	require.NotContains(t, err.Error(), apiKey)

	authenticated, err := NewTracerClient(srv.URL, WithAPIKey(apiKey))
	require.NoError(t, err)
	_, err = authenticated.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
	require.NoError(t, err)
}

// TestTracerClient_Reserve_SetsTenantHeader pins trusted tenant propagation on
// the REST transport: when the request context carries a tenant (multi-tenant
// mode), the client sends it as the X-Tenant-Id header the tracer trusts over
// the mTLS-verified connection.
func TestTracerClient_Reserve_SetsTenantHeader(t *testing.T) {
	var gotTenant string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = r.Header.Get(TenantHeader)

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ReserveResult{TransactionID: fixedTransactionID, ReservationIDs: []uuid.UUID{}})
	}))
	defer srv.Close()

	client, err := NewTracerClient(srv.URL)
	require.NoError(t, err)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-007")

	_, err = client.Reserve(ctx, ReserveRequest{
		TransactionID: fixedTransactionID,
		Amount:        "100",
		Currency:      "USD",
		Account:       ReserveAccount{AccountID: "acc-1"},
	})

	require.NoError(t, err)
	assert.Equal(t, "tenant-007", gotTenant)
}

// TestTracerClient_Reserve_OmitsTenantHeaderWhenAbsent pins single-tenant mode:
// when the context carries no tenant, the client sends no X-Tenant-Id header at
// all (the tracer then runs its single-tenant pass-through), rather than an
// empty-valued header.
func TestTracerClient_Reserve_OmitsTenantHeaderWhenAbsent(t *testing.T) {
	var hadTenant bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadTenant = r.Header[TenantHeader]

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
	assert.False(t, hadTenant)
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
