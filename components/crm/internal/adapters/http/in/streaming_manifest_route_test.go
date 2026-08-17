// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// crmTopicPattern locks the emitted manifest topics to the
// {service}.{resource}.{event} underscore grammar with the bare "crm" service
// prefix (no lerian.streaming.* prefix).
var crmTopicPattern = regexp.MustCompile(`^crm\.[a-z0-9_]+\.[a-z0-9_]+$`)

// newTestCRMManifestHandler builds a real lib-streaming manifest handler from a
// minimal CRM catalog, mirroring how bootstrap.buildCatalog constructs
// EventDefinition entries (Key/ResourceType/EventType/SchemaVersion). It exercises
// the underscore-canonical multi-segment event key (related_party_deleted).
func newTestCRMManifestHandler(t *testing.T) stdhttp.Handler {
	t.Helper()

	catalog, err := libStreaming.NewCatalog(
		libStreaming.EventDefinition{
			Key:           "holder.created",
			ResourceType:  "holder",
			EventType:     "created",
			SchemaVersion: "1.0.0",
		},
		libStreaming.EventDefinition{
			Key:           "alias.related_party_deleted",
			ResourceType:  "alias",
			EventType:     "related_party_deleted",
			SchemaVersion: "1.0.0",
		},
	)
	require.NoError(t, err)

	// ServiceName == SourceBase == "crm": SourceBase is the first topic segment.
	h, err := pkgStreaming.NewManifestHTTPHandler("crm", "crm", catalog)
	require.NoError(t, err)

	return h
}

type crmManifestTopicsBody struct {
	Publisher struct {
		ServiceName string `json:"serviceName"`
		SourceBase  string `json:"sourceBase"`
	} `json:"publisher"`
	Events []struct {
		Topic string `json:"topic"`
	} `json:"events"`
}

// newTestCRMRouter builds a CRM router with a disabled auth client so
// auth.Authorize is a pass-through, isolating the manifest route wiring.
func newTestCRMRouter(t *testing.T, tenantMw fiber.Handler, manifestHandler stdhttp.Handler) *fiber.App {
	t.Helper()

	return NewRouter(
		&libLog.GoLogger{},
		&libOpentelemetry.Telemetry{},
		&middleware.AuthClient{Enabled: false},
		tenantMw,
		nil, // ReadyzHandler
		&HolderHandler{},
		&AliasHandler{},
		nil, // EncryptionHandler
		nil, // AuditHandler
		manifestHandler,
	)
}

func TestNewRouter_StreamingManifest_Serves200JSON(t *testing.T) {
	t.Parallel()

	app := newTestCRMRouter(t, nil, newTestCRMManifestHandler(t))

	req := httptest.NewRequest(fiber.MethodGet, pkgStreaming.ManifestRoutePath, nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var body crmManifestTopicsBody
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotEmpty(t, body.Events, "manifest should advertise at least one event")

	assert.Equal(t, "crm", body.Publisher.ServiceName, "publisher.serviceName must be the bare service name")
	assert.Equal(t, "crm", body.Publisher.SourceBase, "publisher.sourceBase must be the bare service name")

	for _, e := range body.Events {
		assert.Regexp(t, crmTopicPattern, e.Topic,
			"topic must be {service}.{resource}.{event} with underscores and a bare crm prefix")
	}
}

func TestNewRouter_StreamingManifest_NilHandler_NotMounted(t *testing.T) {
	t.Parallel()

	// Degraded path: a nil handler (manifest build failed at bootstrap) must
	// leave the route unmounted rather than mount a nil handler.
	app := newTestCRMRouter(t, nil, nil)

	req := httptest.NewRequest(fiber.MethodGet, pkgStreaming.ManifestRoutePath, nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestNewRouter_StreamingManifest_ReachableWithoutTenantContext(t *testing.T) {
	t.Parallel()

	// rejectingTenantMw rejects every request without a tenant header. If the
	// manifest route were registered AFTER the tenant middleware, this would
	// short-circuit the manifest fetch with 401.
	rejectingTenantMw := func(c *fiber.Ctx) error {
		if c.Get("X-Tenant-ID") == "" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		return c.Next()
	}

	app := newTestCRMRouter(t, rejectingTenantMw, newTestCRMManifestHandler(t))

	// Manifest is control-plane: registered BEFORE the tenant middleware, so it
	// is reachable without any tenant context.
	manifestReq := httptest.NewRequest(fiber.MethodGet, pkgStreaming.ManifestRoutePath, nil)
	manifestResp, err := app.Test(manifestReq, -1)
	require.NoError(t, err)
	defer manifestResp.Body.Close()

	assert.Equal(t, fiber.StatusOK, manifestResp.StatusCode,
		"manifest must be reachable without tenant context (registered before tenant middleware)")

	// A regular API route still requires tenant context, proving the tenant
	// middleware is active for everything registered after it.
	apiReq := httptest.NewRequest(fiber.MethodGet, "/v1/holders", nil)
	apiResp, err := app.Test(apiReq, -1)
	require.NoError(t, err)
	defer apiResp.Body.Close()

	assert.Equal(t, fiber.StatusUnauthorized, apiResp.StatusCode,
		"API routes must remain gated by the tenant middleware")
}
