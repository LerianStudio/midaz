// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strconv"
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestFeeAmountsRefuseOutOfBoundDecimals(t *testing.T) {
	t.Parallel()

	raw := "1e" + strconv.Itoa(safe.MaxDecimalExponent+1)
	deductible := false

	for _, err := range []error{
		(&CreatePackageInput{MinAmount: raw, MaxAmount: "10"}).ValidateMinAndMaxAmount(),
		(&CreatePackageInput{MinAmount: "1", MaxAmount: raw}).ValidateMinAndMaxAmount(),
		(&UpdatePackageInput{MinAmount: &raw}).ValidateMinAndMaxAmount(),
		(&Fee{IsDeductibleFrom: &deductible}).validateCalculation(Calculation{Type: Flat, Value: raw}, "fee", decimal.NewFromInt(1000)),
	} {
		var vErr pkg.ValidationError
		require.ErrorAs(t, err, &vErr)
		require.Equal(t, constant.ErrConvertToDecimal.Error(), vErr.Code)
	}
}
