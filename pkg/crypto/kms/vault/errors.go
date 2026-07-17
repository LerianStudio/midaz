// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package vault

import "errors"

// ErrMountNotFound indicates that a tenant's Vault Transit mount is absent.
// Callers can match it with errors.Is to surface a typed error instead of
// silently falling back when the Transit engine has no handler for the route.
var ErrMountNotFound = errors.New("vault transit mount not found")
