// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"errors"
	"math"
	"unicode/utf8"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// SafeIntToInt32 Function to safely convert int to int32 with overflow check
func SafeIntToInt32(val int) (int32, error) {
	if val > math.MaxInt32 || val < math.MinInt32 {
		return 0, errors.New("integer overflow: value out of range for int32")
	}

	return int32(val), nil
}

// MaxAssetCodeLength is the longest asset code the Midaz ledger accepts, in runes.
const MaxAssetCodeLength = 100

// IsValidAssetCode reports whether code is an asset code the Midaz ledger
// accepts: non-empty, at most MaxAssetCodeLength runes, every rune an uppercase
// letter (utils.ValidateCode, the ledger's own rule).
func IsValidAssetCode(code string) bool {
	if code == "" || utf8.RuneCountInString(code) > MaxAssetCodeLength {
		return false
	}

	return utils.ValidateCode(code) == nil
}
