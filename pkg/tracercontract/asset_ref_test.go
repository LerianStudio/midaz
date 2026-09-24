// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import (
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/stretchr/testify/require"
)

func TestAssetRefValidate(t *testing.T) {
	for _, tc := range []struct {
		name      string
		asset     AssetRef
		namespace string
		max       int
		valid     bool
	}{
		{"opaque unicode", AssetRef{"ledger", "ativo-ç", "TOKEN"}, "ledger", 10, true},
		{"empty namespace", AssetRef{"", "id", "BRL"}, "", 10, false},
		{"foreign namespace", AssetRef{"other", "id", "BRL"}, "ledger", 10, false},
		{"unbounded", AssetRef{"ledger", "id", "BRL"}, "ledger", 0, false},
		{"long namespace", AssetRef{"ledger", "id", "BRL"}, "ledger", 5, false},
		{"long id", AssetRef{"ledger", "too-long-id", "BRL"}, "ledger", 10, false},
		{"long code", AssetRef{"ledger", "id", "CODE-TOO-LONG"}, "ledger", 10, false},
		{"missing id", AssetRef{"ledger", "", "BRL"}, "ledger", 10, false},
		{"missing code", AssetRef{"ledger", "id", ""}, "ledger", 10, false},
		{"whitespace", AssetRef{"ledger", " id", "BRL"}, "ledger", 10, false},
		{"invalid utf8", AssetRef{"ledger", string([]byte{0xff}), "BRL"}, "ledger", 10, false},
		{"nul", AssetRef{"ledger", "id", "B\x00L"}, "ledger", 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.asset.Validate(tc.namespace, tc.max)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
			}
		})
	}
}
