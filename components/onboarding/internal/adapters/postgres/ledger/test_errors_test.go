// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger

import "errors"

// Package-level sentinel error used as test fixture to satisfy err113 linter.
// Injected into go-sqlmock to simulate database failures.
var errTestBoom = errors.New("boom")
