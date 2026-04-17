// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import "errors"

// Package-level sentinel errors used as test fixtures to satisfy err113 linter.
// These are injected into go-sqlmock to simulate database failures.
var (
	errTestBoom          = errors.New("boom")
	errTestDBUnavailable = errors.New("db unavailable")
	errTestDBDown        = errors.New("db down")
)
