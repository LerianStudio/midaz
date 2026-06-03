// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"context"
	"net"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/contextutil"
)

// ClientIPMiddleware extracts the client's IP address and injects it into the request context.
//
// The middleware attempts to extract the real client IP by checking headers in the following order:
//  1. X-Forwarded-For (leftmost IP, as it represents the original client)
//  2. X-Real-IP (set by some proxies)
//  3. c.IP() (Fiber's built-in IP extraction, falls back to RemoteAddr)
//
// The extracted IP is validated to ensure it's a valid IP address format.
// If no valid IP is found, defaults to "0.0.0.0".
//
// The IP is stored in context with a type-safe key (contextutil.ContextKeyClientIP{})
// to prevent key collisions. Use pkg/contextutil.GetClientIP() to retrieve it.
//
// Security considerations:
//   - Only uses the leftmost IP from X-Forwarded-For to avoid spoofing by intermediate proxies
//   - Validates IP format before storing to prevent injection
//   - Never logs the full IP address for privacy compliance
func ClientIPMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		clientIP := extractClientIP(c)

		// Inject into context using type-safe key from pkg/contextutil
		ctx := context.WithValue(c.UserContext(), contextutil.ContextKeyClientIP{}, clientIP)
		c.SetUserContext(ctx)

		return c.Next()
	}
}

// extractClientIP extracts the real client IP address from the request.
//
// Priority order:
//  1. X-Forwarded-For (leftmost IP) - Used by proxies and load balancers
//  2. X-Real-IP - Used by some reverse proxies (e.g., nginx)
//  3. Fiber c.IP() - Falls back to direct connection IP
//
// Returns "0.0.0.0" if no valid IP is found.
func extractClientIP(c *fiber.Ctx) string {
	// 1. Try X-Forwarded-For (leftmost IP is the original client)
	if xff := c.Get("X-Forwarded-For"); xff != "" {
		// Split by comma and take the first IP (original client)
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if isValidIP(ip) {
				return ip
			}
		}
	}

	// 2. Try X-Real-IP (set by some proxies)
	if xri := c.Get("X-Real-IP"); xri != "" {
		if isValidIP(xri) {
			return xri
		}
	}

	// 3. Fallback to Fiber's built-in IP extraction
	// c.IP() handles RemoteAddr and other headers automatically
	ip := c.IP()
	if isValidIP(ip) {
		return ip
	}

	// Final fallback if nothing worked
	return "0.0.0.0"
}

// isValidIP checks if the given string is a valid IP address (IPv4 or IPv6).
func isValidIP(ip string) bool {
	if ip == "" {
		return false
	}

	// Remove port if present (e.g., "192.168.1.1:8080" -> "192.168.1.1")
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}

	return net.ParseIP(ip) != nil
}
