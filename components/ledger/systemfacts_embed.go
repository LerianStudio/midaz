// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger

import "embed"

// SystemFactsContracts is the embedded System Facts contract directory of the
// midaz producer: contracts/system-facts holds the manifest, the fact schemas
// and their fixtures. The producer compiles it at boot and the unit suite runs
// the offline contract check over it. //go:embed is relative to this source
// file, so the anchor lives at the ledger component root beside the directory.
//
//go:embed contracts/system-facts
var SystemFactsContracts embed.FS
