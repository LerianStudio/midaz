// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

// This is the cross-component contract lock between the ledger's outbound
// reserve client and the tracer's reserve validation. The two shapes once
// drifted silently (the ledger omitted requestId and a valid
// transactionTimestamp, and smuggled a lifetime hint through transactionType),
// so the tracer rejected every reserve and `tracer.mode=enforce` never
// enforced. This lock fails when either side moves.
//
// What is REAL on each side here:
//   - LEDGER: the real *TracerGRPCClient.Reserve — the production mapping of
//     the outbound ReserveRequest onto the proto message and the real RPC.
//   - TRACER: the real github.com/.../tracer/pkg/model.ValidationRequest plus
//     the real NormalizeAndValidateForReserve validation rules — the side that
//     rejects a drifted payload. The gRPC endpoint below runs the SAME
//     proto-to-model mapping and the SAME validation the production tracer
//     reservation server runs; on success it returns the scripted reserve
//     decision, exactly as the server does.
//
// Why the endpoint is reconstructed rather than the literal tracer server:
// Go's `internal` rule walls components/tracer/internal/... off from
// components/ledger/..., so the ledger test package physically cannot import the
// tracer's internal ReservationServer. The load-bearing half of the contract —
// the proto message and the reserve validation RULES — lives in the importable
// pkg/proto and the tracer's non-internal pkg/model, so both real sides meet
// over the real wire.

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	tracermodel "github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// tracerReserveEndpoint is the real tracer reserve validation mounted over an
// in-memory gRPC server. It maps the proto request onto the tracer's REAL
// ValidationRequest and runs the tracer's REAL reserve validation rules
// (NormalizeAndValidateForReserve) — the same mapping + validation the
// production server runs. denied/reservationIDs let a test script the
// post-validation decision so the success-decision-flows-back assertion is
// meaningful.
type tracerReserveEndpoint struct {
	reservationv1.UnimplementedReservationServiceServer

	now            time.Time
	denied         bool
	decision       string
	reason         string
	matchedRuleIDs []uuid.UUID
	reservationIDs []uuid.UUID
	// rejectCode, when not OK, answers every well-formed request with this
	// status, standing in for a tracer-side refusal the relaxed validation in
	// this package cannot reproduce.
	rejectCode codes.Code

	parsed   bool // set true once a request successfully mapped + validated
	received *tracermodel.ValidationRequest
}

func (e *tracerReserveEndpoint) Reserve(_ context.Context, req *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
	transactionID, err := uuid.Parse(req.GetTransactionId())
	if err != nil || transactionID == uuid.Nil {
		return nil, status.Error(codes.InvalidArgument, "transactionId is required")
	}

	validationReq, err := reserveValidationRequest(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid reserve request: "+err.Error())
	}

	// The REAL tracer reserve validation rules.
	if err := validationReq.NormalizeAndValidateForReserve(e.now); err != nil {
		return nil, status.Error(codes.InvalidArgument, "reserve validation failed: "+err.Error())
	}

	if e.rejectCode != codes.OK {
		return nil, status.Error(e.rejectCode, "reserve refused")
	}

	e.parsed = true
	e.received = validationReq

	return &reservationv1.ReserveResult{
		TransactionId:  transactionID.String(),
		Denied:         e.denied,
		Decision:       e.decision,
		Reason:         e.reason,
		MatchedRuleIds: uuidStrings(e.matchedRuleIDs),
		ReservationIds: uuidStrings(e.reservationIDs),
	}, nil
}

// reserveValidationRequest is the proto-to-model mapping the tracer's
// reservation server performs before validating: requestId, amount
// (decimal-as-string), asset, account id and type, optional
// segment/portfolio/merchant ids, transactionType, transactionTimestamp
// (RFC3339) and flat metadata. An empty account id is an absent account.
func reserveValidationRequest(req *reservationv1.ReserveRequest) (*tracermodel.ValidationRequest, error) {
	requestID, err := uuid.Parse(req.GetRequestId())
	if err != nil {
		return nil, err
	}

	amount, err := safe.ParseDecimal(req.GetAmount())
	if err != nil {
		return nil, err
	}

	var transactionTimestamp time.Time
	if ts := req.GetTransactionTimestamp(); ts != "" {
		transactionTimestamp, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, err
		}
	}

	var accountID uuid.UUID
	if acc := req.GetAccount(); acc != nil && acc.GetAccountId() != "" {
		accountID, err = uuid.Parse(acc.GetAccountId())
		if err != nil {
			return nil, err
		}
	}

	var metadata map[string]any
	if len(req.GetMetadata()) > 0 {
		metadata = make(map[string]any, len(req.GetMetadata()))
		for key, value := range req.GetMetadata() {
			metadata[key] = value
		}
	}

	validationReq := &tracermodel.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      tracermodel.TransactionType(req.GetTransactionType()),
		Amount:               amount,
		Asset:                req.GetAsset(),
		TransactionTimestamp: transactionTimestamp,
		Account:              tracermodel.AccountContext{ID: accountID, Type: req.GetAccount().GetType()},
		Metadata:             metadata,
	}

	if segment, err := optionalContextID(req.GetSegmentId()); err != nil {
		return nil, err
	} else if segment != nil {
		validationReq.Segment = &tracermodel.SegmentContext{ID: *segment}
	}

	if portfolio, err := optionalContextID(req.GetPortfolioId()); err != nil {
		return nil, err
	} else if portfolio != nil {
		validationReq.Portfolio = &tracermodel.PortfolioContext{ID: *portfolio}
	}

	if merchant, err := optionalContextID(req.GetMerchantId()); err != nil {
		return nil, err
	} else if merchant != nil {
		validationReq.Merchant = &tracermodel.MerchantContext{ID: *merchant}
	}

	return validationReq, nil
}

// optionalContextID parses an optional scope id: an empty string is absent, a
// present-but-malformed value is rejected.
func optionalContextID(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}

	return &id, nil
}

// uuidStrings renders ids as the proto's repeated string field.
func uuidStrings(ids []uuid.UUID) []string {
	if len(ids) == 0 {
		return nil
	}

	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}

	return out
}

// newContractClient serves the reconstructed tracer on an in-memory bufconn
// listener and returns the real ledger client, built through its production
// constructor, dialed to it. The server stops and the client closes via
// t.Cleanup.
func newContractClient(t *testing.T, server reservationv1.ReservationServiceServer) *TracerGRPCClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)

	srv := grpc.NewServer()
	reservationv1.RegisterReservationServiceServer(srv, server)

	go func() { _ = srv.Serve(lis) }()

	client, err := NewTracerGRPCClient("passthrough:///bufnet",
		WithGRPCOperationTimeout(5*time.Second),
		WithGRPCDialOptions(
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		))
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = client.Close()
		srv.Stop()
		_ = lis.Close()
	})

	return client
}

// ledgerStyleReserveRequest builds the reserve request the ledger anchor sends:
// requestId derived from the transactionID, the structured account scope, a
// fee-inclusive amount/asset, and a timestamp. ts must not be in the future
// relative to the endpoint's now; the reserve path does not bound its age.
func ledgerStyleReserveRequest(transactionID, requestID uuid.UUID, ts time.Time) ReserveRequest {
	return ReserveRequest{
		TransactionID:        transactionID,
		RequestID:            requestID.String(),
		Amount:               "1000",
		Asset:                "BRL",
		Account:              ReserveAccount{AccountID: uuid.NewString()},
		TransactionTimestamp: ts.UTC().Format(time.RFC3339Nano),
	}
}

// TestReserveContract_LedgerPayloadAcceptedByTracer is the contract lock: the
// real ledger client's outbound reserve request must be ACCEPTED (no
// InvalidArgument) by the real tracer reserve validation, and the reserve
// decision must flow back. This fails if either the ledger outbound shape or
// the tracer reserve validation drifts apart.
func TestReserveContract_LedgerPayloadAcceptedByTracer(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	reservationID := uuid.MustParse("99999999-9999-9999-9999-999999999999")

	endpoint := &tracerReserveEndpoint{now: now, reservationIDs: []uuid.UUID{reservationID}}
	client := newContractClient(t, endpoint)

	txID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	reqID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	result, err := client.Reserve(context.Background(), ledgerStyleReserveRequest(txID, reqID, now))

	require.NoError(t, err, "the tracer must ACCEPT the ledger reserve request; a rejection here is the contract gap reappearing")
	require.True(t, endpoint.parsed, "the tracer must have mapped + validated the ledger request")

	// The reserve decision flows back.
	require.NotNil(t, result)
	assert.Equal(t, txID, result.TransactionID)
	assert.False(t, result.Denied)
	require.Len(t, result.ReservationIDs, 1)
	assert.Equal(t, reservationID, result.ReservationIDs[0])
}

// TestReserveContract_DeniedDecisionFlowsBack proves a DENIED decision (a
// successful reserve) round-trips as a business outcome, not a transport error.
func TestReserveContract_DeniedDecisionFlowsBack(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	endpoint := &tracerReserveEndpoint{now: now, denied: true}
	client := newContractClient(t, endpoint)

	txID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	reqID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	result, err := client.Reserve(context.Background(), ledgerStyleReserveRequest(txID, reqID, now))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Denied, "a DENIED limit decision must round-trip as a successful reserve result")
	assert.Empty(t, result.ReservationIDs)
}

// TestReserveContract_AccountlessLedgerPayloadAccepted proves the relaxation:
// the ledger may reserve for an external-only source with no internal account
// UUID. An empty account must still be ACCEPTED (matches non-account-scoped
// limits) rather than rejected.
func TestReserveContract_AccountlessLedgerPayloadAccepted(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	endpoint := &tracerReserveEndpoint{now: now}
	client := newContractClient(t, endpoint)

	req := ledgerStyleReserveRequest(
		uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		uuid.MustParse("66666666-6666-6666-6666-666666666666"),
		now,
	)
	req.Account = ReserveAccount{} // external-only source: no internal account UUID

	result, err := client.Reserve(context.Background(), req)

	require.NoError(t, err, "an accountless reserve must be accepted on the relaxed reserve path")
	require.True(t, endpoint.parsed)
	require.NotNil(t, result)
	assert.Equal(t, uuid.Nil, endpoint.received.Account.ID)
}

// TestReserveContract_AccountTypeAndMetadataLandOnTracerModel proves the
// ledger's account type and flat metadata land on the tracer's real model
// fields rather than being silently dropped, and that a metadata key the
// tracer refuses reaches the caller as a rejection of the request, never as an
// unavailable tracer.
func TestReserveContract_AccountTypeAndMetadataLandOnTracerModel(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	endpoint := &tracerReserveEndpoint{now: now}
	client := newContractClient(t, endpoint)

	req := ledgerStyleReserveRequest(
		uuid.MustParse("88888888-8888-8888-8888-888888888888"),
		uuid.MustParse("99999999-9999-9999-9999-999999999990"),
		now,
	)
	req.Account.Type = "deposit"
	req.Metadata = map[string]string{"channel": "app"}

	_, err := client.Reserve(context.Background(), req)
	require.NoError(t, err)
	require.True(t, endpoint.parsed)
	assert.Equal(t, "deposit", endpoint.received.Account.Type)
	assert.Equal(t, map[string]any{"channel": "app"}, endpoint.received.Metadata)

	endpoint.parsed = false
	req.Metadata = map[string]string{"bad-key": "app"}

	_, err = client.Reserve(context.Background(), req)
	require.Error(t, err)
	assert.False(t, endpoint.parsed)
	assert.ErrorIs(t, err, ErrTracerRejected)
	assert.NotErrorIs(t, err, ErrTracerUnavailable)
}

// TestReserveContract_RejectedIsNotUnavailable proves a tracer refusal of the
// request (FailedPrecondition) reaches the caller as ErrTracerRejected and
// never as ErrTracerUnavailable, so the anchor does not route it through
// failPosture.
func TestReserveContract_RejectedIsNotUnavailable(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	endpoint := &tracerReserveEndpoint{now: now, rejectCode: codes.FailedPrecondition}
	client := newContractClient(t, endpoint)

	result, err := client.Reserve(context.Background(), ledgerStyleReserveRequest(
		uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
		now,
	))

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrTracerRejected)
	assert.NotErrorIs(t, err, ErrTracerUnavailable)
}

// TestReserveContract_ReviewDecisionFlowsBack proves the refined decision, its
// reason and the matched rule ids round-trip next to denied.
func TestReserveContract_ReviewDecisionFlowsBack(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	ruleID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")

	endpoint := &tracerReserveEndpoint{
		now:            now,
		denied:         true,
		decision:       "REVIEW",
		reason:         "manual review required",
		matchedRuleIDs: []uuid.UUID{ruleID},
	}
	client := newContractClient(t, endpoint)

	result, err := client.Reserve(context.Background(), ledgerStyleReserveRequest(
		uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd"),
		uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"),
		now,
	))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Denied)
	assert.Equal(t, "REVIEW", result.Decision)
	assert.Equal(t, "manual review required", result.Reason)
	assert.Equal(t, []uuid.UUID{ruleID}, result.MatchedRuleIDs)
	assert.Empty(t, result.ReservationIDs)
}

// legacyAccountTypes is the closed account-type enum a tracer released before
// the free-form account type accepted on reserve.
var legacyAccountTypes = map[string]bool{"checking": true, "savings": true, "credit": true}

// legacyTracerEndpoint wraps the current endpoint with the closed account-type
// enum an older tracer enforced before mapping the request.
type legacyTracerEndpoint struct {
	reservationv1.UnimplementedReservationServiceServer

	current *tracerReserveEndpoint
}

func (e *legacyTracerEndpoint) Reserve(ctx context.Context, req *reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
	if accountType := req.GetAccount().GetType(); accountType != "" && !legacyAccountTypes[accountType] {
		return nil, status.Error(codes.InvalidArgument, "account type must be one of checking, savings, credit")
	}

	return e.current.Reserve(ctx, req)
}

// TestReserveContract_LegacyTracerRejectsLedgerAccountType documents the deploy
// order: the ledger sends its own account type verbatim, which a tracer still
// enforcing the legacy {checking, savings, credit} enum refuses with
// InvalidArgument. The ledger classifies that refusal as ErrTracerRejected —
// the tracer answered — so it never falls back to failPosture. Upgrading the
// tracer first avoids the refusal window.
func TestReserveContract_LegacyTracerRejectsLedgerAccountType(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	current := &tracerReserveEndpoint{now: now}
	client := newContractClient(t, &legacyTracerEndpoint{current: current})

	req := ledgerStyleReserveRequest(
		uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd"),
		uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"),
		now,
	)
	req.Account.Type = "deposit"

	result, err := client.Reserve(context.Background(), req)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrTracerRejected)
	assert.NotErrorIs(t, err, ErrTracerUnavailable)
	assert.False(t, current.parsed, "the legacy tracer must refuse before reserving")

	// A legacy account type still passes the legacy tracer.
	req.Account.Type = "checking"

	_, err = client.Reserve(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, current.parsed)
}
