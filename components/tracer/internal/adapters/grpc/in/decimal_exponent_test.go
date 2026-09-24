// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"strconv"
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextAmountRefusesOutOfBoundDecimalExponent(t *testing.T) {
	t.Parallel()

	amount := "1e" + strconv.Itoa(safe.MaxDecimalExponent+1)
	_, err := tracercontract.Amount(amount).Decimal(context.Background(), tracercontract.Limits{MaxIntegerDigits: 128, MaxFractionDigits: 128})
	require.Error(t, err)
}
