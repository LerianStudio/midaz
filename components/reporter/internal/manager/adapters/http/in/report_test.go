// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/template"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/rabbitmq"
	redisRepo "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"
	reportSeaweed "github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs/report"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/reporter/internal/manager/services"
)

func TestReportHandler_CreateReport(t *testing.T) {
	t.Parallel()

	tempID := uuid.New()
	reportID := uuid.New()

	tests := []struct {
		name           string
		payload        model.CreateReportInput
		mockSetup      func(mockTempRepo *template.MockRepository, mockReportRepo *report.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Success - Create report",
			payload: model.CreateReportInput{
				TemplateID: tempID.String(),
				Filters:    nil,
			},
			mockSetup: func(mockTempRepo *template.MockRepository, mockReportRepo *report.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository) {
				outputFormat := "pdf"
				mappedFields := map[string]map[string][]string{
					"midaz_onboarding": {
						"account": {"id", "name"},
					},
				}

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&report.Report{
						ID:         reportID,
						TemplateID: tempID,
						Status:     constant.ProcessingStatus,
					}, nil)

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil)
			},
			expectedStatus: fiber.StatusCreated,
			expectError:    false,
		},
		{
			name: "Error - Template not found",
			payload: model.CreateReportInput{
				TemplateID: tempID.String(),
				Filters:    nil,
			},
			mockSetup: func(mockTempRepo *template.MockRepository, mockReportRepo *report.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository) {
				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(nil, nil, "", pkg.ValidateBusinessError(cnErr.ErrEntityNotFound, "", constant.MongoCollectionTemplate))
			},
			expectedStatus: fiber.StatusNotFound,
			expectError:    true,
		},
		{
			name: "Error - Report creation fails",
			payload: model.CreateReportInput{
				TemplateID: tempID.String(),
				Filters:    nil,
			},
			mockSetup: func(mockTempRepo *template.MockRepository, mockReportRepo *report.MockRepository, mockRabbitMQ *rabbitmq.MockProducerRepository) {
				outputFormat := "pdf"
				mappedFields := map[string]map[string][]string{
					"midaz_onboarding": {
						"account": {"id", "name"},
					},
				}

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, cnErr.ErrInternalServer)
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTempRepo := template.NewMockRepository(ctrl)
			mockReportRepo := report.NewMockRepository(ctrl)
			mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

			tt.mockSetup(mockTempRepo, mockReportRepo, mockRabbitMQ)

			svc := &services.UseCase{
				Logger:       log.NewNop(),
				Tracer:       noop.NewTracerProvider().Tracer("test"),
				TemplateRepo: mockTempRepo,
				ReportRepo:   mockReportRepo,
				RabbitMQRepo: mockRabbitMQ,
			}

			handler := &ReportHandler{
				service: svc,
			}

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			app.Post("/v1/reports", func(c *fiber.Ctx) error {
				c.SetUserContext(context.Background())
				return handler.CreateReport(&tt.payload, c)
			})

			payloadBytes, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/v1/reports", bytes.NewReader(payloadBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestReportHandler_CreateReport_SetsReplayHeader(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateID := uuid.New()
	reportID := uuid.New()
	mockTempRepo := template.NewMockRepository(ctrl)
	mockReportRepo := report.NewMockRepository(ctrl)
	mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedis := redisRepo.NewMockRedisRepository(ctrl)

	cachedReport := report.Report{ID: reportID, TemplateID: templateID, Status: constant.ProcessingStatus}
	cachedBytes, err := json.Marshal(cachedReport)
	require.NoError(t, err)

	mockRedis.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return(string(cachedBytes), nil)

	svc := &services.UseCase{
		Logger:       log.NewNop(),
		Tracer:       noop.NewTracerProvider().Tracer("test"),
		TemplateRepo: mockTempRepo,
		ReportRepo:   mockReportRepo,
		RabbitMQRepo: mockRabbitMQ,
		RedisRepo:    mockRedis,
	}
	handler := &ReportHandler{service: svc}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/v1/reports", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		payload := model.CreateReportInput{TemplateID: templateID.String(), Filters: nil}
		return handler.CreateReport(&payload, c)
	})

	req := httptest.NewRequest("POST", "/v1/reports", bytes.NewReader([]byte(`{"templateId":"`+templateID.String()+`","filters":null}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(libConstants.IdempotencyKey, "idem-key")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get(libConstants.IdempotencyReplayed))
}

func TestReportHandler_GetReport(t *testing.T) {
	t.Parallel()

	reportID := uuid.New()
	tempID := uuid.New()

	now := time.Now()

	tests := []struct {
		name           string
		reportID       uuid.UUID
		mockSetup      func(mockReportRepo *report.MockRepository)
		expectedStatus int
		expectError    bool
	}{
		{
			name:     "Success - Get report by ID",
			reportID: reportID,
			mockSetup: func(mockReportRepo *report.MockRepository) {
				mockReportRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&report.Report{
						ID:          reportID,
						TemplateID:  tempID,
						Status:      constant.FinishedStatus,
						CreatedAt:   now,
						CompletedAt: &now,
					}, nil)
			},
			expectedStatus: fiber.StatusOK,
			expectError:    false,
		},
		{
			name:     "Error - Report not found",
			reportID: reportID,
			mockSetup: func(mockReportRepo *report.MockRepository) {
				mockReportRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(nil, pkg.ValidateBusinessError(cnErr.ErrEntityNotFound, "", constant.MongoCollectionReport))
			},
			expectedStatus: fiber.StatusNotFound,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockReportRepo := report.NewMockRepository(ctrl)

			tt.mockSetup(mockReportRepo)

			svc := &services.UseCase{
				Logger:     log.NewNop(),
				Tracer:     noop.NewTracerProvider().Tracer("test"),
				ReportRepo: mockReportRepo,
			}

			handler := &ReportHandler{
				service: svc,
			}

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			app.Get("/v1/reports/:id", func(c *fiber.Ctx) error {
				c.Locals("id", tt.reportID)
				c.SetUserContext(context.Background())
				return handler.GetReport(c)
			})

			req := httptest.NewRequest("GET", "/v1/reports/"+tt.reportID.String(), nil)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if !tt.expectError {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result report.Report
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Equal(t, tt.reportID, result.ID)
			}
		})
	}
}

func TestReportHandler_GetAllReports(t *testing.T) {
	t.Parallel()

	reportID1 := uuid.New()
	reportID2 := uuid.New()
	tempID := uuid.New()

	now := time.Now()

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(mockReportRepo *report.MockRepository)
		expectedStatus int
		expectedLen    int
	}{
		{
			name:        "Success - Get all reports",
			queryParams: "?limit=10&page=1",
			mockSetup: func(mockReportRepo *report.MockRepository) {
				mockReportRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*report.Report{
						{
							ID:          reportID1,
							TemplateID:  tempID,
							Status:      constant.FinishedStatus,
							CreatedAt:   now,
							CompletedAt: &now,
						},
						{
							ID:          reportID2,
							TemplateID:  tempID,
							Status:      constant.ProcessingStatus,
							CreatedAt:   now,
							CompletedAt: nil,
						},
					}, nil)
			},
			expectedStatus: fiber.StatusOK,
			expectedLen:    2,
		},
		{
			name:        "Success - Get all reports with empty result",
			queryParams: "?limit=10&page=1",
			mockSetup: func(mockReportRepo *report.MockRepository) {
				mockReportRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*report.Report{}, nil)
			},
			expectedStatus: fiber.StatusOK,
			expectedLen:    0,
		},
		{
			name:        "Error - Database error",
			queryParams: "?limit=10&page=1",
			mockSetup: func(mockReportRepo *report.MockRepository) {
				mockReportRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(nil, cnErr.ErrInternalServer)
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedLen:    0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockReportRepo := report.NewMockRepository(ctrl)

			tt.mockSetup(mockReportRepo)

			svc := &services.UseCase{
				Logger:     log.NewNop(),
				Tracer:     noop.NewTracerProvider().Tracer("test"),
				ReportRepo: mockReportRepo,
			}

			handler := &ReportHandler{
				service: svc,
			}

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			app.Get("/v1/reports", func(c *fiber.Ctx) error {
				c.SetUserContext(context.Background())
				return handler.GetAllReports(c)
			})

			req := httptest.NewRequest("GET", "/v1/reports"+tt.queryParams, nil)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusOK {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result model.Pagination
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Equal(t, tt.expectedLen, result.Total)
			}
		})
	}
}

func TestReportHandler_GetDownloadReport(t *testing.T) {
	t.Parallel()

	reportID := uuid.New()
	tempID := uuid.New()

	now := time.Now()

	tests := []struct {
		name           string
		reportID       uuid.UUID
		mockSetup      func(mockReportRepo *report.MockRepository, mockTempRepo *template.MockRepository, mockSeaweedFS *reportSeaweed.MockRepository)
		expectedStatus int
		expectError    bool
	}{
		{
			name:     "Success - Download report",
			reportID: reportID,
			mockSetup: func(mockReportRepo *report.MockRepository, mockTempRepo *template.MockRepository, mockSeaweedFS *reportSeaweed.MockRepository) {
				mockReportRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&report.Report{
						ID:          reportID,
						TemplateID:  tempID,
						Status:      constant.FinishedStatus,
						CreatedAt:   now,
						CompletedAt: &now,
					}, nil)

				pdfFmt := "pdf"
				mockTempRepo.EXPECT().
					FindOutputFormatByIDIncludeDeleted(gomock.Any(), tempID).
					Return(&pdfFmt, nil)

				mockSeaweedFS.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return([]byte("PDF content here"), nil)
			},
			expectedStatus: fiber.StatusOK,
			expectError:    false,
		},
		{
			name:     "Error - Report not found",
			reportID: reportID,
			mockSetup: func(mockReportRepo *report.MockRepository, mockTempRepo *template.MockRepository, mockSeaweedFS *reportSeaweed.MockRepository) {
				mockReportRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(nil, pkg.ValidateBusinessError(cnErr.ErrEntityNotFound, "", constant.MongoCollectionReport))
			},
			expectedStatus: fiber.StatusNotFound,
			expectError:    true,
		},
		{
			name:     "Error - Report not finished",
			reportID: reportID,
			mockSetup: func(mockReportRepo *report.MockRepository, mockTempRepo *template.MockRepository, mockSeaweedFS *reportSeaweed.MockRepository) {
				mockReportRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&report.Report{
						ID:         reportID,
						TemplateID: tempID,
						Status:     constant.ProcessingStatus,
						CreatedAt:  now,
					}, nil)
			},
			expectedStatus: fiber.StatusUnprocessableEntity, // ErrReportStatusNotFinished maps to UnprocessableOperationError (422)
			expectError:    true,
		},
		{
			name:     "Error - Template not found",
			reportID: reportID,
			mockSetup: func(mockReportRepo *report.MockRepository, mockTempRepo *template.MockRepository, mockSeaweedFS *reportSeaweed.MockRepository) {
				mockReportRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&report.Report{
						ID:          reportID,
						TemplateID:  tempID,
						Status:      constant.FinishedStatus,
						CreatedAt:   now,
						CompletedAt: &now,
					}, nil)

				mockTempRepo.EXPECT().
					FindOutputFormatByIDIncludeDeleted(gomock.Any(), tempID).
					Return(nil, pkg.ValidateBusinessError(cnErr.ErrEntityNotFound, "", constant.MongoCollectionTemplate))
			},
			expectedStatus: fiber.StatusNotFound,
			expectError:    true,
		},
		{
			name:     "Error - File not found in SeaweedFS",
			reportID: reportID,
			mockSetup: func(mockReportRepo *report.MockRepository, mockTempRepo *template.MockRepository, mockSeaweedFS *reportSeaweed.MockRepository) {
				mockReportRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&report.Report{
						ID:          reportID,
						TemplateID:  tempID,
						Status:      constant.FinishedStatus,
						CreatedAt:   now,
						CompletedAt: &now,
					}, nil)

				pdfFmt2 := "pdf"
				mockTempRepo.EXPECT().
					FindOutputFormatByIDIncludeDeleted(gomock.Any(), tempID).
					Return(&pdfFmt2, nil)

				mockSeaweedFS.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return(nil, cnErr.ErrInternalServer)
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockReportRepo := report.NewMockRepository(ctrl)
			mockTempRepo := template.NewMockRepository(ctrl)
			mockSeaweedFS := reportSeaweed.NewMockRepository(ctrl)

			tt.mockSetup(mockReportRepo, mockTempRepo, mockSeaweedFS)

			svc := &services.UseCase{
				Logger:          log.NewNop(),
				Tracer:          noop.NewTracerProvider().Tracer("test"),
				ReportRepo:      mockReportRepo,
				TemplateRepo:    mockTempRepo,
				ReportSeaweedFS: mockSeaweedFS,
			}

			handler := &ReportHandler{
				service: svc,
			}

			app := fiber.New(fiber.Config{
				DisableStartupMessage: true,
			})

			app.Get("/v1/reports/:id/download", func(c *fiber.Ctx) error {
				c.Locals("id", tt.reportID)
				c.SetUserContext(context.Background())
				return handler.GetDownloadReport(c)
			})

			req := httptest.NewRequest("GET", "/v1/reports/"+tt.reportID.String()+"/download", nil)
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if !tt.expectError {
				assert.Contains(t, resp.Header.Get("Content-Type"), "application/pdf")
				assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")
			}
		})
	}
}

func TestNewReportHandler_NilService(t *testing.T) {
	t.Parallel()

	handler, err := NewReportHandler(nil)

	assert.Nil(t, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service must not be nil")
}

func TestNewReportHandler_ValidService(t *testing.T) {
	t.Parallel()

	svc := &services.UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	handler, err := NewReportHandler(svc)

	assert.NotNil(t, handler)
	require.NoError(t, err)
}

func TestReportHandler_GetAllReports_InvalidQueryParams(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportRepo := report.NewMockRepository(ctrl)

	svc := &services.UseCase{
		Logger:     log.NewNop(),
		Tracer:     noop.NewTracerProvider().Tracer("test"),
		ReportRepo: mockReportRepo,
	}

	handler := &ReportHandler{
		service: svc,
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Get("/v1/reports", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.GetAllReports(c)
	})

	// Send request with invalid query param that triggers validation error
	req := httptest.NewRequest("GET", "/v1/reports?outputFormat=INVALID", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestReportHandler_GetReport_InternalError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reportID := uuid.New()

	mockReportRepo := report.NewMockRepository(ctrl)

	mockReportRepo.EXPECT().
		FindByID(gomock.Any(), reportID).
		Return(nil, cnErr.ErrInternalServer)

	svc := &services.UseCase{
		Logger:     log.NewNop(),
		Tracer:     noop.NewTracerProvider().Tracer("test"),
		ReportRepo: mockReportRepo,
	}

	handler := &ReportHandler{
		service: svc,
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Get("/v1/reports/:id", func(c *fiber.Ctx) error {
		c.Locals("id", reportID)
		c.SetUserContext(context.Background())
		return handler.GetReport(c)
	})

	req := httptest.NewRequest("GET", "/v1/reports/"+reportID.String(), nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
}

func TestReportHandler_CreateReport_InvalidTemplateID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTempRepo := template.NewMockRepository(ctrl)
	mockReportRepo := report.NewMockRepository(ctrl)
	mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

	svc := &services.UseCase{
		Logger:       log.NewNop(),
		Tracer:       noop.NewTracerProvider().Tracer("test"),
		TemplateRepo: mockTempRepo,
		ReportRepo:   mockReportRepo,
		RabbitMQRepo: mockRabbitMQ,
	}

	handler := &ReportHandler{
		service: svc,
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	payload := model.CreateReportInput{
		TemplateID: "not-a-valid-uuid",
		Filters:    nil,
	}

	app.Post("/v1/reports", func(c *fiber.Ctx) error {
		c.SetUserContext(context.Background())
		return handler.CreateReport(&payload, c)
	})

	payloadBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/v1/reports", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}
