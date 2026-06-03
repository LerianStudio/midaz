// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache

import "tracer/pkg/model"

// CachedRule holds a domain Rule and its compiled CEL program.
// Program is typed as `any` to avoid cross-package type coupling with
// the cel adapter package (avoids importing cel.Program into cache).
type CachedRule struct {
	Rule    *model.Rule
	Program any
}
