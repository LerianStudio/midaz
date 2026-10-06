// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package reservationmap

import (
	"strconv"
	"testing"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

var (
	fixtureTime = time.Date(2026, time.March, 15, 12, 34, 56, 123456789, time.UTC)
	requestID   = uuid.MustParse("00000000-0000-4000-8000-000000000002")
	accountID   = uuid.MustParse("00000000-0000-4000-8000-000000000003")
	segmentID   = uuid.MustParse("00000000-0000-4000-8000-000000000004")
	portfolioID = uuid.MustParse("00000000-0000-4000-8000-000000000005")
	merchantID  = uuid.MustParse("00000000-0000-4000-8000-000000000006")
)

// ledgerShapedProto is a reserve request shaped as the ledger sends it:
// RFC3339Nano timestamp, free-form account type, flat metadata.
func ledgerShapedProto() *reservationv1.ReserveRequest {
	return &reservationv1.ReserveRequest{
		TransactionId:        "00000000-0000-4000-8000-000000000001",
		RequestId:            requestID.String(),
		Amount:               "100.25",
		Asset:                "BRL",
		Account:              &reservationv1.ReserveAccount{AccountId: accountID.String(), Type: "deposit"},
		TransactionType:      string(model.TransactionTypeCard),
		TransactionTimestamp: fixtureTime.Format(time.RFC3339Nano),
		Metadata:             map[string]string{"channel": "app"},
	}
}

func TestValidationRequestFromReserveProto_MapsEveryField(t *testing.T) {
	t.Parallel()

	req := ledgerShapedProto()
	req.SegmentId = segmentID.String()
	req.PortfolioId = portfolioID.String()
	req.MerchantId = merchantID.String()

	got, err := ValidationRequestFromReserveProto(req)
	require.NoError(t, err)

	require.Equal(t, requestID, got.RequestID)
	require.Equal(t, model.TransactionTypeCard, got.TransactionType)
	require.True(t, decimal.RequireFromString("100.25").Equal(got.Amount))
	require.Equal(t, "BRL", got.Asset)
	require.Equal(t, model.AccountContext{ID: accountID, Type: "deposit"}, got.Account)
	require.Equal(t, map[string]any{"channel": "app"}, got.Metadata)
	require.Equal(t, &model.SegmentContext{ID: segmentID}, got.Segment)
	require.Equal(t, &model.PortfolioContext{ID: portfolioID}, got.Portfolio)
	require.Equal(t, &model.MerchantContext{ID: merchantID}, got.Merchant)
}

func TestValidationRequestFromReserveProto_ParsesRFC3339NanoTimestamp(t *testing.T) {
	t.Parallel()

	got, err := ValidationRequestFromReserveProto(ledgerShapedProto())
	require.NoError(t, err)

	require.True(t, fixtureTime.Equal(got.TransactionTimestamp), "want %s, got %s", fixtureTime, got.TransactionTimestamp)
	require.Equal(t, 123456789, got.TransactionTimestamp.Nanosecond())
}

func TestValidationRequestFromReserveProto_AbsentOptionalFields(t *testing.T) {
	t.Parallel()

	t.Run("empty timestamp stays zero for the validator to reject", func(t *testing.T) {
		t.Parallel()

		req := ledgerShapedProto()
		req.TransactionTimestamp = ""

		got, err := ValidationRequestFromReserveProto(req)
		require.NoError(t, err)
		require.True(t, got.TransactionTimestamp.IsZero())
	})

	t.Run("empty metadata stays nil", func(t *testing.T) {
		t.Parallel()

		req := ledgerShapedProto()
		req.Metadata = map[string]string{}

		got, err := ValidationRequestFromReserveProto(req)
		require.NoError(t, err)
		require.Nil(t, got.Metadata)
	})

	t.Run("account type without an account id is carried", func(t *testing.T) {
		t.Parallel()

		req := ledgerShapedProto()
		req.Account = &reservationv1.ReserveAccount{Type: "deposit"}

		got, err := ValidationRequestFromReserveProto(req)
		require.NoError(t, err)
		require.Equal(t, uuid.Nil, got.Account.ID)
		require.Equal(t, "deposit", got.Account.Type)
	})

	t.Run("absent account is the nil account", func(t *testing.T) {
		t.Parallel()

		req := ledgerShapedProto()
		req.Account = nil

		got, err := ValidationRequestFromReserveProto(req)
		require.NoError(t, err)
		require.Equal(t, model.AccountContext{}, got.Account)
	})

	t.Run("empty scope ids are absent", func(t *testing.T) {
		t.Parallel()

		got, err := ValidationRequestFromReserveProto(ledgerShapedProto())
		require.NoError(t, err)
		require.Nil(t, got.Segment)
		require.Nil(t, got.Portfolio)
		require.Nil(t, got.Merchant)
	})
}

func TestValidationRequestFromReserveProto_RejectsWithSentinels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*reservationv1.ReserveRequest)
		want   error
	}{
		{
			name:   "malformed request id",
			mutate: func(r *reservationv1.ReserveRequest) { r.RequestId = "not-a-uuid" },
			want:   constant.ErrValidationRequestIDRequired,
		},
		{
			name:   "absent request id",
			mutate: func(r *reservationv1.ReserveRequest) { r.RequestId = "" },
			want:   constant.ErrValidationRequestIDRequired,
		},
		{
			name:   "malformed amount",
			mutate: func(r *reservationv1.ReserveRequest) { r.Amount = "ten" },
			want:   constant.ErrValidationAmountNonPositive,
		},
		{
			name:   "out-of-bound decimal exponent",
			mutate: func(r *reservationv1.ReserveRequest) { r.Amount = "1e" + strconv.Itoa(safe.MaxDecimalExponent+1) },
			want:   constant.ErrValidationAmountNonPositive,
		},
		{
			name:   "malformed timestamp",
			mutate: func(r *reservationv1.ReserveRequest) { r.TransactionTimestamp = "2026-03-15 12:34:56" },
			want:   constant.ErrValidationTimestampRequired,
		},
		{
			name:   "malformed account id",
			mutate: func(r *reservationv1.ReserveRequest) { r.Account.AccountId = "not-a-uuid" },
			want:   constant.ErrInvalidPathParameter,
		},
		{
			name:   "malformed segment id",
			mutate: func(r *reservationv1.ReserveRequest) { r.SegmentId = "not-a-uuid" },
			want:   constant.ErrInvalidPathParameter,
		},
		{
			name:   "malformed portfolio id",
			mutate: func(r *reservationv1.ReserveRequest) { r.PortfolioId = "not-a-uuid" },
			want:   constant.ErrInvalidPathParameter,
		},
		{
			name:   "malformed merchant id",
			mutate: func(r *reservationv1.ReserveRequest) { r.MerchantId = "not-a-uuid" },
			want:   constant.ErrInvalidPathParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := ledgerShapedProto()
			tt.mutate(req)

			got, err := ValidationRequestFromReserveProto(req)
			require.ErrorIs(t, err, tt.want)
			require.Nil(t, got)
		})
	}
}

func TestValidationRequestFromReserveProto_PassesReserveValidation(t *testing.T) {
	t.Parallel()

	got, err := ValidationRequestFromReserveProto(ledgerShapedProto())
	require.NoError(t, err)
	require.NoError(t, got.NormalizeAndValidateForReserve(fixtureTime.Add(time.Second)))
}
