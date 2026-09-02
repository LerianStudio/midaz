// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// TestTranslate_OperationalTypeCodePropagation proves the v2 Translate literal copies
// operationalTypeCode onto the canonical carrier; a forgotten copy in the literal
// silently drops the field on the entire v2 surface (HIGH RISK).
func TestTranslate_OperationalTypeCodePropagation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
	}{
		{name: "absent stays empty", code: ""},
		{name: "present is copied", code: "PIX_IN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := mtransaction.CreateTransactionV2Input{
				Asset:               "BRL",
				Amount:              "1000",
				OperationalTypeCode: tt.code,
				Debits: []mtransaction.V2LegInput{
					{
						Alias:          "@person1",
						OrganizationID: "00000000-0000-0000-0000-000000000000",
						LedgerID:       "00000000-0000-0000-0000-000000000000",
						Amount:         "1000",
					},
				},
				Credits: []mtransaction.V2LegInput{
					{
						Alias:          "@person2",
						OrganizationID: "00000000-0000-0000-0000-000000000000",
						LedgerID:       "00000000-0000-0000-0000-000000000000",
						Amount:         "1000",
					},
				},
			}

			got, _, err := in.Translate(false)

			require.NoError(t, err)
			assert.Equal(t, tt.code, got.OperationalTypeCode,
				"Translate must copy OperationalTypeCode onto the canonical carrier")
		})
	}
}
