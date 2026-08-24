// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package ledger co-locates the embedded access-manager permissions manifest
// with the YAML it embeds: //go:embed is relative to this source file and
// rejects parent-directory references, so the anchor must live at the ledger
// component root alongside permissions.yaml.
package ledger

import _ "embed"

// MidazManifest is the embedded authorization surface of the "midaz" service
// slug (the core ledger plus embedded CRM). It is the byte content of
// permissions.yaml, published to the access-manager via the lib-auth
// Responsibility-Inversion declaration publisher at boot.
//
//go:embed permissions.yaml
var MidazManifest []byte
