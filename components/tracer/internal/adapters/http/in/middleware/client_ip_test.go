// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"net"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
)

// mustParseCIDRs is a test helper that parses CIDR strings into *net.IPNet,
// failing the test on any malformed entry.
func mustParseCIDRs(t *testing.T, cidrs ...string) []*net.IPNet {
	t.Helper()

	nets := make([]*net.IPNet, 0, len(cidrs))

	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		require.NoErrorf(t, err, "failed to parse test CIDR %q", c)

		nets = append(nets, n)
	}

	return nets
}

// extractWithFasthttp builds a real Fiber ctx backed by a fasthttp.RequestCtx so
// the socket peer IP (c.IP() → RemoteAddr) can be set deterministically. The
// Fiber app.Test() harness cannot propagate RemoteAddr (it always reports
// 0.0.0.0), so the trusted-proxy logic — which depends on a real socket IP — is
// exercised here against the underlying connection instead.
func extractWithFasthttp(t *testing.T, app *fiber.App, trusted []*net.IPNet, remoteAddr string, headers map[string]string) string {
	t.Helper()

	fctx := &fasthttp.RequestCtx{}

	if remoteAddr != "" {
		addr, err := net.ResolveTCPAddr("tcp", remoteAddr)
		require.NoErrorf(t, err, "failed to resolve test RemoteAddr %q", remoteAddr)

		fctx.SetRemoteAddr(addr)
	}

	for k, v := range headers {
		fctx.Request.Header.Set(k, v)
	}

	c := app.AcquireCtx(fctx)
	defer app.ReleaseCtx(c)

	return extractClientIP(c, trusted)
}

// TestExtractClientIP_TrustedProxies is the core table covering the safe
// XFF-derivation contract. With no trusted proxies configured, XFF is ignored
// entirely and the socket peer IP wins. With trusted proxies configured, the
// chain is walked right-to-left and the first hop OUTSIDE the trusted set is the
// client.
func TestExtractClientIP_TrustedProxies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		trusted    []string
		remoteAddr string
		xff        string
		want       string
	}{
		{
			name:       "no trusted proxies ignores forged XFF and uses socket IP",
			trusted:    nil,
			remoteAddr: "192.0.2.10:54321",
			xff:        "1.2.3.4, 5.6.7.8",
			want:       "192.0.2.10",
		},
		{
			name:       "no trusted proxies and no XFF uses socket IP",
			trusted:    nil,
			remoteAddr: "192.0.2.11:40000",
			xff:        "",
			want:       "192.0.2.11",
		},
		{
			name:       "trusted proxy chain returns rightmost untrusted hop",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.5:1234",
			xff:        "203.0.113.7, 10.1.2.3, 10.0.0.9",
			want:       "203.0.113.7",
		},
		{
			name:       "single trusted hop with one untrusted client",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.5:1234",
			xff:        "198.51.100.23, 10.0.0.9",
			want:       "198.51.100.23",
		},
		{
			name:       "all hops trusted falls back to socket IP",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.5:1234",
			xff:        "10.1.1.1, 10.2.2.2, 10.0.0.9",
			want:       "10.0.0.5",
		},
		{
			name:       "garbage XFF entries are skipped and socket IP wins",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.5:1234",
			xff:        "not-an-ip, also-garbage",
			want:       "10.0.0.5",
		},
		{
			name:       "garbage trailing hop is treated as untrusted client",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.5:1234",
			xff:        "garbage, 10.0.0.9",
			want:       "10.0.0.5",
		},
		{
			name:       "IPv6 client behind IPv6 trusted proxy",
			trusted:    []string{"fd00::/8"},
			remoteAddr: "[fd00::1]:9000",
			xff:        "2001:db8::1, fd00::2",
			want:       "2001:db8::1",
		},
		{
			name:       "mixed IPv4 trusted proxy with IPv6 client",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.5:1234",
			xff:        "2001:db8::99, 10.0.0.9",
			want:       "2001:db8::99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			headers := map[string]string{}
			if tt.xff != "" {
				headers["X-Forwarded-For"] = tt.xff
			}

			app := fiber.New()
			trusted := mustParseCIDRs(t, tt.trusted...)

			got := extractWithFasthttp(t, app, trusted, tt.remoteAddr, headers)
			assert.Equal(t, tt.want, got, tt.name)
		})
	}
}

// TestClientIPMiddleware_IgnoresXFFWithoutTrustedProxies verifies the default
// constructor (empty trusted-proxy set) never honors a forged X-Forwarded-For
// or X-Real-IP header and always records the socket peer IP.
func TestClientIPMiddleware_IgnoresXFFWithoutTrustedProxies(t *testing.T) {
	t.Parallel()

	got := extractWithFasthttp(t, fiber.New(), nil, "192.0.2.50:33333", map[string]string{
		"X-Forwarded-For": "203.0.113.1",
		"X-Real-IP":       "198.51.100.1",
	})

	assert.Equal(t, "192.0.2.50", got, "forged XFF/X-Real-IP must never override the socket IP")
}

// TestClientIPMiddleware_InjectsIntoContext verifies the full middleware wires
// the resolved IP into the request context via contextutil.
func TestClientIPMiddleware_InjectsIntoContext(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(ClientIPMiddleware())

	var captured string

	app.Get("/test", func(c *fiber.Ctx) error {
		captured = contextutil.GetClientIP(c.UserContext())
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	// app.Test() cannot inject a socket IP, so c.IP() is 0.0.0.0 here; the
	// assertion proves the middleware always populates the context key.
	assert.Equal(t, "0.0.0.0", captured, "middleware must inject a client IP into context")
}
