// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeValidateBody_CreatePackageSelectorReservedKey pins that a selector
// cannot name a metadata key the ledger reserves for its own writes.
func TestDecodeValidateBody_CreatePackageSelectorReservedKey(t *testing.T) {
	t.Parallel()

	body := `{
		"feeGroupLabel": "Standard",
		"minimumAmount": "100.00",
		"maximumAmount": "1000.00",
		"enable": true,
		"metadataSelector": {"packageAppliedID": "x"},
		"fees": {
			"test": {
				"feeLabel": "TestFee",
				"referenceAmount": "originalAmount",
				"priority": 1,
				"isDeductibleFrom": false,
				"creditAccount": "@fee_account",
				"calculationModel": {
					"applicationRule": "flatFee",
					"calculations": [{"type": "flat", "value": "10"}]
				}
			}
		}
	}`

	_, err := DecodeValidateBody([]byte(body), new(model.CreatePackageInput))

	var validationErr pkg.ValidationError

	require.True(t, errors.As(err, &validationErr), "got %v", err)
	assert.Equal(t, constant.ErrReservedMetadataKey.Error(), validationErr.Code)
}
