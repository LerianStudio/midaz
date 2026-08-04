// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package declaration holds the fees component's embedded D7 permissions
// manifest. The fees slice owns its own declaration (slug plugin-fees) even
// though it ships inside the unified ledger binary. The manifest bytes are
// embedded here because //go:embed is resolved relative to this source file,
// keeping the YAML co-located with the component that owns it.
package declaration

import _ "embed"

// Manifest is the embedded fees permissions manifest (authored as YAML). It is
// passed verbatim as declaration.Config.Manifest to the lib-auth publisher,
// which parses + validates it eagerly at construction time.
//
//go:embed permissions.yaml
var Manifest []byte
