// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/model"
	mongodb2 "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb"
	reportData "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"
	postgres2 "github.com/LerianStudio/midaz/v3/components/reporter/pkg/postgres"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/seaweedfs/report"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/seaweedfs/template"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

// NOTE: Kept separate from table-driven TestUseCase_GenerateReport_ErrorAndSkipPaths due to complex mock
// wiring for the full success path (template repo, postgres schema, query, report upload, status update).
// The error/skip table-driven test below covers negative and early-return paths with simpler setup.
func TestUseCase_GenerateReport_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockReportRepo := report.NewMockRepository(ctrl)
	mockPostgresRepo := postgres2.NewMockRepository(ctrl)
	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	templateID := uuid.New()
	reportID := uuid.New()

	body := GenerateReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: "txt",
		DataQueries: map[string]map[string][]string{
			"onboarding": {"organization": {"name"}},
		},
		Filters: map[string]map[string]map[string]model.FilterCondition{
			"onboarding": {
				"organization": {
					"id": {
						Equals: []any{1, 2, 3},
					},
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	mockReportDataRepo.
		EXPECT().
		FindByID(gomock.Any(), reportID).
		Return(&reportData.Report{
			ID:     reportID,
			Status: "processing",
		}, nil)

	mockTemplateRepo.
		EXPECT().
		Get(gomock.Any(), templateID.String()).
		Return([]byte("Hello {{ onboarding.organization.0.name }}"), nil)

	mockPostgresRepo.
		EXPECT().
		GetDatabaseSchema(gomock.Any(), gomock.Any()).
		Return([]postgres2.TableSchema{
			{
				TableName: "organization",
				Columns: []postgres2.ColumnInformation{
					{Name: "name", DataType: "text"},
					{Name: "id", DataType: "integer", IsPrimaryKey: true},
				},
			},
		}, nil)

	mockPostgresRepo.
		EXPECT().
		QueryWithAdvancedFilters(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(), // schemaName
			"organization",
			[]string{"name"},
			gomock.Any(),
		).
		Return([]map[string]any{{"name": "World"}}, nil)

	mockReportRepo.
		EXPECT().
		Put(gomock.Any(), gomock.Any(), "text/plain", gomock.Any(), "").
		Return(nil)

	mockReportDataRepo.
		EXPECT().
		UpdateReportStatusById(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), nil).
		Return(nil)

	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	circuitBreakerManager := pkg.NewCircuitBreakerManager(logger)

	useCase := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		TemplateSeaweedFS:     mockTemplateRepo,
		ReportSeaweedFS:       mockReportRepo,
		ReportDataRepo:        mockReportDataRepo,
		CircuitBreakerManager: circuitBreakerManager,
		ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
			"onboarding": {
				Initialized:        true,
				DatabaseType:       "postgresql",
				PostgresRepository: mockPostgresRepo,
			},
		}),
	}

	err := useCase.GenerateReport(context.Background(), bodyBytes)
	require.NoError(t, err)
}

func TestUseCase_GenerateReport_ErrorAndSkipPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mockSetup   func(ctrl *gomock.Controller, templateID, reportID uuid.UUID) *UseCase
		expectError bool
		errContains string
	}{
		{
			name: "Error - Template repo failure",
			mockSetup: func(ctrl *gomock.Controller, templateID, reportID uuid.UUID) *UseCase {
				mockTemplateRepo := template.NewMockRepository(ctrl)
				mockReportDataRepo := reportData.NewMockRepository(ctrl)

				mockReportDataRepo.
					EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&reportData.Report{ID: reportID, Status: "processing"}, nil)

				mockTemplateRepo.
					EXPECT().
					Get(gomock.Any(), templateID.String()).
					Return(nil, errors.New("failed to get file"))

				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				return &UseCase{
					Logger:              log.NewNop(),
					Tracer:              noop.NewTracerProvider().Tracer("test"),
					TemplateSeaweedFS:   mockTemplateRepo,
					ReportDataRepo:      mockReportDataRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				}
			},
			expectError: true,
			errContains: "failed to get file",
		},
		{
			name: "Success - Report already finished is skipped",
			mockSetup: func(ctrl *gomock.Controller, templateID, reportID uuid.UUID) *UseCase {
				mockReportDataRepo := reportData.NewMockRepository(ctrl)

				mockReportDataRepo.
					EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&reportData.Report{ID: reportID, Status: "Finished"}, nil)

				return &UseCase{
					Logger:              log.NewNop(),
					Tracer:              noop.NewTracerProvider().Tracer("test"),
					ReportDataRepo:      mockReportDataRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				}
			},
			expectError: false,
		},
		{
			name: "Success - Report in error state is skipped",
			mockSetup: func(ctrl *gomock.Controller, templateID, reportID uuid.UUID) *UseCase {
				mockReportDataRepo := reportData.NewMockRepository(ctrl)

				mockReportDataRepo.
					EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&reportData.Report{ID: reportID, Status: "Error"}, nil)

				return &UseCase{
					Logger:              log.NewNop(),
					Tracer:              noop.NewTracerProvider().Tracer("test"),
					ReportDataRepo:      mockReportDataRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				}
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			templateID := uuid.New()
			reportID := uuid.New()

			body := GenerateReportMessage{
				TemplateID:   templateID,
				ReportID:     reportID,
				OutputFormat: "txt",
				DataQueries:  map[string]map[string][]string{},
			}
			bodyBytes, _ := json.Marshal(body)

			useCase := tt.mockSetup(ctrl, templateID, reportID)

			err := useCase.GenerateReport(context.Background(), bodyBytes)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// NOTE: Kept separate from table-driven TestUseCase_GenerateReport due to complex crypto setup requirements.
// This test exercises the CRM plugin path with cipher initialization, hash generation, and encrypted
// field decryption, which demands significantly different UseCase wiring (crypto keys, MongoDB mocks,
// organization-scoped collections) that would bloat shared table-driven setup beyond readability.
func TestUseCase_GenerateReport_PluginCRMWithEncryptedData(t *testing.T) {
	t.Parallel()

	hashKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	encryptKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockReportRepo := report.NewMockRepository(ctrl)
	mockMongoRepo := mongodb2.NewMockRepository(ctrl)
	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	templateID := uuid.New()
	reportID := uuid.New()
	organizationID := "01956b69-9102-75b7-8860-3e75c11d231c"

	// Dados de teste - documento que sera filtrado
	testDocument := "12345678901"

	// Criar instancia de crypto para gerar hash do documento
	logger, _, _, _ := libObservability.NewTrackingFromContext(context.Background())
	crypto := &libCrypto.Crypto{
		HashSecretKey:    hashKey,
		EncryptSecretKey: encryptKey,
		Logger:           logger,
	}

	// Inicializar o cipher para criptografia
	err := crypto.InitializeCipher()
	require.NoError(t, err, "Failed to initialize cipher")

	hashedDocument := crypto.GenerateHash(&testDocument)

	templateContent := `Cliente: {{ plugin_crm.holders.0.name }}
Documento: {{ plugin_crm.holders.0.document }}
Email: {{ plugin_crm.holders.0.contact.primary_email }}
Conta Bancaria: {{ plugin_crm.holders.0.banking_details.account }}`

	nameStr := "Joao Silva"
	emailStr := "joao@example.com"
	accountStr := "12345-6"

	encryptedName, _ := crypto.Encrypt(&nameStr)
	encryptedDocument, _ := crypto.Encrypt(&testDocument)
	encryptedEmail, _ := crypto.Encrypt(&emailStr)
	encryptedAccount, _ := crypto.Encrypt(&accountStr)

	mockMongoData := []map[string]any{
		{
			"_id":      "holder-123",
			"name":     *encryptedName,
			"document": *encryptedDocument,
			"search": map[string]any{
				"document": hashedDocument,
				"name":     crypto.GenerateHash(encryptedName),
			},
			"contact": map[string]any{
				"primary_email": *encryptedEmail,
			},
			"banking_details": map[string]any{
				"account": *encryptedAccount,
			},
		},
	}

	body := GenerateReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: "html",
		DataQueries: map[string]map[string][]string{
			"plugin_crm": {
				"holders": {"name", "document", "contact.primary_email", "banking_details.account"},
			},
		},
		Filters: map[string]map[string]map[string]model.FilterCondition{
			"plugin_crm": {
				"holders": {
					"document": {
						Equals: []any{testDocument},
					},
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	mockReportDataRepo.
		EXPECT().
		FindByID(gomock.Any(), reportID).
		Return(&reportData.Report{
			ID:     reportID,
			Status: "processing",
		}, nil)

	mockTemplateRepo.
		EXPECT().
		Get(gomock.Any(), templateID.String()).
		Return([]byte(templateContent), nil)

	// ListCollectionNames discovers org-scoped collections by prefix
	mockMongoRepo.
		EXPECT().
		ListCollectionNames(gomock.Any()).
		Return([]string{"holders_" + organizationID}, nil)

	mockMongoRepo.
		EXPECT().
		QueryWithAdvancedFilters(
			gomock.Any(),
			"holders_"+organizationID,
			[]string{"name", "document", "contact.primary_email", "banking_details.account"},
			gomock.Any(),
		).
		DoAndReturn(func(ctx context.Context, collection string, fields []string, filters map[string]model.FilterCondition) ([]map[string]any, error) {
			searchDocFilter, exists := filters["search.document"]
			assert.True(t, exists, "Expected search.document filter to be present")
			if exists && len(searchDocFilter.Equals) > 0 {
				assert.Equal(t, hashedDocument, searchDocFilter.Equals[0], "Expected hashed document")
			}
			return mockMongoData, nil
		})

	mockReportRepo.
		EXPECT().
		Put(gomock.Any(), gomock.Any(), "text/html", gomock.Any(), "").
		DoAndReturn(func(ctx context.Context, objectName, contentType string, data []byte, ttl string) error {
			content := string(data)
			assert.Contains(t, content, "Joao Silva", "Expected decrypted name in rendered content")
			assert.Contains(t, content, testDocument, "Expected decrypted document in rendered content")
			assert.Contains(t, content, "joao@example.com", "Expected decrypted email in rendered content")
			assert.Contains(t, content, "12345-6", "Expected decrypted account in rendered content")
			return nil
		})

	mockReportDataRepo.
		EXPECT().
		UpdateReportStatusById(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), nil).
		Return(nil)

	circuitBreakerManager := pkg.NewCircuitBreakerManager(logger)

	useCase := &UseCase{
		Logger:                          log.NewNop(),
		Tracer:                          noop.NewTracerProvider().Tracer("test"),
		TemplateSeaweedFS:               mockTemplateRepo,
		ReportSeaweedFS:                 mockReportRepo,
		ReportDataRepo:                  mockReportDataRepo,
		CircuitBreakerManager:           circuitBreakerManager,
		CryptoHashSecretKeyPluginCRM:    hashKey,
		CryptoEncryptSecretKeyPluginCRM: encryptKey,
		ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
			"plugin_crm": {
				Initialized:         true,
				DatabaseType:        "mongodb",
				MongoDBRepository:   mockMongoRepo,
				MidazOrganizationID: organizationID,
			},
		}),
	}

	err = useCase.GenerateReport(context.Background(), bodyBytes)
	require.NoError(t, err)
}

func TestUseCase_ParseMessage(t *testing.T) {
	t.Parallel()

	templateID := uuid.New()
	reportID := uuid.New()

	validBody := GenerateReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: "pdf",
		DataQueries:  map[string]map[string][]string{},
	}
	validBodyBytes, _ := json.Marshal(validBody)

	tests := []struct {
		name                string
		input               []byte
		needsReportDataMock bool
		expectError         bool
		expectedTemplateID  uuid.UUID
		expectedReportID    uuid.UUID
	}{
		{
			name:                "Error - Invalid JSON",
			input:               []byte("invalid json"),
			needsReportDataMock: false,
			expectError:         true,
		},
		{
			name:                "Success - Valid JSON",
			input:               validBodyBytes,
			needsReportDataMock: false,
			expectError:         false,
			expectedTemplateID:  templateID,
			expectedReportID:    reportID,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
			_, span := tracer.Start(context.Background(), "test")

			useCase := &UseCase{
				Logger: log.NewNop(),
				Tracer: noop.NewTracerProvider().Tracer("test"),
			}

			if tt.needsReportDataMock {
				mockReportDataRepo := reportData.NewMockRepository(ctrl)
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				useCase.ReportDataRepo = mockReportDataRepo
			}

			message, err := useCase.parseMessage(context.Background(), tt.input, &span)
			if tt.expectError {
				require.Error(t, err, "expected error for invalid input")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedTemplateID, message.TemplateID)
				assert.Equal(t, tt.expectedReportID, message.ReportID)
			}
		})
	}
}

func TestUseCase_UpdateReportWithErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reportID    uuid.UUID
		reportErr   error
		mockSetup   func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID)
		expectError bool
		errContains string
	}{
		{
			name:      "Success - Update report with error",
			reportID:  uuid.New(),
			reportErr: errors.New("test error message"),
			mockSetup: func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, _ uuid.UUID, _ time.Time, metadata map[string]any) error {
						assert.Equal(t, "Report generation failed", metadata["error"])
						assert.Equal(t, "report_generation_failed", metadata["error_code"])
						assert.Equal(t, "test error message", metadata["error_detail"])
						return nil
					})
			},
			expectError: false,
		},
		{
			name:      "Error - Failed to update report",
			reportID:  uuid.New(),
			reportErr: errors.New("test error message"),
			mockSetup: func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
					Return(errors.New("database error"))
			},
			expectError: true,
			errContains: "database error",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockReportDataRepo := reportData.NewMockRepository(ctrl)
			tt.mockSetup(mockReportDataRepo, tt.reportID)

			useCase := &UseCase{
				Logger:         log.NewNop(),
				Tracer:         noop.NewTracerProvider().Tracer("test"),
				ReportDataRepo: mockReportDataRepo,
			}

			err := useCase.updateReportWithErrors(context.Background(), tt.reportID, tt.reportErr)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUseCase_UpdateReportWithErrors_SanitizesTimeoutMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reportID := uuid.New()
	mockReportDataRepo := reportData.NewMockRepository(ctrl)
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, _ uuid.UUID, _ time.Time, metadata map[string]any) error {
			assert.Equal(t, "Report generation timed out", metadata["error"])
			assert.Equal(t, "report_generation_timeout", metadata["error_code"])
			assert.Equal(t, context.DeadlineExceeded.Error(), metadata["error_detail"])
			return nil
		})

	useCase := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), ReportDataRepo: mockReportDataRepo}
	err := useCase.updateReportWithErrors(context.Background(), reportID, context.DeadlineExceeded)
	require.NoError(t, err)
}

func TestUseCase_MarkReportAsFinished(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reportID    uuid.UUID
		mockSetup   func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID)
		expectError bool
		errContains string
	}{
		{
			name:     "Success - Mark report as finished",
			reportID: uuid.New(),
			mockSetup: func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "Finished", reportID, gomock.Any(), nil).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "Error - Failed to mark as finished",
			reportID: uuid.New(),
			mockSetup: func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "Finished", reportID, gomock.Any(), nil).
					Return(errors.New("database error"))
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError: true,
			errContains: "database error",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockReportDataRepo := reportData.NewMockRepository(ctrl)
			tt.mockSetup(mockReportDataRepo, tt.reportID)

			_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
			_, span := tracer.Start(context.Background(), "test")

			useCase := &UseCase{
				Logger:         log.NewNop(),
				Tracer:         noop.NewTracerProvider().Tracer("test"),
				ReportDataRepo: mockReportDataRepo,
			}

			err := useCase.markReportAsFinished(context.Background(), tt.reportID, &span)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUseCase_HandleErrorWithUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reportID    uuid.UUID
		errorMsg    string
		inputErr    error
		mockSetup   func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID)
		expectError bool
		errContains string
	}{
		{
			name:     "Success - Log error and update report",
			reportID: uuid.New(),
			errorMsg: "Test error message",
			inputErr: errors.New("original error"),
			mockSetup: func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError: true, // Returns the original error
			errContains: "original error",
		},
		{
			name:     "Error - Failed to update report status",
			reportID: uuid.New(),
			errorMsg: "Test error message",
			inputErr: errors.New("original error"),
			mockSetup: func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
					Return(errors.New("update failed"))
			},
			expectError: true,
			errContains: "update failed",
		},
		{
			name:     "Success - Business error uses HandleSpanBusinessErrorEvent",
			reportID: uuid.New(),
			errorMsg: "Validation failed",
			inputErr: pkg.ValidationError{Code: "VAL001", Title: "Validation", Message: "invalid input"},
			mockSetup: func(mockReportDataRepo *reportData.MockRepository, reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError: true,
			errContains: "invalid input",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockReportDataRepo := reportData.NewMockRepository(ctrl)
			tt.mockSetup(mockReportDataRepo, tt.reportID)

			_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
			_, span := tracer.Start(context.Background(), "test")

			useCase := &UseCase{
				Logger:         log.NewNop(),
				Tracer:         noop.NewTracerProvider().Tracer("test"),
				ReportDataRepo: mockReportDataRepo,
			}

			err := useCase.handleErrorWithUpdate(context.Background(), tt.reportID, &span, tt.errorMsg, tt.inputErr)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

func TestUseCase_ParseMessage_ZeroUUIDSkipsUpdate(t *testing.T) {
	t.Parallel()

	// When parseMessage fails on completely invalid JSON, ReportID is uuid.Nil.
	// The guard should skip updateReportWithErrors and return the unmarshal error directly.
	useCase := &UseCase{
		Logger: log.NewNop(),
		Tracer: noop.NewTracerProvider().Tracer("test"),
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
	_, span := tracer.Start(context.Background(), "test")

	_, err := useCase.parseMessage(context.Background(), []byte("invalid json"), &span)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid character")
}

func TestUseCase_ParseMessage_UpdateReportAlsoFails(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	// When parseMessage fails but ReportID was partially parsed (non-nil),
	// it calls updateReportWithErrors. If that also fails, it should return
	// the updateReportWithErrors error.
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("database connection lost"))

	useCase := &UseCase{
		Logger:         log.NewNop(),
		Tracer:         noop.NewTracerProvider().Tracer("test"),
		ReportDataRepo: mockReportDataRepo,
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
	_, span := tracer.Start(context.Background(), "test")

	// Partial JSON with valid reportId but invalid structure for other fields
	partialJSON := []byte(`{"reportId":"d11e8b99-1645-4bfe-b64a-db372240c80e","mappedFields":"not-a-map"}`)
	_, err := useCase.parseMessage(context.Background(), partialJSON, &span)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection lost")
}

func TestUseCase_MarkReportAsFinished_NestedErrorWhenUpdateAlsoFails(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportDataRepo := reportData.NewMockRepository(ctrl)
	reportID := uuid.New()

	// First call: UpdateReportStatusById for "Finished" fails
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "Finished", reportID, gomock.Any(), nil).
		Return(errors.New("first update failed"))

	// Second call: updateReportWithErrors also fails
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
		Return(errors.New("second update also failed"))

	_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())
	_, span := tracer.Start(context.Background(), "test")

	useCase := &UseCase{
		Logger:         log.NewNop(),
		Tracer:         noop.NewTracerProvider().Tracer("test"),
		ReportDataRepo: mockReportDataRepo,
	}

	err := useCase.markReportAsFinished(context.Background(), reportID, &span)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "second update also failed")
}
