// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"errors"
	"math"

	"golang.org/x/text/currency"
)

// SafeIntToInt32 Function to safely convert int to int32 with overflow check
func SafeIntToInt32(val int) (int32, error) {
	if val > math.MaxInt32 || val < math.MinInt32 {
		return 0, errors.New("integer overflow: value out of range for int32")
	}

	return int32(val), nil
}

// IsValidCurrency checks if currency is a valid ISO 4217 code.
// Uses golang.org/x/text/currency for proper ISO 4217 lookup validation.
// Requires uppercase as per ISO 4217 canonical format.
func IsValidCurrency(code string) bool {
	// ISO 4217 requires uppercase letters
	for _, c := range code {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	// Validate against actual ISO 4217 currency list
	_, err := currency.ParseISO(code)

	return err == nil
}
