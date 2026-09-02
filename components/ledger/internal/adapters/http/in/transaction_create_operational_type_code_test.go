// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// TestResolveIdempotencyHashSource_OperationalTypeCode proves the idempotency hash source
// (the raw body serialization) participates in operationalTypeCode:
//   - two bodies that differ only in the code produce DIFFERENT sources => distinct hashes
//     => distinct transactions;
//   - the same body (code included) produces the SAME source => same hash => original
//     response replayed;
//   - an absent code produces a source byte-identical to a body that never had the field
//     (ADR-002).
func TestResolveIdempotencyHashSource_OperationalTypeCode(t *testing.T) {
	t.Parallel()

	base := mtransaction.Transaction{
		Description: "transfer",
		Code:        "TR12345",
		Send: mtransaction.Send{
			Asset: "BRL",
		},
	}

	withCode := base
	withCode.OperationalTypeCode = "PIX_IN"

	withOtherCode := base
	withOtherCode.OperationalTypeCode = "TED_OUT"

	absentSource, err := resolveIdempotencyHashSource(base)
	require.NoError(t, err)

	codeSource, err := resolveIdempotencyHashSource(withCode)
	require.NoError(t, err)

	otherCodeSource, err := resolveIdempotencyHashSource(withOtherCode)
	require.NoError(t, err)

	// Same body (code included) => same source => original response replayed.
	repeatSource, err := resolveIdempotencyHashSource(withCode)
	require.NoError(t, err)
	assert.Equal(t, codeSource, repeatSource,
		"an identical body must resolve to the same idempotency hash source")

	// Different code => different source => distinct transaction.
	assert.NotEqual(t, codeSource, absentSource,
		"a body carrying a code must differ from one that does not")
	assert.NotEqual(t, codeSource, otherCodeSource,
		"bodies with different codes must resolve to different hash sources")

	// ADR-002: absent code => body byte-identical to a pre-2.2.2 body (no key emitted).
	assert.NotContains(t, absentSource, "operationalTypeCode",
		"an absent operationalTypeCode must not appear in the idempotency hash source")
}
