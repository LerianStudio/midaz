// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestBuildRequestSnapshot_WritesSchemeAndTransactionType(t *testing.T) {
	t.Parallel()

	req := &model.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1),
		TransactionType:      model.TransactionType("BOLETO"),
		Scheme:               "BOLETO",
		Amount:               decimal.RequireFromString("10"),
		Asset:                "BRL",
		TransactionTimestamp: testutil.FixedTime(),
		Account:              model.AccountContext{ID: testutil.MustDeterministicUUID(2)},
	}

	snapshot := buildRequestSnapshot(req)

	assert.Equal(t, model.TransactionType("BOLETO"), snapshot["transactionType"])
	assert.Equal(t, "BOLETO", snapshot["scheme"])
}
