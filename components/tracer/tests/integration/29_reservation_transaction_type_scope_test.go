// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// typeScopeReserveTimeout bounds one Reserve call so a stalled seam fails the
// test instead of hanging it.
const typeScopeReserveTimeout = 10 * time.Second

// typeScopeLimitCap is the DAILY cap shared by the account-only and the
// PIX-scoped limit, so the same spend reaches both at once.
const typeScopeLimitCap = "1000"

// typeScopeRequestSeedOffset separates a reserve's request-id seed from its
// transaction-id seed, so both ids stay deterministic and distinct.
const typeScopeRequestSeedOffset = 100

// typeScopeTransactionID is the deterministic transaction id of the reserve
// seeded by seed.
func typeScopeTransactionID(seed int64) uuid.UUID {
	return testutil.MustDeterministicUUID(seed)
}

// reserveTyped reserves for accountID over the gRPC seam with a ledger-shaped
// request carrying transactionType; an empty transactionType omits the field.
// It returns the transport error so a caller can assert the gRPC status.
// seed derives the transaction and request ids, so each call site passes its
// own seed to keep reservations apart.
//
// The transaction's reservations are released when the test ends: the
// integration database is shared, and a RESERVED row left behind would be
// swept by any reaper another test drives past the reservation TTL.
func reserveTyped(t *testing.T, seed int64, accountID uuid.UUID, amount, transactionType string) (*reservationv1.ReserveResult, error) {
	t.Helper()

	transactionID := typeScopeTransactionID(seed)

	client := testutil.DialReservationClient(t)
	t.Cleanup(func() { releaseTypedReservations(t, client, transactionID) })

	ctx, cancel := context.WithTimeout(context.Background(), typeScopeReserveTimeout)
	defer cancel()

	return client.Reserve(ctx, &reservationv1.ReserveRequest{
		TransactionId:        transactionID.String(),
		RequestId:            testutil.MustDeterministicUUID(seed + typeScopeRequestSeedOffset).String(),
		Amount:               amount,
		Asset:                "BRL",
		TransactionType:      transactionType,
		TransactionTimestamp: testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		Account: &reservationv1.ReserveAccount{
			AccountId: accountID.String(),
			Type:      "deposit",
		},
		Metadata: map[string]string{"channel": "app"},
	})
}

// releaseTypedReservations releases every reservation transactionID holds.
// The release is idempotent, so a transaction the seam refused or denied is a
// no-op.
func releaseTypedReservations(t *testing.T, client reservationv1.ReservationServiceClient, transactionID uuid.UUID) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), typeScopeReserveTimeout)
	defer cancel()

	_, err := client.ReleaseByTransaction(ctx, &reservationv1.ReleaseByTransactionRequest{
		TransactionId: transactionID.String(),
	})
	assert.NoError(t, err, "ReleaseByTransaction must succeed over the gRPC seam")
}

// mustReserveTyped is reserveTyped for a request the seam must accept.
func mustReserveTyped(t *testing.T, seed int64, accountID uuid.UUID, amount, transactionType string) *reservationv1.ReserveResult {
	t.Helper()

	got, err := reserveTyped(t, seed, accountID, amount, transactionType)
	require.NoError(t, err, "Reserve must succeed over the gRPC seam")
	require.NotNil(t, got)

	return got
}

// createActiveTypeScopedLimits creates and activates, on accountID, the two
// DAILY BRL limits the scenarios share: one scoped to the account alone and one
// scoped to the account plus the PIX transaction type.
func createActiveTypeScopedLimits(t *testing.T, accountID uuid.UUID) (accountLimitID, pixLimitID string) {
	t.Helper()

	accountLimitID = createActiveAccountLimit(t, accountID, typeScopeLimitCap)

	scopedAccount := accountID.String()
	pixType := string(model.TransactionTypePix)
	pixLimitID = testutil.CreateLimitWithScope(t, "Reserve type scope PIX "+testutil.RandomSuffix(), typeScopeLimitCap,
		[]testutil.ScopeInput{{AccountID: &scopedAccount, TransactionType: &pixType}})
	testutil.ActivateLimit(t, pixLimitID)

	t.Cleanup(func() { testutil.CleanupLimit(t, pixLimitID) })

	return accountLimitID, pixLimitID
}

// assertReserveAllowed asserts an admitted reserve holding one reservation per
// matched limit.
func assertReserveAllowed(t *testing.T, got *reservationv1.ReserveResult, wantReservations int) {
	t.Helper()

	assert.False(t, got.GetDenied())
	assert.Equal(t, "ALLOW", got.GetDecision())
	assert.Len(t, got.GetReservationIds(), wantReservations)
}

// assertReserveLimitExceeded asserts a reserve refused by a limit, holding no
// capacity.
func assertReserveLimitExceeded(t *testing.T, got *reservationv1.ReserveResult) {
	t.Helper()

	assert.True(t, got.GetDenied())
	assert.Equal(t, "DENY", got.GetDecision())
	assert.Equal(t, "limit_exceeded", got.GetReason())
	assert.Empty(t, got.GetReservationIds())
}

// TestIntegration_Reservation_TransactionTypeScope_TypedReserveCountsBothLimits
// proves a reserve carrying transactionType PIX resolves the account-only limit
// AND the PIX-scoped limit: each admitted reserve holds one reservation per
// limit, both counters move together, and the reserve past the shared cap is
// refused as limit_exceeded.
func TestIntegration_Reservation_TransactionTypeScope_TypedReserveCountsBothLimits(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	accountID := testutil.MustDeterministicUUID(96501)
	accountLimitID, pixLimitID := createActiveTypeScopedLimits(t, accountID)

	assertReserveAllowed(t, mustReserveTyped(t, 96510, accountID, "600.00", string(model.TransactionTypePix)), 2)
	assertReserveAllowed(t, mustReserveTyped(t, 96511, accountID, "400.00", string(model.TransactionTypePix)), 2)
	assertReserveLimitExceeded(t, mustReserveTyped(t, 96512, accountID, "1.00", string(model.TransactionTypePix)))

	wantUsage := decimal.RequireFromString(typeScopeLimitCap)
	assert.True(t, limitUsage(t, db, accountLimitID).Equal(wantUsage),
		"the account-only limit must hold the full typed spend")
	assert.True(t, limitUsage(t, db, pixLimitID).Equal(wantUsage),
		"the PIX-scoped limit must hold the full typed spend")
}

// TestIntegration_Reservation_TransactionTypeScope_TypelessReserveSkipsTypedLimit
// proves a reserve without transactionType resolves only the account-only
// limit: it holds exactly one reservation, is refused by that limit past its
// cap, and never counts against the PIX-scoped limit.
func TestIntegration_Reservation_TransactionTypeScope_TypelessReserveSkipsTypedLimit(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	accountID := testutil.MustDeterministicUUID(96502)
	accountLimitID, pixLimitID := createActiveTypeScopedLimits(t, accountID)

	assertReserveAllowed(t, mustReserveTyped(t, 96513, accountID, "600.00", ""), 1)
	assertReserveAllowed(t, mustReserveTyped(t, 96514, accountID, "400.00", ""), 1)
	assertReserveLimitExceeded(t, mustReserveTyped(t, 96515, accountID, "1.00", ""))

	assert.True(t, limitUsage(t, db, accountLimitID).Equal(decimal.RequireFromString(typeScopeLimitCap)),
		"the account-only limit must hold the full typeless spend")
	assert.True(t, limitUsage(t, db, pixLimitID).IsZero(),
		"a typeless reserve must never count against the PIX-scoped limit")
}

// TestIntegration_Reservation_TransactionTypeScope_UnknownTypeRejected proves
// the seam refuses a transactionType outside the enum as InvalidArgument
// before any limit is touched.
func TestIntegration_Reservation_TransactionTypeScope_UnknownTypeRejected(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	accountID := testutil.MustDeterministicUUID(96503)
	createActiveTypeScopedLimits(t, accountID)

	const seed = 96516

	got, err := reserveTyped(t, seed, accountID, "1.00", "TED")

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Nil(t, got)
	assert.Zero(t, reservationRowCount(t, db, typeScopeTransactionID(seed)), "a rejected reserve must not write a reservation row")
}
