// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

// ManifestRoutePath is the versioned HTTP path at which a producing service
// serves its lib-streaming manifest (catalog-only). It is shared by the ledger
// and tracer binaries so the streaming hub discovers every producer's manifest
// at one stable, predictable path.
const ManifestRoutePath = "/v1/streaming/manifest"
