// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHashExpression_Deterministic tests that hash is deterministic.
func TestHashExpression_Deterministic(t *testing.T) {
	t.Parallel()

	expression := "amount > 100 && transactionType == \"CARD\""

	hash1 := HashExpression(expression)
	hash2 := HashExpression(expression)
	hash3 := HashExpression(expression)

	assert.Equal(t, hash1, hash2, "Hash should be deterministic")
	assert.Equal(t, hash2, hash3, "Hash should be deterministic")
	assert.Len(t, hash1, 64, "SHA-256 hash should be 64 hex characters")
}

// TestHashExpression_Different tests that different expressions produce different hashes.
func TestHashExpression_Different(t *testing.T) {
	t.Parallel()

	hash1 := HashExpression("amount > 100")
	hash2 := HashExpression("amount > 10001")

	assert.NotEqual(t, hash1, hash2, "Different expressions should produce different hashes")
}
