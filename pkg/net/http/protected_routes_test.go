// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	obsconst "github.com/LerianStudio/lib-observability/v4/constants"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtectedRouteChain_RunsPostAuthMiddlewareAfterAuth(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	chain := ProtectedRouteChain(
		func(c fiber.Ctx) error {
			c.Locals("auth_ran", true)
			return c.Next()
		},
		&ProtectedRouteOptions{
			PostAuthMiddlewares: []fiber.Handler{func(c fiber.Ctx) error {
				assert.Equal(t, true, c.Locals("auth_ran"))
				c.Locals("post_auth_ran", true)
				return c.Next()
			}},
		},
		func(c fiber.Ctx) error {
			assert.Equal(t, true, c.Locals("post_auth_ran"))
			return c.SendStatus(fiber.StatusNoContent)
		},
	)

	// Fiber v3's Get takes (handler any, handlers ...any); a []fiber.Handler
	// cannot spread into ...any, so split the chain across the fixed first
	// handler and a []any tail (mirrors production registerRoute).
	tail := make([]any, len(chain)-1)
	for i, h := range chain[1:] {
		tail[i] = h
	}

	app.Get("/test", chain[0], tail...)

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
}

func TestMarkTrustedAuthAssertion_SetsTrustedLocalsAndTenantContext(t *testing.T) {
	t.Parallel()

	token := mustUnsignedToken(t, jwt.MapClaims{
		"sub":      "user-123",
		"tenantId": "tenant_123",
	})

	app := fiber.New()
	app.Use(MarkTrustedAuthAssertion())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"userID":   c.Locals("user_id"),
			"tenantID": tmcore.GetTenantIDContext(c.Context()),
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestMarkTrustedAuthAssertion_UsesSentinelWhenIdentityClaimMissing(t *testing.T) {
	t.Parallel()

	token := mustUnsignedToken(t, jwt.MapClaims{"tenantId": "tenant_123"})

	app := fiber.New()
	app.Use(MarkTrustedAuthAssertion())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString(c.Locals("user_id").(string))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func mustUnsignedToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	return signed
}

func TestMarkTrustedAuthAssertion_AttestsUUIDTenantForTelemetry(t *testing.T) {
	t.Parallel()

	tenantUUID := uuid.MustParse("0f6e2b3a-1c4d-4e5f-8a9b-0c1d2e3f4a5b")

	cases := []struct {
		name     string
		claims   jwt.MapClaims
		wantOK   bool
		wantID   uuid.UUID
		wantName string
	}{
		{
			name:     "hyphenated uuid with slug",
			claims:   jwt.MapClaims{"sub": "u", "tenantId": tenantUUID.String(), "tenantSlug": "acme"},
			wantOK:   true,
			wantID:   tenantUUID,
			wantName: "acme",
		},
		{
			// The tenant-manager writes the Casdoor org name as 32 hex with no
			// hyphens, so this is the shape production actually carries.
			name:     "unhyphenated uuid without slug",
			claims:   jwt.MapClaims{"sub": "u", "tenantId": strings.ReplaceAll(tenantUUID.String(), "-", "")},
			wantOK:   true,
			wantID:   tenantUUID,
			wantName: "",
		},
		{
			// Passes tmcore.IsValidTenantID but is not a UUID: no attestation.
			name:   "non-uuid claim",
			claims: jwt.MapClaims{"sub": "u", "tenantId": "org_01KHVKQQP6D2N4RDJK0ADEKQX1"},
			wantOK: false,
		},
		{
			name:   "absent claim",
			claims: jwt.MapClaims{"sub": "u"},
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				gotTenant libObservability.AuthenticatedTenant
				gotOK     bool
			)

			app := fiber.New()

			// Mirrors production ordering: telemetry reads the attestation from
			// c.Context() only after the whole chain has returned.
			app.Use(func(c fiber.Ctx) error {
				err := c.Next()
				gotTenant, gotOK = libObservability.AuthenticatedTenantFromContext(c.Context())

				return err
			})
			app.Use(MarkTrustedAuthAssertion())
			app.Get("/test", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", mustUnsignedToken(t, tc.claims)))

			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, fiber.StatusNoContent, resp.StatusCode)

			require.Equal(t, tc.wantOK, gotOK)

			if tc.wantOK {
				assert.Equal(t, tc.wantID, gotTenant.ID)
				assert.Equal(t, tc.wantName, gotTenant.Name)
			}
		})
	}
}

func TestMarkTrustedAuthAssertion_SetsTenantIDContextAlongsideAttestation(t *testing.T) {
	t.Parallel()

	// A non-UUID tenant still reaches the tenant-DB resolver: dropping the
	// attestation must not drop tmcore's tenant id with it.
	const slugTenant = "org_01KHVKQQP6D2N4RDJK0ADEKQX1"

	var got string

	app := fiber.New()
	app.Use(MarkTrustedAuthAssertion())
	app.Get("/test", func(c fiber.Ctx) error {
		got = tmcore.GetTenantIDContext(c.Context())
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s",
		mustUnsignedToken(t, jwt.MapClaims{"sub": "u", "tenantId": slugTenant})))

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusNoContent, resp.StatusCode)
	assert.Equal(t, slugTenant, got)
}

// tenantFieldLogger records the fields bound to each logger derived from it,
// so a test can assert what a handler's log record carried. Every derived
// logger shares one sink because the middleware hands the handler a logger
// two With calls removed from the one seeded here.
type tenantFieldLogger struct {
	sink  *tenantFieldSink
	bound []libLog.Field
}

type tenantFieldSink struct {
	mu      sync.Mutex
	records []tenantFieldRecord
}

type tenantFieldRecord struct {
	message string
	fields  []libLog.Field
}

func newTenantFieldLogger() *tenantFieldLogger {
	return &tenantFieldLogger{sink: &tenantFieldSink{}}
}

func (l *tenantFieldLogger) Log(_ context.Context, _ int, msg string, fields ...any) {
	l.sink.mu.Lock()
	defer l.sink.mu.Unlock()

	l.sink.records = append(l.sink.records, tenantFieldRecord{
		message: msg,
		fields:  append(append([]libLog.Field(nil), l.bound...), libLog.Fields(fields...)...),
	})
}

func (l *tenantFieldLogger) With(fields ...any) libLog.Logger {
	return &tenantFieldLogger{
		sink:  l.sink,
		bound: append(append([]libLog.Field(nil), l.bound...), libLog.Fields(fields...)...),
	}
}

func (l *tenantFieldLogger) WithGroup(_ string) libLog.Logger { return l }

func (l *tenantFieldLogger) Enabled(_ int) bool { return true }

func (l *tenantFieldLogger) Sync(_ context.Context) error { return nil }

func (l *tenantFieldLogger) records() []tenantFieldRecord {
	l.sink.mu.Lock()
	defer l.sink.mu.Unlock()

	return append([]tenantFieldRecord(nil), l.sink.records...)
}

// recordsWithMessage isolates the records a specific emitter produced. The
// non-UUID path also logs a one-time attestation warning, so selecting by
// position would make the assertion depend on which subtest tripped the
// sync.Once first.
func (l *tenantFieldLogger) recordsWithMessage(msg string) []tenantFieldRecord {
	var out []tenantFieldRecord

	for _, rec := range l.records() {
		if rec.message == msg {
			out = append(out, rec)
		}
	}

	return out
}

func tenantFieldsOf(record []libLog.Field) []string {
	var values []string

	for _, field := range record {
		if field.Key != obsconst.AttrKeyTenantID {
			continue
		}

		value, _ := field.Value.(string)
		values = append(values, value)
	}

	return values
}

func TestMarkTrustedAuthAssertion_ReseedsContextLoggerWithTenantID(t *testing.T) {
	t.Parallel()

	// Both tenant shapes, because the re-seed sits outside the UUID branch on
	// purpose: a non-UUID tenant gets no metric attestation, so the log field
	// is the only per-tenant signal it has.
	cases := []struct {
		name     string
		tenantID string
	}{
		{name: "uuid tenant", tenantID: "0f6e2b3a-1c4d-4e5f-8a9b-0c1d2e3f4a5b"},
		{name: "non-uuid tenant", tenantID: "org_01KHVKQQP6D2N4RDJK0ADEKQX1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := newTenantFieldLogger()

			app := fiber.New()

			// Stands in for lib-observability's WithHTTPLogging, which binds
			// the request logger into the context BEFORE the auth chain runs.
			app.Use(func(c fiber.Ctx) error {
				c.SetContext(libObservability.ContextWithLogger(c.Context(), logger))

				return c.Next()
			})
			app.Use(MarkTrustedAuthAssertion())
			app.Get("/test", func(c fiber.Ctx) error {
				libObservability.NewLoggerFromContext(c.Context()).
					Log(c.Context(), libLog.LevelInfo, "handler ran")

				return c.SendStatus(fiber.StatusNoContent)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s",
				mustUnsignedToken(t, jwt.MapClaims{"sub": "u", "tenantId": tc.tenantID})))

			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, fiber.StatusNoContent, resp.StatusCode)

			records := logger.recordsWithMessage("handler ran")
			require.Len(t, records, 1, "the handler must emit exactly one record")
			assert.Equal(t, []string{tc.tenantID}, tenantFieldsOf(records[0].fields),
				"a handler log must carry the raw tenantId claim once, matching the span attribute")

			// The non-UUID warning is itself about this tenant, so it must be
			// attributable to it too - the re-seed sits above that branch.
			for _, warned := range logger.recordsWithMessage(
				"Tenant claim is not a UUID; per-tenant metrics will not be labelled for it") {
				assert.Equal(t, []string{tc.tenantID}, tenantFieldsOf(warned.fields))
			}
		})
	}
}

func TestMarkTrustedAuthAssertion_LeavesContextLoggerAloneWithoutTenantClaim(t *testing.T) {
	t.Parallel()

	logger := newTenantFieldLogger()

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.SetContext(libObservability.ContextWithLogger(c.Context(), logger))

		return c.Next()
	})
	app.Use(MarkTrustedAuthAssertion())
	app.Get("/test", func(c fiber.Ctx) error {
		libObservability.NewLoggerFromContext(c.Context()).
			Log(c.Context(), libLog.LevelInfo, "handler ran")

		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s",
		mustUnsignedToken(t, jwt.MapClaims{"sub": "u"})))

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusNoContent, resp.StatusCode)

	records := logger.recordsWithMessage("handler ran")
	require.Len(t, records, 1)
	assert.Empty(t, tenantFieldsOf(records[0].fields),
		"a token without a tenantId claim must add no tenant.id field, not an empty one")
}
