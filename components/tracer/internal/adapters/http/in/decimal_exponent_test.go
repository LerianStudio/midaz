// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"strconv"
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestHandlersRefuseOutOfBoundDecimals(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	value := "1e" + strconv.Itoa(safe.MaxDecimalExponent+1)

	cases := []struct {
		name  string
		field string
		call  func() error
	}{
		{"create limit", "maxAmount", func() error {
			_, err := (&LimitHandler{}).createLimit(ctx, []byte(`{"maxAmount":"`+value+`"}`))
			return err
		}},
		{"update limit", "maxAmount", func() error {
			_, err := (&LimitHandler{}).updateLimit(ctx, uuid.NewString(), []byte(`{"maxAmount":"`+value+`"}`))
			return err
		}},
		{"reserve", "amount", func() error {
			_, err := (&ReservationHandler{clock: clock.New()}).reserve(ctx, []byte(`{"amount":"`+value+`"}`))
			return err
		}},
		{"validate", "amount", func() error {
			_, err := (&ValidationHandler{clock: clock.New()}).validate(ctx, []byte(`{"amount":"`+value+`"}`))
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var vErr pkg.ValidationKnownFieldsError
			require.ErrorAs(t, tc.call(), &vErr)
			require.Equal(t, constant.ErrBadRequest.Error(), vErr.Code)
			require.Contains(t, vErr.Fields, tc.field)
		})
	}
}
