// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// TestNewRouter verifies that NewRouter constructs a Fiber app with the
// expected routes wired in (metadata indexes, /health, /version).
// We assert behaviour at the public HTTP boundary — status codes for each
// registered endpoint — not internal route metadata.
func TestNewRouter(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	mdi := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	auth := middleware.NewAuthClient("", false, nil)

	// Telemetry needs a non-nil pointer with LibraryName set so the middleware
	// can initialise its tracer without panicking. We don't export any spans —
	// the backing exporter is the default no-op.
	telemetry := &libOpentelemetry.Telemetry{
		TelemetryConfig: libOpentelemetry.TelemetryConfig{LibraryName: "test"},
	}

	app := NewRouter(logger, telemetry, auth, mdi)
	require.NotNil(t, app)

	t.Run("health endpoint registered", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", http.NoBody)

		resp, err := app.Test(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("version endpoint registered", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/version", http.NoBody)

		resp, err := app.Test(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("unknown endpoint returns 404", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/not-a-real-path", http.NoBody)

		resp, err := app.Test(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// TestRegisterRoutesToApp verifies that the metadata-index routes are
// registered on an externally-owned Fiber app. Unknown methods/paths on
// the same prefix should return 404 / 405, proving registration was surgical.
func TestRegisterRoutesToApp(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	// Set up expectations for the success path so the handler proceeds far enough
	// to exercise route dispatch. We validate that the registered handler is
	// reachable — not that the mock returns anything specific.
	mockTransactionRepo.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()
	mockOnboardingRepo.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	mdi := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	auth := middleware.NewAuthClient("", false, nil)

	app := fiber.New()
	// Install a no-op user-context setter so downstream handlers (libCommons.NewTrackingFromContext)
	// receive a non-nil context.
	app.Use(func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return c.Next()
	})

	RegisterRoutesToApp(app, auth, mdi)

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "GET /v1/settings/metadata-indexes registered",
			method:         http.MethodGet,
			path:           "/v1/settings/metadata-indexes",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "PUT on same path not registered",
			method:         http.MethodPut,
			path:           "/v1/settings/metadata-indexes",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "unrelated path returns 404",
			method:         http.MethodGet,
			path:           "/v1/settings/other-thing",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, http.NoBody)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

// TestCreateRouteRegistrar verifies that the returned registrar function
// produces the same routing behaviour as calling RegisterRoutesToApp
// directly. We exercise a single registered route end-to-end to avoid
// asserting on internal Fiber state.
func TestCreateRouteRegistrar(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockOnboardingRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)
	mockTransactionRepo := mbootstrap.NewMockMetadataIndexRepository(ctrl)

	mockTransactionRepo.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()
	mockOnboardingRepo.EXPECT().
		FindAllIndexes(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	mdi := &MetadataIndexHandler{
		OnboardingMetadataRepo:  mockOnboardingRepo,
		TransactionMetadataRepo: mockTransactionRepo,
	}

	auth := middleware.NewAuthClient("", false, nil)

	registrar := CreateRouteRegistrar(auth, mdi)
	require.NotNil(t, registrar)

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return c.Next()
	})

	// Apply the registrar — this is the production usage pattern.
	registrar(app)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/settings/metadata-indexes", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
