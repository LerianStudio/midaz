// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strconv"
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

func TestToValidationRequestRefusesOutOfBoundDecimalExponent(t *testing.T) {
	t.Parallel()

	amount := "1e" + strconv.Itoa(safe.MaxDecimalExponent+1)
	_, err := (&ReservationServer{}).toValidationRequest(&reservationv1.ReserveRequest{RequestId: uuid.NewString(), Amount: amount})
	require.ErrorIs(t, err, constant.ErrValidationAmountNonPositive)
}
