// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package contextutil

import "context"

// ContextKeyClientIP is the type-safe key for storing client IP in context.
// Exported so ClientIPMiddleware can use the same key.
type ContextKeyClientIP struct{}

// GetClientIP extracts the client IP address from the request context.
//
// The IP is injected by the ClientIPMiddleware in the HTTP layer, which extracts
// the real client IP considering proxies and load balancers (X-Forwarded-For, X-Real-IP).
//
// This function is architecture-neutral and can be used by any layer (services, handlers, etc.)
// without creating dependency violations.
//
// Returns "0.0.0.0" if the IP is not found in the context (e.g., internal calls, tests).
//
// Usage:
//
//	clientIP := contextutil.GetClientIP(ctx)
//	// Use clientIP for audit logging, rate limiting, etc.
func GetClientIP(ctx context.Context) string {
	if ip, ok := ctx.Value(ContextKeyClientIP{}).(string); ok && ip != "" {
		return ip
	}

	return "0.0.0.0"
}
