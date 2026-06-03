// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/datasource"
	"github.com/LerianStudio/reporter/pkg/model"
	"github.com/LerianStudio/reporter/pkg/mongodb/report"
	"github.com/LerianStudio/reporter/pkg/mongodb/template"
	"github.com/LerianStudio/reporter/pkg/rabbitmq"
	"github.com/LerianStudio/reporter/pkg/redis"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

func TestUseCase_CreateReport(t *testing.T) {
	t.Parallel()

	reportId := uuid.New()
	tempId := uuid.New()

	mappedFields := map[string]map[string][]string{
		"midaz_transaction_metadata": {
			"transaction": {"metadata"},
		},
		"midaz_onboarding": {
			"asset":        {"name", "type", "code"},
			"organization": {"legal_document", "legal_name", "doing_business_as", "address"},
			"ledger":       {"name", "status"},
		},
	}

	outputFormat := "xml"

	reportInput := &model.CreateReportInput{
		TemplateID: tempId.String(),
		Filters:    nil,
	}

	reportEntity := &report.Report{
		ID:         reportId,
		TemplateID: tempId,
		Filters:    nil,
		Status:     "processing",
	}

	tests := []struct {
		name           string
		reportInput    *model.CreateReportInput
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		errContains    string
		expectedResult *report.Report
	}{
		{
			name:        "Success - Create a report",
			reportInput: reportInput,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(reportEntity, nil)

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
				}
			},
			expectErr: false,
			expectedResult: &report.Report{
				ID:         reportId,
				TemplateID: tempId,
				Filters:    nil,
				Status:     "processing",
			},
		},
		{
			name:        "Error - Find mapped fields and output format",
			reportInput: reportInput,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(nil, nil, "", constant.ErrInternalServer)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
				}
			},
			expectErr:      true,
			errContains:    constant.ErrInternalServer.Error(),
			expectedResult: nil,
		},
		{
			name:        "Error - Create report",
			reportInput: reportInput,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInternalServer)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
				}
			},
			expectErr:      true,
			errContains:    constant.ErrInternalServer.Error(),
			expectedResult: nil,
		},
		{
			name:        "Error - Send message on RabbitMQ",
			reportInput: reportInput,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(reportEntity, nil)

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInternalServer)

				// Expect the report status to be updated to error when queue send fails
				mockReportRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), constant.ErrorStatus, reportId, gomock.Any(), gomock.Any()).
					Return(nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
				}
			},
			expectErr:      true,
			errContains:    constant.ErrInternalServer.Error(),
			expectedResult: nil,
		},
		{
			name: "Error - Invalid template ID (not a UUID)",
			reportInput: &model.CreateReportInput{
				TemplateID: "not-a-valid-uuid",
				Filters:    nil,
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: template.NewMockRepository(ctrl),
					ReportRepo:   report.NewMockRepository(ctrl),
					RabbitMQRepo: rabbitmq.NewMockProducerRepository(ctrl),
				}
			},
			expectErr:      true,
			errContains:    "not a valid UUID",
			expectedResult: nil,
		},
		{
			name:        "Error - Template not found (ErrNoDocuments)",
			reportInput: reportInput,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(nil, nil, "", mongo.ErrNoDocuments)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
				}
			},
			expectErr:      true,
			errContains:    "template",
			expectedResult: nil,
		},
		{
			name: "Error - Filters validation fails",
			reportInput: &model.CreateReportInput{
				TemplateID: tempId.String(),
				Filters: map[string]map[string]map[string]model.FilterCondition{
					"midaz_onboarding": {
						"invalid_table": {
							"field": {Equals: []any{"value"}},
						},
					},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)
				mockProvider := datasource.NewMockDataSourceProvider(ctrl)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockProvider.EXPECT().
					ValidateSchema(gomock.Any(), "midaz_onboarding", gomock.Any()).
					Return(nil, errors.New("data source \"midaz_onboarding\" not found"))

				return &UseCase{
					Logger:             log.NewNop(),
					Tracer:             noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:       mockTempRepo,
					ReportRepo:         mockReportRepo,
					RabbitMQRepo:       mockRabbitMQ,
					DataSourceProvider: mockProvider,
				}
			},
			expectErr:      true,
			errContains:    "data source",
			expectedResult: nil,
		},
		{
			name:        "Error - Queue send fails and status update also fails",
			reportInput: reportInput,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(reportEntity, nil)

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInternalServer)

				// Expect the report status update to also fail
				mockReportRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), constant.ErrorStatus, reportId, gomock.Any(), gomock.Any()).
					Return(constant.ErrInternalServer)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
				}
			},
			expectErr:      true,
			errContains:    constant.ErrInternalServer.Error(),
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			reportSvc := tt.mockSetup(ctrl)

			ctx := context.Background()
			result, err := reportSvc.CreateReport(ctx, tt.reportInput)

			if tt.expectErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

// hashRequestBody computes a SHA256 hash of the JSON-serialized request body.
// This is a test helper that mirrors the expected hashing logic in the idempotency implementation.
func hashRequestBody(t *testing.T, input *model.CreateReportInput) string {
	t.Helper()

	data, err := json.Marshal(input)
	require.NoError(t, err, "failed to marshal report input for hash computation")

	return commons.HashSHA256(string(data))
}

func TestUseCase_CreateReport_Idempotency(t *testing.T) {
	t.Parallel()

	reportID := uuid.New()
	tempID := uuid.New()

	mappedFields := map[string]map[string][]string{
		"midaz_onboarding": {
			"organization": {"legal_name", "legal_document"},
		},
	}

	outputFormat := "html"

	reportInput := &model.CreateReportInput{
		TemplateID: tempID.String(),
		Filters:    nil,
	}

	reportEntity := &report.Report{
		ID:         reportID,
		TemplateID: tempID,
		Filters:    nil,
		Status:     constant.ProcessingStatus,
	}

	// Pre-compute the expected idempotency key based on the request body hash
	expectedHash := hashRequestBody(t, reportInput)
	expectedIdempotencyKey := "idempotency:" + expectedHash

	// Pre-compute the expected cached response JSON
	cachedResponseJSON, err := json.Marshal(reportEntity)
	require.NoError(t, err, "failed to marshal report entity for cached response")

	idempotencyTTL := constant.IdempotencyTTL

	tests := []struct {
		name           string
		reportInput    *model.CreateReportInput
		idempotencyKey string
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		expectedResult *report.Report
		description    string
	}{
		{
			name:           "Success - First call creates report with idempotency lock",
			reportInput:    reportInput,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				// Expect SetNX to be called with the hash-based key BEFORE report creation
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(reportEntity, nil)

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil)

				// After successful creation, expect the result to be cached in Redis
				mockRedisRepo.EXPECT().
					Set(gomock.Any(), expectedIdempotencyKey, string(cachedResponseJSON), idempotencyTTL).
					Return(nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
					RedisRepo:    mockRedisRepo,
				}
			},
			expectErr: false,
			expectedResult: &report.Report{
				ID:         reportID,
				TemplateID: tempID,
				Filters:    nil,
				Status:     constant.ProcessingStatus,
			},
			description: "The first call must acquire the SetNX lock, create the report, " +
				"send to RabbitMQ, and cache the response for future duplicates.",
		},
		{
			name:           "Success - Duplicate request returns cached response (no new report created)",
			reportInput:    reportInput,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				// SetNX returns false: key already exists (duplicate request)
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(false, nil)

				// Expect Get to retrieve the cached response
				mockRedisRepo.EXPECT().
					Get(gomock.Any(), expectedIdempotencyKey).
					Return(string(cachedResponseJSON), nil)

				// NO calls to TemplateRepo, ReportRepo, or RabbitMQ should happen
				// (gomock will fail if unexpected calls are made)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
					RedisRepo:    mockRedisRepo,
				}
			},
			expectErr: false,
			expectedResult: &report.Report{
				ID:         reportID,
				TemplateID: tempID,
				Filters:    nil,
				Status:     constant.ProcessingStatus,
			},
			description: "When SetNX returns false, it means a duplicate request. " +
				"The service must return the cached response without creating a new report or publishing to RabbitMQ.",
		},
		{
			name: "Success - Different request body creates a different report",
			reportInput: &model.CreateReportInput{
				TemplateID: uuid.New().String(),
				Filters:    nil,
			},
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				// Different body produces a different hash, so SetNX succeeds (new key)
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), gomock.Not(gomock.Eq(expectedIdempotencyKey)), gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(reportEntity, nil)

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil)

				mockRedisRepo.EXPECT().
					Set(gomock.Any(), gomock.Not(gomock.Eq(expectedIdempotencyKey)), gomock.Any(), idempotencyTTL).
					Return(nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
					RedisRepo:    mockRedisRepo,
				}
			},
			expectErr:      false,
			expectedResult: reportEntity,
			description: "A request with a different body produces a different hash, " +
				"so SetNX succeeds and a new report is created normally.",
		},
		{
			name:           "Success - Client-provided Idempotency-Key header is used instead of hash",
			reportInput:    reportInput,
			idempotencyKey: "client-provided-unique-key-12345",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				// When client provides an explicit key, it is used instead of hashing the body
				clientIdempotencyKey := "idempotency:client-provided-unique-key-12345"

				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), clientIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(reportEntity, nil)

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil)

				mockRedisRepo.EXPECT().
					Set(gomock.Any(), clientIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
					RedisRepo:    mockRedisRepo,
				}
			},
			expectErr:      false,
			expectedResult: reportEntity,
			description: "When the client provides an Idempotency-Key header, the service must " +
				"use that key instead of computing a hash from the request body.",
		},
		{
			name:           "Error - Duplicate in-flight request (SetNX false, no cached value yet)",
			reportInput:    reportInput,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				// SetNX returns false: key already exists
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(false, nil)

				// Get returns empty string: first request is still processing
				mockRedisRepo.EXPECT().
					Get(gomock.Any(), expectedIdempotencyKey).
					Return("", nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
					RedisRepo:    mockRedisRepo,
				}
			},
			expectErr:      true,
			expectedResult: nil,
			description: "When SetNX returns false and Get returns empty, it means the first request " +
				"is still in-flight. The service must return an error indicating a duplicate in-flight request.",
		},
		{
			name:           "Error - Template lookup failure releases idempotency lock",
			reportInput:    reportInput,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(nil, nil, "", constant.ErrEntityNotFound)

				mockRedisRepo.EXPECT().
					Del(gomock.Any(), expectedIdempotencyKey).
					Return(nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
					RedisRepo:    mockRedisRepo,
				}
			},
			expectErr:      true,
			expectedResult: nil,
			description:    "Failures after acquiring the idempotency lock must release the key so retries are not blocked.",
		},
		{
			name:           "Success - Cache write failure releases idempotency lock",
			reportInput:    reportInput,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&outputFormat, mappedFields, "Test Description", nil)

				mockReportRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(reportEntity, nil)

				mockRabbitMQ.EXPECT().
					ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil)

				mockRedisRepo.EXPECT().
					Set(gomock.Any(), expectedIdempotencyKey, string(cachedResponseJSON), idempotencyTTL).
					Return(errors.New("cache write failed"))

				mockRedisRepo.EXPECT().
					Del(gomock.Any(), expectedIdempotencyKey).
					Return(nil)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
					RedisRepo:    mockRedisRepo,
				}
			},
			expectErr:      false,
			expectedResult: reportEntity,
			description:    "A cache write failure must not leave the idempotency key stuck in processing state.",
		},
		{
			name:           "Error - Redis SetNX fails",
			reportInput:    reportInput,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockReportRepo := report.NewMockRepository(ctrl)
				mockRabbitMQ := rabbitmq.NewMockProducerRepository(ctrl)

				// Redis is unavailable
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(false, constant.ErrInternalServer)

				return &UseCase{
					Logger:       log.NewNop(),
					Tracer:       noop.NewTracerProvider().Tracer("test"),
					TemplateRepo: mockTempRepo,
					ReportRepo:   mockReportRepo,
					RabbitMQRepo: mockRabbitMQ,
					RedisRepo:    mockRedisRepo,
				}
			},
			expectErr:      true,
			expectedResult: nil,
			description: "When Redis SetNX fails due to infrastructure error, the service must " +
				"return an error rather than proceeding without idempotency protection.",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			reportSvc := tt.mockSetup(ctrl)

			ctx := context.Background()

			// If an idempotency key is provided, inject it into context
			if tt.idempotencyKey != "" {
				ctx = context.WithValue(ctx, constant.IdempotencyKeyCtx, tt.idempotencyKey)
			}

			result, err := reportSvc.CreateReport(ctx, tt.reportInput)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.ID, result.ID)
				assert.Equal(t, tt.expectedResult.TemplateID, result.TemplateID)
				assert.Equal(t, tt.expectedResult.Status, result.Status)
			}
		})
	}
}

func TestUseCase_ConvertFiltersToMappedFieldsType(t *testing.T) {
	t.Parallel()

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	tests := []struct {
		name     string
		input    map[string]map[string]map[string]model.FilterCondition
		expected map[string]map[string][]string
	}{
		{
			name: "Success - Single datasource single table single field",
			input: map[string]map[string]map[string]model.FilterCondition{
				"midaz_onboarding": {
					"organization": {
						"id": {Equals: []any{"123"}},
					},
				},
			},
			expected: map[string]map[string][]string{
				"midaz_onboarding": {
					"organization": {"id"},
				},
			},
		},
		{
			name: "Success - Single datasource single table multiple fields (max 3)",
			input: map[string]map[string]map[string]model.FilterCondition{
				"midaz_onboarding": {
					"organization": {
						"id":     {Equals: []any{"123"}},
						"name":   {In: []any{"Test"}},
						"status": {Equals: []any{"active"}},
						"extra":  {Equals: []any{"ignored"}},
					},
				},
			},
			expected: map[string]map[string][]string{
				"midaz_onboarding": {
					"organization": {"id", "name", "status"},
				},
			},
		},
		{
			name: "Success - Multiple datasources and tables",
			input: map[string]map[string]map[string]model.FilterCondition{
				"datasource_one": {
					"organization": {
						"id": {Equals: []any{"123"}},
					},
					"ledger": {
						"status": {Equals: []any{"active"}},
					},
				},
				"datasource_two": {
					"analytics.transfers": {
						"amount": {GreaterThan: []any{100}},
					},
				},
			},
			expected: map[string]map[string][]string{
				"datasource_one": {
					"organization": {"id"},
					"ledger":       {"status"},
				},
				"datasource_two": {
					"analytics.transfers": {"amount"},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := uc.convertFiltersToMappedFieldsType(tt.input)

			// Verify structure matches (can't guarantee order of keys in maps)
			assert.Equal(t, len(tt.expected), len(result))
			for datasource, tables := range tt.expected {
				assert.Contains(t, result, datasource)
				assert.Equal(t, len(tables), len(result[datasource]))
				for table, fields := range tables {
					assert.Contains(t, result[datasource], table)
					assert.Equal(t, len(fields), len(result[datasource][table]))
				}
			}
		})
	}
}

func TestUseCase_HandleDuplicateRequest_RedisGetError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:test-key").
		Return("", errors.New("redis connection refused"))

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, err := uc.handleDuplicateRequest(ctx, "idempotency:test-key")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis connection refused")
	assert.Nil(t, result)
}

func TestUseCase_HandleDuplicateRequest_InvalidCachedJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:test-key").
		Return("{invalid-json", nil)

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, err := uc.handleDuplicateRequest(ctx, "idempotency:test-key")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal cached idempotency response")
	assert.Nil(t, result)
}

func TestUseCase_HandleDuplicateRequest_ReplayedSignal(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reportID := uuid.New()
	tempID := uuid.New()

	cachedReport := &report.Report{
		ID:         reportID,
		TemplateID: tempID,
		Status:     constant.ProcessingStatus,
	}

	cachedJSON, err := json.Marshal(cachedReport)
	require.NoError(t, err)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:test-key").
		Return(string(cachedJSON), nil)

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	replayed := false
	ctx := context.WithValue(context.Background(), constant.IdempotencyReplayedCtx, &replayed)

	result, err := uc.handleDuplicateRequest(ctx, "idempotency:test-key")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, reportID, result.ID)
	assert.True(t, replayed, "replayed flag should be set to true")
}

func TestUseCase_CacheIdempotencyResult_SetError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), "idempotency:test-key", gomock.Any(), constant.IdempotencyTTL).
		Return(errors.New("redis write failure"))

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	reportResult := &report.Report{
		ID:         uuid.New(),
		TemplateID: uuid.New(),
		Status:     constant.ProcessingStatus,
	}

	ctx := context.Background()

	// This method does not return an error, it only logs. We verify no panic occurs.
	uc.cacheIdempotencyResult(ctx, "idempotency:test-key", reportResult)
}

func TestUseCase_BuildIdempotencyKey_ClientProvidedKey(t *testing.T) {
	t.Parallel()

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	ctx := context.WithValue(context.Background(), constant.IdempotencyKeyCtx, "my-client-key")

	key, err := uc.buildIdempotencyKey(ctx, &model.CreateReportInput{
		TemplateID: uuid.New().String(),
	})

	require.NoError(t, err)
	assert.Equal(t, "idempotency:my-client-key", key)
}

func TestUseCase_BuildIdempotencyKey_HashBased(t *testing.T) {
	t.Parallel()

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	ctx := context.Background()
	input := &model.CreateReportInput{
		TemplateID: uuid.New().String(),
	}

	key, err := uc.buildIdempotencyKey(ctx, input)

	require.NoError(t, err)
	assert.Contains(t, key, "idempotency:")

	// Verify the key is deterministic
	key2, err2 := uc.buildIdempotencyKey(ctx, input)
	require.NoError(t, err2)
	assert.Equal(t, key, key2)
}

func TestUseCase_HandleDuplicateRequest_ProcessingResponse(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Redis Get returns "processing" — the first request is still in-flight
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:test-key-processing").
		Return("processing", nil)

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, err := uc.handleDuplicateRequest(ctx, "idempotency:test-key-processing")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "A duplicate request is currently being processed")
	assert.Nil(t, result)
}

func TestUseCase_HandleDuplicateRequest_WithoutReplayedPointerInContext(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reportID := uuid.New()
	tempID := uuid.New()

	cachedReport := &report.Report{
		ID:         reportID,
		TemplateID: tempID,
		Status:     constant.ProcessingStatus,
	}

	cachedJSON, err := json.Marshal(cachedReport)
	require.NoError(t, err)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:test-key-no-replayed").
		Return(string(cachedJSON), nil)

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	// Use plain context.Background() — no replayed pointer injected
	ctx := context.Background()
	result, err := uc.handleDuplicateRequest(ctx, "idempotency:test-key-no-replayed")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, reportID, result.ID)
	assert.Equal(t, tempID, result.TemplateID)
	assert.Equal(t, constant.ProcessingStatus, result.Status)
}

func TestUseCase_ConvertFiltersToMappedFieldsType_EmptyInput(t *testing.T) {
	t.Parallel()

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	tests := []struct {
		name  string
		input map[string]map[string]map[string]model.FilterCondition
	}{
		{
			name:  "Nil input returns empty map",
			input: nil,
		},
		{
			name:  "Empty map returns empty map",
			input: map[string]map[string]map[string]model.FilterCondition{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := uc.convertFiltersToMappedFieldsType(tt.input)

			require.NotNil(t, result)
			assert.Empty(t, result)
		})
	}
}
