// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestFormatScopeString_SchemeScope_KeepsTransactionTypeToken(t *testing.T) {
	t.Parallel()

	boleto := model.TransactionType("BOLETO")
	schemeValue := "BOLETO"

	got := formatScopeString([]model.Scope{{TransactionType: &boleto, Scheme: &schemeValue}})

	assert.Equal(t, "(transactionType:BOLETO)", got)
}
