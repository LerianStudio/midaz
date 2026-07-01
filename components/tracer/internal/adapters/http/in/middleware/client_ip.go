// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"context"
	"net"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
)

// ClientIPMiddleware extracts the client's IP address and injects it into the
// request context using ClientIPMiddlewareWithTrustedProxies with an empty
// trusted-proxy set. With no trusted proxies, the client-controlled
// X-Forwarded-For header is ignored entirely and the socket peer IP is used.
func ClientIPMiddleware() fiber.Handler {
	return ClientIPMiddlewareWithTrustedProxies(nil)
}

// ClientIPMiddlewareWithTrustedProxies builds the client-IP middleware with a
// trusted-proxy CIDR set. The extracted IP is stored in context with a
// type-safe key (contextutil.ContextKeyClientIP{}); retrieve it with
// contextutil.GetClientIP().
//
// Because the audit trail records this IP into durable, hash-chained records,
// it must never be forgeable by the client. X-Forwarded-For is only consulted
// when at least one trusted proxy is configured, and even then only hops that
// sit behind the trusted set are believed — see extractClientIP for the
// right-to-left walk.
func ClientIPMiddlewareWithTrustedProxies(trustedProxies []*net.IPNet) fiber.Handler {
	return func(c *fiber.Ctx) error {
		clientIP := extractClientIP(c, trustedProxies)

		ctx := context.WithValue(c.UserContext(), contextutil.ContextKeyClientIP{}, clientIP)
		c.SetUserContext(ctx)

		return c.Next()
	}
}

// extractClientIP derives the trustworthy client IP for the request.
//
// The socket peer IP (Fiber c.IP(), which reads the TCP RemoteAddr) is the only
// value the client cannot forge, so it is the authoritative default and the
// fallback for every ambiguous case.
//
// When trustedProxies is non-empty the X-Forwarded-For chain is walked from
// RIGHT (closest proxy) to LEFT (claimed origin), skipping every hop that falls
// inside the trusted set. The first hop NOT in the trusted set is the real
// client: a trusted proxy appended it, so it is the furthest point we can still
// believe. If every hop is trusted (or the header is empty/garbage), the socket
// IP is used. When trustedProxies is empty the header is ignored outright.
//
// Returns "0.0.0.0" only when even the socket IP is unparseable.
func extractClientIP(c *fiber.Ctx, trustedProxies []*net.IPNet) string {
	socketIP := normalizeSocketIP(c.IP())

	if len(trustedProxies) == 0 {
		return socketIP
	}

	xff := c.Get("X-Forwarded-For")
	if xff == "" {
		return socketIP
	}

	hops := strings.Split(xff, ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(hops[i])

		parsed := net.ParseIP(hop)
		if parsed == nil {
			// A garbage hop breaks the trust chain: we cannot prove the hop to
			// its left was appended by a trusted proxy, so we stop believing
			// the header and fall back to the socket IP.
			return socketIP
		}

		if !ipInAnyCIDR(parsed, trustedProxies) {
			return hop
		}
	}

	// Every hop was trusted — the header carries no client beyond the proxies.
	return socketIP
}

// ipInAnyCIDR reports whether ip is contained in any of the given networks.
func ipInAnyCIDR(ip net.IP, networks []*net.IPNet) bool {
	for _, n := range networks {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}

// normalizeSocketIP strips any port from Fiber's c.IP() value and validates it,
// returning "0.0.0.0" when no valid IP can be recovered.
func normalizeSocketIP(ip string) string {
	if ip == "" {
		return "0.0.0.0"
	}

	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}

	if net.ParseIP(ip) == nil {
		return "0.0.0.0"
	}

	return ip
}
