// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestValidateBusinessError_CrossLedgerNotEnabled(t *testing.T) {
	t.Parallel()

	err := pkg.ValidateBusinessError(constant.ErrCrossLedgerNotEnabled, constant.EntityLedger, "ledger-id")
	require.Error(t, err)

	mapped, ok := err.(pkg.UnprocessableOperationError)
	require.True(t, ok, "cross-ledger policy refusal must be an HTTP 422 error, got %T", err)
	assert.Equal(t, "0200", mapped.Code)
	assert.Equal(t, constant.EntityLedger, mapped.EntityType)
	assert.Contains(t, mapped.Message, "ledger-id")
}
