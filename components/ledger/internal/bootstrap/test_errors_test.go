// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import "errors"

// Package-level sentinel errors used as test fixtures to satisfy err113 linter.
// These simulate downstream close() failures for service shutdown tests.
var (
	errTestOnboardingClose  = errors.New("onboarding close failed")
	errTestTransactionClose = errors.New("transaction close failed")
)
