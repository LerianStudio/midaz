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
	libStreaming "github.com/LerianStudio/lib-streaming"
	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// topicPattern locks the emitted manifest topics to the {service}.{resource}.{event}
// underscore grammar with the bare "ledger" service prefix (no lerian.streaming.*).
var topicPattern = regexp.MustCompile(`^ledger\.[a-z0-9_]+\.[a-z0-9_]+$`)

// newTestManifestHandler builds a real lib-streaming manifest handler from a
// minimal ledger catalog, mirroring how bootstrap.buildCatalog constructs
// EventDefinition entries (Key/ResourceType/EventType/SchemaVersion).
func newTestManifestHandler(t *testing.T) stdhttp.Handler {
	t.Helper()

	catalog, err := libStreaming.NewCatalog(
		libStreaming.EventDefinition{
			Key:           "account.created",
			ResourceType:  "account",
			EventType:     "created",
			SchemaVersion: "1.0.0",
		},
		libStreaming.EventDefinition{
			Key:           "operation_route.created",
			ResourceType:  "operation_route",
			EventType:     "created",
			SchemaVersion: "1.0.0",
		},
	)
	require.NoError(t, err)

	// ServiceName == SourceBase == "ledger": SourceBase is the first topic segment.
	h, err := pkgStreaming.NewManifestHTTPHandler("ledger", "ledger", catalog)
	require.NoError(t, err)

	return h
}

type manifestTopicsBody struct {
	Events []struct {
		Topic string `json:"topic"`
	} `json:"events"`
}

func TestRegisterStreamingManifestRoute_Serves200JSON(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	// Disabled auth client mirrors PLUGIN_AUTH_ENABLED=false: Authorize is a
	// pass-through, so the protected chain (product "midaz", resource
	// "streaming-manifest", action "get") does not short-circuit the request.
	auth := &middleware.AuthClient{Enabled: false}

	RegisterStreamingManifestRoute(app, auth, newTestManifestHandler(t), &pkgHTTP.ProtectedRouteOptions{})

	req := httptest.NewRequest(fiber.MethodGet, pkgStreaming.ManifestRoutePath, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var body manifestTopicsBody
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotEmpty(t, body.Events, "manifest should advertise at least one event")

	for _, e := range body.Events {
		assert.Regexp(t, topicPattern, e.Topic,
			"topic must be {service}.{resource}.{event} with underscores and no lerian.streaming prefix")
	}
}

func TestRegisterStreamingManifestRoute_RegistersRoute(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	RegisterStreamingManifestRoute(app, auth, newTestManifestHandler(t), &pkgHTTP.ProtectedRouteOptions{})

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	assert.True(t, routeSet[fiber.MethodGet+":"+pkgStreaming.ManifestRoutePath],
		"should register GET streaming manifest route")
}

func TestRegisterStreamingManifestRoute_NilHandler_NotMounted(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	auth := &middleware.AuthClient{Enabled: false}

	// Degraded path: a nil handler (manifest build failed at bootstrap) must
	// leave the route unmounted rather than mount a nil handler.
	RegisterStreamingManifestRoute(app, auth, nil, &pkgHTTP.ProtectedRouteOptions{})

	req := httptest.NewRequest(fiber.MethodGet, pkgStreaming.ManifestRoutePath, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}
