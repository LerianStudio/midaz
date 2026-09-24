// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestValidateAccountAssets(t *testing.T) {
	bounds := Limits{MaxAccounts: 2, MaxEntries: 2, MaxTextBytes: 30, MaxIntegerDigits: 20, MaxFractionDigits: 10}
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	asset := AssetRef{Namespace: "producer", ID: "opaque", Code: "TOKEN"}
	for _, tc := range []struct {
		name  string
		facts []AccountAsset
		valid bool
	}{
		{"complete", []AccountAsset{{AccountID: id, Asset: asset}}, true},
		{"empty", nil, false},
		{"zero account", []AccountAsset{{Asset: asset}}, false},
		{"missing asset", []AccountAsset{{AccountID: id}}, false},
		{"duplicate", []AccountAsset{{AccountID: id, Asset: asset}, {AccountID: id, Asset: asset}}, false},
		{"wrong namespace", []AccountAsset{{AccountID: id, Asset: AssetRef{Namespace: "other", ID: "opaque", Code: "TOKEN"}}}, false},
		{"contradiction", []AccountAsset{{AccountID: id, Asset: asset}, {AccountID: uuid.MustParse("22222222-2222-2222-2222-222222222222"), Asset: AssetRef{Namespace: "producer", ID: "opaque", Code: "OTHER"}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAccountAssets(t.Context(), tc.facts, "producer", bounds)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, ValidateAccountAssets(ctx, []AccountAsset{{AccountID: id, Asset: asset}}, "producer", bounds), context.Canceled)
}
