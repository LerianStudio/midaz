// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"strconv"
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestDecodeValidateBodyRefusesOutOfBoundDecimalExponent(t *testing.T) {
	t.Parallel()

	type feeBody struct {
		FeeAmount *decimal.Decimal `json:"feeAmount"`
	}

	_, err := DecodeValidateBody([]byte(`{"feeAmount":"`+"1e"+strconv.Itoa(safe.MaxDecimalExponent+1)+`"}`), &feeBody{})

	var vErr pkg.ValidationKnownFieldsError
	require.ErrorAs(t, err, &vErr)
	require.Equal(t, constant.ErrBadRequest.Error(), vErr.Code)
	require.Contains(t, vErr.Fields, "feeAmount")

	var body feeBody

	_, err = DecodeValidateBody([]byte(`{"feeAmount":"50.00"}`), &body)
	require.NoError(t, err)
	require.True(t, decimal.RequireFromString("50.00").Equal(*body.FeeAmount))
}
