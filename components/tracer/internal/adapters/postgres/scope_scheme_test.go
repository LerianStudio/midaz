// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestFillScopeSchemes(t *testing.T) {
	t.Parallel()

	boleto := model.TransactionType("BOLETO")
	pix := model.TransactionTypePix
	accountID := testutil.MustDeterministicUUID(1)

	scopes := []model.Scope{
		{TransactionType: &boleto},
		{TransactionType: &pix, Scheme: testutil.Ptr("PIX")},
		{AccountID: &accountID},
	}

	fillScopeSchemes(scopes)

	assert.Equal(t, testutil.Ptr("BOLETO"), scopes[0].Scheme, "filled from transactionType")
	assert.Equal(t, testutil.Ptr("PIX"), scopes[1].Scheme, "an existing scheme is kept")
	assert.Nil(t, scopes[2].Scheme, "a scope without a scheme stays without one")
	assert.Nil(t, scopes[2].TransactionType)

	*scopes[0].Scheme = "CHANGED"
	assert.Equal(t, model.TransactionType("BOLETO"), *scopes[0].TransactionType, "the scheme does not alias transactionType")

	fillScopeSchemes(nil)
}
