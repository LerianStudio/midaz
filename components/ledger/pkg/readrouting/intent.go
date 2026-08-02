// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package readrouting transports read-routing intent through context.Context.
// It carries only the intent marker; the routing decision (flag + adapter seam)
// is made elsewhere.
package readrouting

import "context"

// primaryReadKey is an unexported context key type to avoid collisions with
// keys defined in other packages.
type primaryReadKey struct{}

// WithPrimaryRead returns a child context marked to route reads to the primary.
func WithPrimaryRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, primaryReadKey{}, struct{}{})
}

// IsPrimaryRead reports whether ctx carries the primary-read intent marker.
// It returns false for a nil or unmarked context.
func IsPrimaryRead(ctx context.Context) bool {
	if ctx == nil {
		return false
	}

	_, ok := ctx.Value(primaryReadKey{}).(struct{})

	return ok
}
