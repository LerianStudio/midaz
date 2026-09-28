// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"strconv"
	"testing"

	"github.com/LerianStudio/lib-commons/v7/commons/safe"

	"github.com/stretchr/testify/require"
)

func TestBalanceSettingsRefusesOutOfBoundOverdraftLimit(t *testing.T) {
	t.Parallel()

	limit := "1e" + strconv.Itoa(safe.MaxDecimalExponent+1)
	s := &BalanceSettings{OverdraftLimitEnabled: true, OverdraftLimit: &limit}

	s.Normalize()
	require.Equal(t, limit, *s.OverdraftLimit, "an out-of-bound limit is left for Validate to refuse")
	require.ErrorIs(t, s.Validate(), errInvalidOverdraftLimit)
}
