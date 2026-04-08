// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/stretchr/testify/assert"
)

func TestTransactionHandler_ApplyDefaultBalanceKeys(t *testing.T) {
	t.Parallel()

	handler := &TransactionHandler{}

	entries := []pkgTransaction.FromTo{
		{AccountAlias: "@origin", BalanceKey: ""},
		{AccountAlias: "@destination", BalanceKey: "custom-key"},
	}

	handler.ApplyDefaultBalanceKeys(entries)

	assert.Equal(t, constant.DefaultBalanceKey, entries[0].BalanceKey)
	assert.Equal(t, "custom-key", entries[1].BalanceKey)
}
