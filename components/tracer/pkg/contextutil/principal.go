// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package contextutil

import "context"

// Principal represents the authenticated identity that initiated the current
// request, captured by the auth middleware after the credentials have been
// validated upstream (lib-auth for JWT, constant-time compare for API key).
//
// Type is intentionally a plain string — not model.ActorType — to avoid an
// import cycle between contextutil and pkg/model. The audit writer is the
// boundary that converts Type back into model.ActorType.
//
// Recognized Type values match the actor_type_enum stored in the database:
// "user", "api_key", "system".
//
// Empty Name is allowed and is the common case for API-key principals where
// only a label (e.g. "tracer-default") is known.
type Principal struct {
	Type string
	ID   string
	Name string
}

// ContextKeyPrincipal is the type-safe key for storing the authenticated
// principal in context. Follows the same idiom as ContextKeyClientIP to
// guarantee collision-free lookups across packages.
type ContextKeyPrincipal struct{}

// WithPrincipal returns a derived context that carries the given principal.
//
// Callers should also propagate the new context to Fiber via
// c.SetUserContext(...) so downstream handlers see the same value.
//
// A nil ctx is normalized to context.Background() so the middleware can
// stamp a Principal even on contexts whose lifecycle wasn't fully attached.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	return context.WithValue(ctx, ContextKeyPrincipal{}, p)
}

// GetPrincipal extracts the authenticated principal from context.
//
// The second return value indicates whether a principal was found — callers
// MUST honor it to distinguish an unauthenticated request (background
// workers, system events) from an authenticated one whose principal fields
// happen to be zero-valued.
func GetPrincipal(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}

	if v := ctx.Value(ContextKeyPrincipal{}); v != nil {
		if p, ok := v.(Principal); ok {
			return p, true
		}
	}

	return Principal{}, false
}
