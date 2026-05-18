// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateParameters_AcceptsAliasBankingDetailsBankIDAndType(t *testing.T) {
	t.Parallel()

	query, err := ValidateParameters(map[string]string{
		"banking_details_bank_id": "12345678",
		"banking_details_type":    "CACC",
	})

	require.NoError(t, err)
	require.NotNil(t, query.BankingDetailsBankID)
	require.NotNil(t, query.BankingDetailsType)
	assert.Equal(t, "12345678", *query.BankingDetailsBankID)
	assert.Equal(t, "CACC", *query.BankingDetailsType)
}
