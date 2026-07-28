// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	nethttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// validV2Input returns a fully populated, valid CreateTransactionV2Input.
func validV2Input() mtransaction.CreateTransactionV2Input {
	routeID := "00000000-0000-0000-0000-000000000000"
	operationRouteID := "11111111-1111-1111-1111-111111111111"

	return mtransaction.CreateTransactionV2Input{
		Description:      "New Transaction",
		Code:             "TR12345",
		Asset:            "BRL",
		Amount:           "1000",
		From:             "@person1",
		To:               "@person2",
		RouteID:          &routeID,
		OperationRouteID: &operationRouteID,
		Metadata:         map[string]any{"reference": "TRANSACTION-001"},
	}
}

func TestCreateTransactionV2Input_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(in *mtransaction.CreateTransactionV2Input)
		wantErr bool
	}{
		{
			name:    "fully populated valid input passes",
			mutate:  func(_ *mtransaction.CreateTransactionV2Input) {},
			wantErr: false,
		},
		{
			name:    "missing asset fails",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.Asset = "" },
			wantErr: true,
		},
		{
			name:    "missing amount fails",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.Amount = "" },
			wantErr: true,
		},
		{
			name:    "missing from fails",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.From = "" },
			wantErr: true,
		},
		{
			name:    "missing to fails",
			mutate:  func(in *mtransaction.CreateTransactionV2Input) { in.To = "" },
			wantErr: true,
		},
		{
			name: "metadata key over 100 chars fails (keymax)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				in.Metadata = map[string]any{strings.Repeat("k", 101): "value"}
			},
			wantErr: true,
		},
		{
			name: "metadata value over 2000 chars fails (valuemax)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				in.Metadata = map[string]any{"key": strings.Repeat("v", 2001)}
			},
			wantErr: true,
		},
		{
			name: "nested metadata value fails (nonested)",
			mutate: func(in *mtransaction.CreateTransactionV2Input) {
				in.Metadata = map[string]any{"key": map[string]any{"nested": "value"}}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := validV2Input()
			tt.mutate(&in)

			err := nethttp.ValidateStruct(in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestCreateTransactionV2Input_Translate_Stub locks the stub contract for Task
// 1.2.1: Translate compiles and returns a not-implemented error rather than a
// silently valid transaction. The mapping logic itself is Task 1.2.2.
func TestCreateTransactionV2Input_Translate_Stub(t *testing.T) {
	t.Parallel()

	for _, pending := range []bool{false, true} {
		got, err := validV2Input().Translate(pending)

		require.Error(t, err)
		assert.True(t, got.IsEmpty(), "stub must not produce a populated transaction")
	}
}
