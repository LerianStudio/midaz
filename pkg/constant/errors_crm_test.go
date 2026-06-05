// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCRMErrorSentinelWireCodes locks the wire code string of every surviving
// CRM-00xx sentinel after the standalone CRM error-code transformer was
// dropped. These codes are part of the external API contract: a change to any
// string here is a breaking change for clients that branch on the code, so the
// lock makes such a change fail loudly in review rather than ship silently.
//
// The gaps in the sequence (CRM-0001..0005, 0007, 0009, 0011, 0012, 0014..0016,
// 0018) are intentional — those were the 1:1-mapping transform codes with
// canonical midaz equivalents (and the orphan CRM-0018) and no longer exist as
// sentinels.
func TestCRMErrorSentinelWireCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"ErrHolderNotFound", ErrHolderNotFound, "CRM-0006"},
		{"ErrInstrumentNotFound", ErrInstrumentNotFound, "CRM-0008"},
		{"ErrDocumentAssociationError", ErrDocumentAssociationError, "CRM-0010"},
		{"ErrAccountAlreadyAssociated", ErrAccountAlreadyAssociated, "CRM-0013"},
		{"ErrHolderHasInstruments", ErrHolderHasInstruments, "CRM-0017"},
		{"ErrMetadataQueryInvalidFormat", ErrMetadataQueryInvalidFormat, "CRM-0019"},
		{"ErrMetadataQueryInvalidKey", ErrMetadataQueryInvalidKey, "CRM-0020"},
		{"ErrMetadataQueryContainsOperator", ErrMetadataQueryContainsOperator, "CRM-0021"},
		{"ErrInvalidHeaderValue", ErrInvalidHeaderValue, "CRM-0022"},
		{"ErrInstrumentClosingDateBeforeCreation", ErrInstrumentClosingDateBeforeCreation, "CRM-0023"},
		{"ErrRelatedPartyNotFound", ErrRelatedPartyNotFound, "CRM-0024"},
		{"ErrInvalidRelatedPartyRole", ErrInvalidRelatedPartyRole, "CRM-0025"},
		{"ErrRelatedPartyDocumentRequired", ErrRelatedPartyDocumentRequired, "CRM-0026"},
		{"ErrRelatedPartyNameRequired", ErrRelatedPartyNameRequired, "CRM-0027"},
		{"ErrRelatedPartyStartDateRequired", ErrRelatedPartyStartDateRequired, "CRM-0028"},
		{"ErrRelatedPartyEndDateInvalid", ErrRelatedPartyEndDateInvalid, "CRM-0029"},
	}

	seen := make(map[string]string, len(cases))

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.code, tc.err.Error(),
				"CRM sentinel %s must resolve to its locked wire code", tc.name)
		})

		if prev, dup := seen[tc.code]; dup {
			t.Errorf("wire code %q is shared by %s and %s: CRM sentinels must be unique", tc.code, prev, tc.name)
		}

		seen[tc.code] = tc.name
	}
}
