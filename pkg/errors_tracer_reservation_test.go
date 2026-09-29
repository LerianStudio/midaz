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
	} {
		assert.False(t, codes[sentinel.Error()], "code %s is reused", sentinel.Error())
		codes[sentinel.Error()] = true
	}
}
