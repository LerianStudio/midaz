// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"errors"
	"math"
)

// SafeIntToInt32 Function to safely convert int to int32 with overflow check
func SafeIntToInt32(val int) (int32, error) {
	if val > math.MaxInt32 || val < math.MinInt32 {
		return 0, errors.New("integer overflow: value out of range for int32")
	}

	return int32(val), nil
}
