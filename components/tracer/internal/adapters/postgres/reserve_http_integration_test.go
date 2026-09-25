// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	grpcin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in"
	grpcmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in/mocks"
	httpin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	contractpb "github.com/LerianStudio/midaz/v4/pkg/tracercontract/protobuf"
)

// Identity is supplied by the fixture at the already-authenticated boundary.
// This exercises Fiber/Huma, gRPC/protobuf and real transactional repositories;
// certificate verification and the Ledger binary are outside this test.
func TestIntegrationReserveHTTPPersistsAndReplays(t *testing.T) {
	db := completionDatabase(t)
	admission, policies, request := admissionFixture(t, db)
	admissionPolicy(t, db, policies, model.DecisionAllow)
	limit := admissionLimit(t, db, request, 89601, "100")
	completion, decisions, _ := completionCommand(t, db, true)
	byID, err := command.NewCompleteReserveReservationCommand(decisions, completion, true)
	require.NoError(t, err)
	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	handler, err := httpin.NewContextReservationHandler(admission, completion, byID, bounds, 65536, 100)
	require.NoError(t, err)
	problem.Install()
	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	t.Cleanup(func() { require.NoError(t, app.Shutdown()) })
	group := app.Group("/v1", func(c fiber.Ctx) error { c.SetContext(completionContext(c.Context(), "producer")); return c.Next() })
	api := openapi.New(app, group, openapi.Config{Title: "reservation persistence test", Version: "1", Servers: []string{"/v1"}})
	pkgHTTP.InstallSchemaNamer(api)
	httpin.RegisterContextReservationRoutes(api, handler, nil)
	post := func(path string, body any, status int) []byte {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		input := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		input.Header.Set("Content-Type", "application/json")
		response, err := app.Test(input)
		require.NoError(t, err)
		defer response.Body.Close()
		result, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, status, response.StatusCode, string(result))
		return result
	}
	raw := post("/v1/reservations", request, http.StatusCreated)
	first, err := tracercontract.DecodeReserveResultJSON(t.Context(), raw, 65536, 100)
	require.NoError(t, err)
	require.NoError(t, first.ValidateFor(request, 100))
	require.Len(t, first.ReservationIDs, 1)
	require.JSONEq(t, string(raw), string(post("/v1/reservations", request, http.StatusCreated)))
	// Replay across transports must resolve the original durable decision.
	server, err := grpcin.NewContextReservationServer(grpcmocks.NewMockReservationService(gomock.NewController(t)), testutil.NewDefaultMockClock(), admission, completion, byID, grpcin.ContextReservationConfig{Bounds: bounds, MaxBodyBytes: 65536, MaxReservations: 100})
	require.NoError(t, err)
	client := reservePersistenceGRPCClient(t, server)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	wire, err := contractpb.EncodeReserve(ctx, request, request.Asset.Namespace, bounds)
	require.NoError(t, err)
	wireResult, err := client.Reserve(ctx, wire)
	require.NoError(t, err)
	grpcResult, err := contractpb.DecodeResult(wireResult, 100)
	require.NoError(t, err)
	require.Equal(t, first, grpcResult)
	confirmed, err := client.ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{ContractRevision: request.ContractRevision, TransactionId: request.TransactionID.String()})
	require.NoError(t, err)
	require.EqualValues(t, 1, confirmed.GetFlipped())
	body := map[string]string{"contractRevision": tracercontract.ReserveContractRevision}
	confirmPath := "/v1/reservations/transaction/" + request.TransactionID.String() + "/confirm"
	post(confirmPath, body, http.StatusOK) // Treat this acknowledgement as lost.
	replay := post(confirmPath, body, http.StatusOK)
	completed, err := tracercontract.DecodeTransactionCompletionJSON(t.Context(), replay, 65536)
	require.NoError(t, err)
	require.NoError(t, completed.Validate())
	require.Zero(t, completed.Flipped)
	post("/v1/reservations/transaction/"+request.TransactionID.String()+"/release", body, http.StatusConflict)
	request.Amount = "11"
	post("/v1/reservations", request, http.StatusConflict)
	current, held := readCounterDecimal(t, db, limit, "acct:"+request.Context.Accounts[0].ID.String(), testutil.FixedTime().Format("2006-01-02"))
	require.Equal(t, "10.125", current.String())
	require.True(t, held.IsZero())
	require.Len(t, completionEvents(t, db, request.TransactionID), 2)
}

// The interceptor supplies only the already-verified fixture identity. Native
// mTLS authentication has separate handshake tests; this fixture tests transport
// serialization against the same durable state, without mocking commands.
func reservePersistenceGRPCClient(t *testing.T, handler *grpcin.ReservationServer) reservationv1.ReservationServiceClient {
	t.Helper()
	listener := bufconn.Listen(65536)
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, request any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		return next(completionContext(ctx, "producer"), request)
	}))
	reservationv1.RegisterReservationServiceServer(server, handler)
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		require.NoError(t, listener.Close())
		require.NoError(t, <-served)
	})
	connection, err := grpc.NewClient("passthrough:///reservation-persistence", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.DialContext(ctx)
	}))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	return reservationv1.NewReservationServiceClient(connection)
}
