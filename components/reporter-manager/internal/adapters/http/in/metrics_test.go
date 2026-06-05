// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/components/reporter-manager/internal/services"
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/template"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func setupMetricsTestApp(handler *MetricsHandler) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	return app
}

func setupMetricsContextMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		tracer := noop.NewTracerProvider().Tracer("test")

		ctx := ctxutil.ContextWithLogger(c.UserContext(), log.NewNop())
		ctx = ctxutil.ContextWithTracer(ctx, tracer)

		c.SetUserContext(ctx)

		return c.Next()
	}
}

func TestNewMetricsHandler_NilService(t *testing.T) {
	t.Parallel()

	handler, err := NewMetricsHandler(nil)

	assert.Nil(t, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service must not be nil")
}

func TestNewMetricsHandler_ValidService(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svc := &services.UseCase{
		Logger:              log.NewNop(),
		Tracer:              noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:        template.NewMockRepository(ctrl),
		ReportRepo:          report.NewMockRepository(ctrl),
		ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
	}

	handler, err := NewMetricsHandler(svc)

	assert.NotNil(t, handler)
	require.NoError(t, err)
}

// MetricsResponse represents the expected JSON response from GET /v1/metrics.
type MetricsResponse struct {
	Templates   int64        `json:"templates"`
	Reports     int64        `json:"reports"`
	DataSources int64        `json:"dataSources"`
	Errors      ErrorMetrics `json:"errors"`
}

// ErrorMetrics represents the error counters in the metrics response.
type ErrorMetrics struct {
	Total               int64 `json:"total"`
	PreviousPeriodTotal int64 `json:"previousPeriodTotal"`
}

func TestMetricsHandler_GetMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository)
		dataSources    map[string]pkg.DataSource
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:        "Success - Get metrics with default errorPeriodDays",
			queryParams: "",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(42), nil)

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(156), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(7), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(12), nil)
			},
			dataSources: map[string]pkg.DataSource{
				"postgres-main":   {},
				"mongo-analytics": {},
				"postgres-crm":    {},
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp MetricsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, int64(42), resp.Templates)
				assert.Equal(t, int64(156), resp.Reports)
				assert.Equal(t, int64(3), resp.DataSources)
				assert.Equal(t, int64(7), resp.Errors.Total)
				assert.Equal(t, int64(12), resp.Errors.PreviousPeriodTotal)
			},
		},
		{
			name:        "Success - Get metrics with custom errorPeriodDays",
			queryParams: "?errorPeriodDays=30",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(10), nil)

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(50), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(3), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(5), nil)
			},
			dataSources: map[string]pkg.DataSource{
				"postgres-main": {},
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp MetricsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, int64(10), resp.Templates)
				assert.Equal(t, int64(50), resp.Reports)
				assert.Equal(t, int64(1), resp.DataSources)
				assert.Equal(t, int64(3), resp.Errors.Total)
				assert.Equal(t, int64(5), resp.Errors.PreviousPeriodTotal)
			},
		},
		{
			name:        "Success - Get metrics with zero data sources",
			queryParams: "",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(0), nil)

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(0), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(0), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp MetricsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, int64(0), resp.Templates)
				assert.Equal(t, int64(0), resp.Reports)
				assert.Equal(t, int64(0), resp.DataSources)
				assert.Equal(t, int64(0), resp.Errors.Total)
				assert.Equal(t, int64(0), resp.Errors.PreviousPeriodTotal)
			},
		},
		{
			name:        "Success - Get metrics with errorPeriodDays=1 (minimum)",
			queryParams: "?errorPeriodDays=1",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(5), nil)

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(20), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(1), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(2), nil)
			},
			dataSources: map[string]pkg.DataSource{
				"postgres-main": {},
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp MetricsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, int64(1), resp.Errors.Total)
				assert.Equal(t, int64(2), resp.Errors.PreviousPeriodTotal)
			},
		},
		{
			name:        "Success - Get metrics with errorPeriodDays=365 (maximum)",
			queryParams: "?errorPeriodDays=365",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(5), nil)

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(20), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(10), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(15), nil)
			},
			dataSources: map[string]pkg.DataSource{
				"postgres-main": {},
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp MetricsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, int64(10), resp.Errors.Total)
				assert.Equal(t, int64(15), resp.Errors.PreviousPeriodTotal)
			},
		},
		{
			name:        "Error 400 - errorPeriodDays is zero",
			queryParams: "?errorPeriodDays=0",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				// No repository calls expected for validation errors
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Contains(t, resp["message"], "errorPeriodDays must be between")
			},
		},
		{
			name:        "Error 400 - errorPeriodDays is negative",
			queryParams: "?errorPeriodDays=-5",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				// No repository calls expected for validation errors
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error 400 - errorPeriodDays exceeds 365",
			queryParams: "?errorPeriodDays=366",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				// No repository calls expected for validation errors
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error 400 - errorPeriodDays is not a number",
			queryParams: "?errorPeriodDays=abc",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				// No repository calls expected for validation errors
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error 400 - errorPeriodDays is a float",
			queryParams: "?errorPeriodDays=7.5",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				// No repository calls expected for validation errors
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error 400 - errorPeriodDays is special characters",
			queryParams: "?errorPeriodDays=<script>",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				// No repository calls expected for validation errors
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Contains(t, resp["message"], "errorPeriodDays must be an integer")
			},
		},
		{
			name:        "Error 400 - errorPeriodDays is empty string",
			queryParams: "?errorPeriodDays=",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				// Empty string treated as absent, so repos will be called
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(1), nil)

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(1), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(0), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
			},
			dataSources: map[string]pkg.DataSource{
				"ds1": {},
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp MetricsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				// Empty string falls through to default (7 days)
				assert.Equal(t, int64(1), resp.Templates)
			},
		},
		{
			name:        "Error 400 - errorPeriodDays is very large negative",
			queryParams: "?errorPeriodDays=-999999",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				// No repository calls expected for validation errors
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Contains(t, resp["message"], "errorPeriodDays must be between")
			},
		},
		{
			name:        "Error 500 - Template CountAll fails",
			queryParams: "",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(0), errors.New("database connection error"))

				// Other counts may or may not be called depending on parallel execution,
				// so we use AnyTimes to avoid flaky test expectations.
				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(0), nil).
					AnyTimes()

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil).
					AnyTimes()
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusInternalServerError,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "TPL-0018", resp["code"])
				assert.Equal(t, "Internal Server Error", resp["title"])
				assert.Contains(t, resp["message"], "The server encountered an unexpected error")
			},
		},
		{
			name:        "Error 500 - Report CountAll fails",
			queryParams: "",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(0), nil).
					AnyTimes()

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(0), errors.New("database connection error"))

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil).
					AnyTimes()
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Error 500 - Report CountByStatus (current period) fails",
			queryParams: "",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(0), nil).
					AnyTimes()

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(0), nil).
					AnyTimes()

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), errors.New("query timeout")).
					AnyTimes()
			},
			dataSources:    map[string]pkg.DataSource{},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:        "Success - Get metrics with many data sources",
			queryParams: "",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(100), nil)

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(500), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(25), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(30), nil)
			},
			dataSources: func() map[string]pkg.DataSource {
				ds := make(map[string]pkg.DataSource)
				for i := 0; i < 50; i++ {
					ds[fmt.Sprintf("ds-%d", i)] = pkg.DataSource{}
				}
				return ds
			}(),
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp MetricsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, int64(100), resp.Templates)
				assert.Equal(t, int64(500), resp.Reports)
				assert.Equal(t, int64(50), resp.DataSources)
				assert.Equal(t, int64(25), resp.Errors.Total)
				assert.Equal(t, int64(30), resp.Errors.PreviousPeriodTotal)
			},
		},
		{
			name:        "Success - Get metrics with large counts",
			queryParams: "?errorPeriodDays=7",
			mockSetup: func(mockTemplateRepo *template.MockRepository, mockReportRepo *report.MockRepository) {
				mockTemplateRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(999999), nil)

				mockReportRepo.EXPECT().
					CountAll(gomock.Any()).
					Return(int64(999999), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(999999), nil)

				mockReportRepo.EXPECT().
					CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
					Return(int64(999999), nil)
			},
			dataSources: map[string]pkg.DataSource{
				"ds1": {},
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp MetricsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, int64(999999), resp.Templates)
				assert.Equal(t, int64(999999), resp.Reports)
				assert.Equal(t, int64(999999), resp.Errors.Total)
				assert.Equal(t, int64(999999), resp.Errors.PreviousPeriodTotal)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTemplateRepo := template.NewMockRepository(ctrl)
			mockReportRepo := report.NewMockRepository(ctrl)

			tt.mockSetup(mockTemplateRepo, mockReportRepo)

			safeDatasources := pkg.NewSafeDataSources(tt.dataSources)

			useCase := &services.UseCase{
				Logger:              log.NewNop(),
				Tracer:              noop.NewTracerProvider().Tracer("test"),
				TemplateRepo:        mockTemplateRepo,
				ReportRepo:          mockReportRepo,
				ExternalDataSources: safeDatasources,
			}

			handler, err := NewMetricsHandler(useCase)
			require.NoError(t, err)

			app := setupMetricsTestApp(handler)
			app.Get("/v1/metrics", setupMetricsContextMiddleware(), handler.GetMetrics)

			req := httptest.NewRequest(http.MethodGet, "/v1/metrics"+tt.queryParams, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.checkBody != nil {
				body, readErr := io.ReadAll(resp.Body)
				require.NoError(t, readErr)
				tt.checkBody(t, body)
			}
		})
	}
}
