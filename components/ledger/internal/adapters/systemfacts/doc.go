// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package systemfacts is the ledger's outbound adapter to the System Facts
// kit: it derives the midaz.transaction_committed fact from a completion write
// set and records it inside the completion's SQL transaction. A write set that
// is not an APPROVED direct, commit or revert derives no fact.
package systemfacts
