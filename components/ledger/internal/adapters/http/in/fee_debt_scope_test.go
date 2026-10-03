// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	services "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
)

// TestListFeeDebts_ConfinedToThePartnerScope drives the listing with a partner the
// authorization service allows no account: it lists nothing, and never reaches the
// record, which this handler does not even have.
func TestListFeeDebts_ConfinedToThePartnerScope(t *testing.T) {
	handler := &FeeDebtHandler{Service: &services.FeeDebtService{}}
	ctx := partnerScopedContext(t, `,"allowed":{"accountId":[]}`, scopeDimensionAccount)

	resp, err := handler.ListFeeDebtsV2(ctx, &ListFeeDebtsV2Request{FeeV2Path: FeeV2Path{OrganizationID: uuid.NewString(), LedgerID: uuid.NewString()}})
	require.NoError(t, err)
	assert.Empty(t, resp.Body.Items)
}
