// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

// FeeAliasDetachResult reports what removing a deleted account's alias from a
// ledger's fee configuration changed. Both counts cover fee packages and billing
// packages together.
type FeeAliasDetachResult struct {
	// PackagesUpdated is how many packages lost at least one reference to the alias.
	PackagesUpdated int
	// PackagesDisabled is how many of those packages went from enabled to
	// disabled because the alias was a fee, leg or target they could not do without.
	PackagesDisabled int
}
