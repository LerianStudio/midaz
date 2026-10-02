// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

// Cross-component proof of the reservation seam identities. Each case boots
// the real tracer (bootstrap.InitServers over the suite's Postgres container)
// with one seam identity and drives the real :4021 listener with the exact
// gRPC metadata the ledger's reservation client sends:
//
//   - token identity: "authorization: Bearer <application token>";
//   - API key identity: "x-api-key: <API_KEY>";
//   - no identity: no credential metadata at all.
//
// The ledger's *TracerGRPCClient cannot drive this listener from here: it
// lives under components/ledger/internal, and Go's internal rule admits it
// only to components/ledger/..., while the tracer bootstrap that boots the
// real seam lives under components/tracer/internal. No package can import
// both, so the client half is the generated reservation client plus the
// ledger's wire metadata, and the ledger half is proven on its own side:
// grpc_client_credentials_test.go and grpc_client_test.go in the ledger's
// adapters/tracer package map the two codes asserted here
// (Unauthenticated, PermissionDenied) to ErrTracerUnauthorized and attach the
// same metadata keys.
//
// The multi-tenant variant is proven over bufconn by
// reservation_e2e_auth_test.go (the tenant resolver receives the claim's
// tenant); it is not repeated here because a multi-tenant boot also needs a
// primed per-tenant rule cache before any reserve is answered.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	testutil_integration "github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_integration"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// Ledger application clients the fake Access Manager knows. Both hold the
// tracer/reservations grant; only seamLedgerClient is allowlisted on the seam.
const (
	seamLedgerClient   = "lerian/midaz-ledger-seam"
	seamOutsiderClient = "lerian/other-application"
)

// seamCallTimeout bounds one seam RPC so a stalled listener fails the test.
const seamCallTimeout = 10 * time.Second

// seamAccessManager stands in for the Access Manager: /health answers
// "healthy" and /v1/authorize grants tracer/reservations to the granted
// subjects, refusing everything else with 403. It records the subjects it
// authorized.
type seamAccessManager struct {
	server  *httptest.Server
	granted map[string]bool

	mu   sync.Mutex
	subs []string
}

func newSeamAccessManager(t *testing.T, granted ...string) *seamAccessManager {
	t.Helper()

	am := &seamAccessManager{granted: map[string]bool{}}
	for _, sub := range granted {
		am.granted[sub] = true
	}

	am.server = httptest.NewServer(http.HandlerFunc(am.serve))
	t.Cleanup(am.server.Close)

	return am
}

func (am *seamAccessManager) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health":
		_, _ = w.Write([]byte("healthy"))
	case "/v1/authorize":
		var body struct {
			Sub      string `json:"sub"`
			Resource string `json:"resource"`
			Action   string `json:"action"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		am.mu.Lock()
		am.subs = append(am.subs, body.Sub)
		am.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if !am.granted[body.Sub] || body.Resource != "reservations" || body.Action != "post" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":"AUT-1003","title":"Forbidden","message":"insufficient privileges"}`))

			return
		}

		_, _ = w.Write([]byte(`{"authorized":true,"timestamp":"2026-10-01T12:00:00Z"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (am *seamAccessManager) authorizedSubjects() []string {
	am.mu.Lock()
	defer am.mu.Unlock()

	return append([]string(nil), am.subs...)
}

// applicationToken returns an Access Manager-shaped application token for
// sub. The seam does not verify the signature locally (AUTH_JWT_VERIFY_CERT
// is unset); the Access Manager round trip is the verification.
func applicationToken(t *testing.T, sub string) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "application",
		"sub":  sub,
	}).SignedString([]byte("seam-identity-test-signing-key"))
	require.NoError(t, err)

	return token
}

// ledgerSeamClient dials the seam the way the ledger does in plaintext mode
// and attaches pairs (the ledger's credential metadata) to every RPC.
func ledgerSeamClient(t *testing.T, pairs ...string) reservationv1.ReservationServiceClient {
	t.Helper()

	withCredential := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if len(pairs) > 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}

	conn, err := grpc.NewClient(
		testutil.GetGRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(withCredential),
	)
	require.NoError(t, err)

	t.Cleanup(func() { _ = conn.Close() })

	return reservationv1.NewReservationServiceClient(conn)
}

// seamReserve reserves 10 BRL for accountID under transactionID.
func seamReserve(client reservationv1.ReservationServiceClient, transactionID, accountID uuid.UUID) (*reservationv1.ReserveResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), seamCallTimeout)
	defer cancel()

	return client.Reserve(ctx, &reservationv1.ReserveRequest{
		TransactionId:        transactionID.String(),
		RequestId:            uuid.New().String(),
		Amount:               "10",
		Asset:                "BRL",
		TransactionTimestamp: testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		Account: &reservationv1.ReserveAccount{
			AccountId: accountID.String(),
			Type:      "deposit",
		},
	})
}

// seamConfirmByTransaction confirms every reservation of transactionID.
func seamConfirmByTransaction(client reservationv1.ReservationServiceClient, transactionID uuid.UUID) (*reservationv1.ConfirmByTransactionResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), seamCallTimeout)
	defer cancel()

	return client.ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{
		TransactionId: transactionID.String(),
	})
}

// restartSeam reboots the tracer with env and restores the suite's server
// when the test ends.
func restartSeam(t *testing.T, env map[string]string) {
	t.Helper()

	cleanup, err := testutil_integration.RestartServerWithConfig(env)
	require.NoError(t, err, "boot the tracer with the seam identity under test")

	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("restore the suite's tracer: %v", err)
		}
	})
}

// TestIntegration_ReservationSeam_TokenIdentity boots the seam with the
// Access Manager token identity in single-tenant mode and proves the ledger's
// application token reserves and confirms, while a granted application
// outside TRACER_SEAM_ALLOWED_CLIENTS and a call without a token are refused
// with the codes the ledger maps to ErrTracerUnauthorized.
func TestIntegration_ReservationSeam_TokenIdentity(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(96401)
	createActiveAccountLimit(t, accountID, "1000")

	am := newSeamAccessManager(t, seamLedgerClient, seamOutsiderClient)

	restartSeam(t, map[string]string{
		"PLUGIN_AUTH_ENABLED":         "true",
		"PLUGIN_AUTH_ADDRESS":         am.server.URL,
		"AUTH_M2M_INVERSION_ENABLED":  "true",
		"AUTH_CACHE_TTL":              "",
		"TRACER_SEAM_ALLOWED_CLIENTS": seamLedgerClient,
		"TRACER_TLS_MODE":             "",
		"DEPLOYMENT_MODE":             "local",
	})

	t.Run("allowlisted application token reserves then confirms by transaction", func(t *testing.T) {
		client := ledgerSeamClient(t, "authorization", "Bearer "+applicationToken(t, seamLedgerClient))
		transactionID := uuid.New()

		reserved, err := seamReserve(client, transactionID, accountID)
		require.NoError(t, err, "the ledger's token must reach the reservation service")
		require.False(t, reserved.GetDenied())
		require.Len(t, reserved.GetReservationIds(), 1, "the account-scoped limit holds one reservation")

		confirmed, err := seamConfirmByTransaction(client, transactionID)
		require.NoError(t, err, "the same token must confirm by transaction")
		assert.Equal(t, uint32(1), confirmed.GetConfirmed())

		assert.Contains(t, am.authorizedSubjects(), seamLedgerClient, "the token authorizes under the ledger's own client id")
	})

	t.Run("granted application outside the allowlist is permission denied", func(t *testing.T) {
		client := ledgerSeamClient(t, "authorization", "Bearer "+applicationToken(t, seamOutsiderClient))

		_, err := seamReserve(client, uuid.New(), accountID)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("call without a token is unauthenticated", func(t *testing.T) {
		_, err := seamReserve(ledgerSeamClient(t), uuid.New(), accountID)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}

// TestIntegration_ReservationSeam_APIKeyIdentity drives the suite's own boot,
// which runs the seam under the API key identity (API_KEY_ENABLED=true,
// PLUGIN_AUTH_ENABLED=false): the right key reserves, a wrong or missing key
// is unauthenticated.
func TestIntegration_ReservationSeam_APIKeyIdentity(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(96402)

	t.Run("right API key reserves", func(t *testing.T) {
		reserved, err := seamReserve(ledgerSeamClient(t, "x-api-key", testutil.GetAPIKey()), uuid.New(), accountID)
		require.NoError(t, err, "the ledger's TRACER_API_KEY must reach the reservation service")
		assert.False(t, reserved.GetDenied())
	})

	t.Run("wrong API key is unauthenticated", func(t *testing.T) {
		_, err := seamReserve(ledgerSeamClient(t, "x-api-key", "not-the-tracer-api-key"), uuid.New(), accountID)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("missing API key is unauthenticated", func(t *testing.T) {
		_, err := seamReserve(ledgerSeamClient(t), uuid.New(), accountID)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}

// TestIntegration_ReservationSeam_NoIdentityBYOC boots a BYOC tracer with no
// seam identity (neither auth mechanism, plaintext transport): the seam boots
// and a call carrying no credential is served.
func TestIntegration_ReservationSeam_NoIdentityBYOC(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(96403)

	restartSeam(t, map[string]string{
		"PLUGIN_AUTH_ENABLED": "false",
		"API_KEY_ENABLED":     "false",
		"TRACER_TLS_MODE":     "",
		"DEPLOYMENT_MODE":     "byoc",
	})

	reserved, err := seamReserve(ledgerSeamClient(t), uuid.New(), accountID)
	require.NoError(t, err, "an identity-less BYOC seam serves a call without credentials")
	assert.False(t, reserved.GetDenied())
}
