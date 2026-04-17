// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import "errors"

// Package-level sentinel errors used as test fixtures to satisfy err113 linter.
// These simulate mock repository failures for round-trip assertions.
var (
	errTestCreateFailed = errors.New("create failed")
	errTestDeleteFailed = errors.New("delete failed")
)
