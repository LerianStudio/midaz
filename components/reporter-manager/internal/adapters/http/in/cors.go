// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// CORSConfig holds explicit CORS configuration loaded from environment variables.
// Origins, methods, and headers are comma-separated strings passed directly to
// Fiber's CORS middleware instead of relying on wildcard defaults.
type CORSConfig struct {
	AllowedOrigins string
	AllowedMethods string
	AllowedHeaders string
}

// CORSMiddleware returns a Fiber middleware that configures CORS using explicit
// origins from the provided configuration. This prevents wildcard (*) defaults
// from leaking into production environments.
//
// Input strings are sanitized before passing to Fiber's cors.New():
//   - Origins are filtered to remove empty segments and entries with invalid
//     scheme://host format, preventing panics from Fiber's origin validation.
//   - Methods and headers have empty segments stripped from comma-separated
//     values to prevent panics from leading, trailing, or consecutive commas.
func CORSMiddleware(cfg CORSConfig) fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins: sanitizeOrigins(cfg.AllowedOrigins),
		AllowMethods: sanitizeCommaSeparated(cfg.AllowedMethods),
		AllowHeaders: sanitizeCommaSeparated(cfg.AllowedHeaders),
		Next:         corsSkipPath,
	})
}

// corsSkipPath returns true for paths that should bypass CORS processing.
// Health, readiness, version, and Swagger endpoints are infrastructure paths
// that do not serve cross-origin browser requests and should not receive
// CORS headers.
//
// /readyz is the canonical readiness path (see ring:dev-readyz Gate 2).
func corsSkipPath(c *fiber.Ctx) bool {
	path := c.Path()

	switch path {
	case "/health", "/readyz", "/version":
		return true
	}

	return strings.HasPrefix(path, "/swagger")
}

// sanitizeOrigins splits a comma-separated origin string, trims whitespace,
// filters out empty segments, and validates that each remaining origin has a
// well-formed scheme://host structure with no path, query, fragment, or
// userinfo components. The wildcard "*" is preserved as-is since Fiber
// handles it explicitly.
//
// This prevents Fiber's cors.New() from panicking on:
//   - Empty segments from leading/trailing/consecutive commas
//   - Malformed origins like "://missing-scheme" that lack a scheme or host
//   - Origins with fragments, query strings, or paths that fail Fiber's
//     internal origin format validation
func sanitizeOrigins(input string) string {
	parts := strings.Split(input, ",")

	var clean []string

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if p == "*" {
			clean = append(clean, p)
			continue
		}

		if isValidOrigin(p) {
			clean = append(clean, p)
		}
	}

	return strings.Join(clean, ",")
}

// isValidOrigin checks whether a string is a well-formed origin suitable for
// Fiber's CORS middleware. A valid origin must have a scheme and host, and must
// not contain path, query, fragment, or userinfo components. This matches the
// origin format defined in RFC 6454: scheme "://" host [ ":" port ].
func isValidOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return false
	}

	if parsed.Path != "" && parsed.Path != "/" {
		return false
	}

	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}

	if parsed.User != nil {
		return false
	}

	return true
}

// sanitizeCommaSeparated splits a comma-separated string, trims whitespace from
// each segment, filters out empty segments, and rejoins the non-empty parts
// with commas. This guards against Fiber's cors.New() panicking on empty
// strings produced by splitting values with leading, trailing, or consecutive
// commas.
func sanitizeCommaSeparated(input string) string {
	parts := strings.Split(input, ",")

	var clean []string

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}

	return strings.Join(clean, ",")
}
