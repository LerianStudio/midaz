// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestValidateBusinessError_TracerReservationOutcomesAreUnprocessable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		sentinel error
		code     string
	}{
		{name: "review", sentinel: constant.ErrTransactionReservationReview, code: "0531"},
		{name: "rejected", sentinel: constant.ErrTransactionReservationRejected, code: "0532"},
		{name: "rule denied", sentinel: constant.ErrTransactionReservationRuleDenied, code: "0535"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := pkg.ValidateBusinessError(tc.sentinel, constant.EntityTransaction)
			require.Error(t, err)

			mapped, ok := err.(pkg.UnprocessableOperationError)
			require.True(t, ok, "%s must be an HTTP 422 error, got %T", tc.code, err)
			assert.Equal(t, tc.code, mapped.Code)
			assert.Equal(t, constant.EntityTransaction, mapped.EntityType)
			assert.NotEmpty(t, mapped.Title)
			assert.NotEmpty(t, mapped.Message)
		})
	}
}

func TestTracerReservationSentinelsAreDistinct(t *testing.T) {
	t.Parallel()

	codes := map[string]bool{}
	for _, sentinel := range []error{
		constant.ErrTransactionReservationDenied,
		constant.ErrTransactionReservationUnavailable,
		constant.ErrTransactionReservationReview,
		constant.ErrTransactionReservationRejected,
		constant.ErrTransactionReservationRuleDenied,
		constant.ErrTransactionReservationUnauthorized,
		constant.ErrReservationAlreadySettled,
		constant.ErrReservationTenantInactive,
	} {
		assert.False(t, codes[sentinel.Error()], "code %s is reused", sentinel.Error())
		codes[sentinel.Error()] = true
	}
}

func TestValidateBusinessError_TracerReservationRuleDeniedContract(t *testing.T) {
	t.Parallel()

	err := pkg.ValidateBusinessError(constant.ErrTransactionReservationRuleDenied, constant.EntityTransaction)

	mapped, ok := err.(pkg.UnprocessableOperationError)
	require.True(t, ok, "0535 must be an HTTP 422 error, got %T", err)
	assert.Equal(t, "0535", mapped.Code)
	assert.Equal(t, "Transaction Reservation Rule Denied Error", mapped.Title)
	assert.Equal(t, "The transaction was denied by a transaction validation rule and this ledger enforces tracer decisions. Review the tracer rules or the ledger tracer settings.", mapped.Message)
}

func TestValidateBusinessError_TracerReservationUnauthorizedContract(t *testing.T) {
	t.Parallel()

	err := pkg.ValidateBusinessError(constant.ErrTransactionReservationUnauthorized, constant.EntityTransaction)

	mapped, ok := err.(pkg.ServiceUnavailableError)
	require.True(t, ok, "0536 must be an HTTP 503 error, got %T", err)
	assert.Equal(t, "0536", mapped.Code)
	assert.Equal(t, constant.EntityTransaction, mapped.EntityType)
	assert.Equal(t, "Transaction Reservation Unauthorized Error", mapped.Title)
	assert.Equal(t, "The tracer reservation seam rejected this ledger's credential and the ledger enforces tracer decisions with a closed fail posture. Check the ledger's Access Manager client and the tracer's allowed clients.", mapped.Message)
}
