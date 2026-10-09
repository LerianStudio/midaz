// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
)

// TestValidateOperationDirection_IdentifiesAnInvalidDirection covers the error
// identity the legacy replay relies on to tell a record that can never be
// written from a transient failure.
func TestValidateOperationDirection_IdentifiesAnInvalidDirection(t *testing.T) {
	t.Parallel()

	logger := &libLog.NopLogger{}

	for _, direction := range []string{"", "debit", "CREDIT"} {
		assert.NoError(t, validateOperationDirection(context.Background(), logger, &operation.Operation{ID: "op", Direction: direction}), direction)
	}

	err := validateOperationDirection(context.Background(), logger, &operation.Operation{ID: "op", Direction: "sideways"})
	require.ErrorIs(t, err, ErrInvalidOperationDirection)
	assert.Contains(t, err.Error(), "sideways")
}
