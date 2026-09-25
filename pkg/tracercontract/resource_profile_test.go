// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestResourceProfileRequiresLedgerFractionFloor(t *testing.T) {
	profile := tracercontract.DefaultResourceProfile()
	profile.Facts.MaxFractionDigits = tracercontract.MinimumResourceProfileFractionDigits - 1
	require.Error(t, profile.Validate())

	profile.Facts.MaxFractionDigits = tracercontract.MinimumResourceProfileFractionDigits
	require.NoError(t, profile.Validate())
}
