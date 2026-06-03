// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package testhelper

import (
	"fmt"

	"tracer/pkg/model"
)

// MustNewTimeOfDay creates a model.TimeOfDay from a string or panics.
// Use only in tests where invalid input indicates a bug.
func MustNewTimeOfDay(s string) model.TimeOfDay {
	tod, err := model.NewTimeOfDay(s)
	if err != nil {
		panic(fmt.Sprintf("MustNewTimeOfDay(%q): %v", s, err))
	}

	return tod
}
