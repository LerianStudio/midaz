// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import _ "embed"

// FeesManifest is the embedded authorization surface of the "plugin-fees"
// service slug (the embedded fees/billing component that ships inside the
// ledger deployable under its own M2M identity). It is the byte content of
// permissions.yaml, published to the access-manager via the lib-auth
// Responsibility-Inversion declaration publisher at boot.
//
//go:embed permissions.yaml
var FeesManifest []byte
