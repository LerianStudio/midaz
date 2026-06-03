// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb/template"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/postgres"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/redis"
	templateSeaweedFS "github.com/LerianStudio/midaz/v3/pkg/reporter/seaweedfs/template"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUseCase_CreateTemplate(t *testing.T) {
	t.Parallel()

	// Register datasource IDs additively (no Reset) AFTER t.Parallel(). This ensures
	// registration occurs after all non-parallel tests (which may call Reset) have
	// completed, preventing races. RegisterDataSourceIDsForTesting is lock-protected
	// and additive; subtests only READ the global state via IsValidDataSourceID.
	pkg.RegisterDataSourceIDsForTesting([]string{"midaz_organization", "midaz_onboarding"})

	templateTest := `
		<?xml version="1.0" encoding="UTF-8"?>
		{% for org in midaz_organization.organization %}
		<Organizacao>
			<CNPJ>{{ org.legal_document }}</CNPJ>
			<NomeLegal>{{ org.legal_name }}</NomeLegal>
			<NomeFantasia>{{ org.doing_business_as }}</NomeFantasia>
			<Endereco>{{ org.address.line1 }}, {{ org.address.city }} - {{ org.address.state }}</Endereco>
		</Organizacao>
		{% endfor %}

		{% for l in midaz_onboarding.ledger %}
		<Ledger>
			<Nome>{{ l.name }}</Nome>
			<Status>{{ l.status }}</Status>
		</Ledger>
		{% endfor %}
	`
	templateTestFileHeader, _ := createFileHeaderFromString(templateTest, "teste_template_XML.tpl")

	tests := []struct {
		name           string
		templateFile   string
		outFormat      string
		description    string
		fileHeader     *multipart.FileHeader
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		errContains    string
		expectedResult bool
	}{
		{
			name:         "Success - Create a template",
			templateFile: templateTest,
			outFormat:    "xml",
			description:  "Template Financeiro",
			fileHeader:   templateTestFileHeader,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)

				tempId := uuid.New()
				timestamp := time.Now().Unix()

				mockTempRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						ID:           tempId,
						OutputFormat: "xml",
						Description:  "Template Financeiro",
						FileName:     fmt.Sprintf("%s_%d.tpl", tempId.String(), timestamp),
						CreatedAt:    time.Time{},
					}, nil)
				mockTemplateStorage.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType: "mongodb", MongoDBName: "organization", Initialized: true,
						},
						"midaz_onboarding": {
							DatabaseType: "postgresql", MongoDBName: "ledger", Initialized: true,
						},
					}),
				}
			},
			expectErr:      false,
			expectedResult: true,
		},
		{
			name:         "Error - Create a template",
			templateFile: templateTest,
			outFormat:    "xml",
			description:  "Template Financeiro",
			fileHeader:   templateTestFileHeader,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)

				mockTempRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInternalServer)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType: "mongodb", MongoDBName: "organization", Initialized: true,
						},
						"midaz_onboarding": {
							DatabaseType: "postgresql", MongoDBName: "ledger", Initialized: true,
						},
					}),
				}
			},
			expectErr:   true,
			errContains: constant.ErrInternalServer.Error(),
		},
		{
			name:         "Error - Create a template with <script> tag",
			templateFile: `<html><script>alert('x')</script></html>`,
			outFormat:    "html",
			description:  "Malicious Template",
			fileHeader:   templateTestFileHeader,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				return &UseCase{
					Logger:              log.NewNop(),
					Tracer:              noop.NewTracerProvider().Tracer("test"),
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				}
			},
			expectErr:   true,
			errContains: constant.ErrScriptTagDetected.Error(),
		},
		{
			name:         "Error - ReadMultipartFile failure",
			templateFile: templateTest,
			outFormat:    "xml",
			description:  "Template Financeiro",
			fileHeader:   &multipart.FileHeader{},
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)

				tempId := uuid.New()
				timestamp := time.Now().Unix()

				mockTempRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						ID:           tempId,
						OutputFormat: "xml",
						Description:  "Template Financeiro",
						FileName:     fmt.Sprintf("%s_%d.tpl", tempId.String(), timestamp),
						CreatedAt:    time.Time{},
					}, nil)
				mockTempRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), true).
					Return(nil)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType: "mongodb", MongoDBName: "organization", Initialized: true,
						},
						"midaz_onboarding": {
							DatabaseType: "postgresql", MongoDBName: "ledger", Initialized: true,
						},
					}),
				}
			},
			expectErr:   true,
			errContains: "open",
		},
		{
			name:         "Error - Storage Put failure with successful rollback",
			templateFile: templateTest,
			outFormat:    "xml",
			description:  "Template Financeiro",
			fileHeader:   templateTestFileHeader,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)

				tempId := uuid.New()
				timestamp := time.Now().Unix()

				mockTempRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						ID:           tempId,
						OutputFormat: "xml",
						Description:  "Template Financeiro",
						FileName:     fmt.Sprintf("%s_%d.tpl", tempId.String(), timestamp),
						CreatedAt:    time.Time{},
					}, nil)
				mockTemplateStorage.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("storage unavailable"))
				mockTempRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), true).
					Return(nil)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType: "mongodb", MongoDBName: "organization", Initialized: true,
						},
						"midaz_onboarding": {
							DatabaseType: "postgresql", MongoDBName: "ledger", Initialized: true,
						},
					}),
				}
			},
			expectErr:   true,
			errContains: "storage unavailable",
		},
		{
			name:         "Error - Storage Put failure with rollback failure",
			templateFile: templateTest,
			outFormat:    "xml",
			description:  "Template Financeiro",
			fileHeader:   templateTestFileHeader,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)

				tempId := uuid.New()
				timestamp := time.Now().Unix()

				mockTempRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						ID:           tempId,
						OutputFormat: "xml",
						Description:  "Template Financeiro",
						FileName:     fmt.Sprintf("%s_%d.tpl", tempId.String(), timestamp),
						CreatedAt:    time.Time{},
					}, nil)
				mockTemplateStorage.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("storage unavailable"))
				mockTempRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), true).
					Return(errors.New("delete failed"))

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType: "mongodb", MongoDBName: "organization", Initialized: true,
						},
						"midaz_onboarding": {
							DatabaseType: "postgresql", MongoDBName: "ledger", Initialized: true,
						},
					}),
				}
			},
			expectErr:   true,
			errContains: "storage unavailable",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			tempSvc := tt.mockSetup(ctrl)

			ctx := context.Background()
			result, _, err := tempSvc.CreateTemplate(ctx, tt.templateFile, tt.outFormat, tt.description, tt.fileHeader)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

func TestUseCase_CreateTemplateWithPluginCRM(t *testing.T) {
	t.Parallel()

	// Register datasource IDs additively (no Reset) AFTER t.Parallel(). This ensures
	// registration occurs after all non-parallel tests (which may call Reset) have
	// completed, preventing races. RegisterDataSourceIDsForTesting is lock-protected
	// and additive; subtests only READ the global state via IsValidDataSourceID.
	pkg.RegisterDataSourceIDsForTesting([]string{"plugin_crm"})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTempRepo := template.NewMockRepository(ctrl)
	mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)
	tempId := uuid.New()

	externalDataSourcesMap := map[string]pkg.DataSource{}
	externalDataSourcesMap["plugin_crm"] = pkg.DataSource{
		DatabaseType:        "mongodb",
		MongoURI:            "",
		MongoDBName:         "plugin_crm",
		Connection:          nil,
		Initialized:         true,
		MidazOrganizationID: "org-123-abc",
	}

	tempSvc := &UseCase{
		Logger:              log.NewNop(),
		Tracer:              noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:        mockTempRepo,
		TemplateSeaweedFS:   mockTemplateStorage,
		ExternalDataSources: pkg.NewSafeDataSources(externalDataSourcesMap),
	}

	templateEntity := &template.Template{
		ID:           tempId,
		OutputFormat: "xml",
		Description:  "CRM Template",
		FileName:     fmt.Sprintf("%s.tpl", tempId.String()),
		CreatedAt:    time.Time{},
	}

	templateCRM := `
		<?xml version="1.0" encoding="UTF-8"?>
		{% for h in plugin_crm.holder %}
		<Holder>
			<Name>{{ h.name }}</Name>
			<Document>{{ h.document }}</Document>
		</Holder>
		{% endfor %}
	`
	templateCRMFileHeader, _ := createFileHeaderFromString(templateCRM, "crm_template.tpl")

	t.Run("Success - Template with plugin_crm datasource", func(t *testing.T) {
		mockTempRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(templateEntity, nil)

		mockTemplateStorage.EXPECT().
			Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		ctx := context.Background()
		result, _, err := tempSvc.CreateTemplate(ctx, templateCRM, "xml", "CRM Template", templateCRMFileHeader)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, tempId, result.ID)
	})
}

// hashTemplateIdempotencyInput computes a SHA256 hash of the JSON-serialized template
// idempotency input. This is a test helper that mirrors the hashing logic in
// buildTemplateIdempotencyKey.
func hashTemplateIdempotencyInput(t *testing.T, templateFile, outFormat, description string) string {
	t.Helper()

	input := templateIdempotencyInput{
		TemplateFile: templateFile,
		OutputFormat: outFormat,
		Description:  description,
	}

	data, err := json.Marshal(input)
	require.NoError(t, err, "failed to marshal template input for hash computation")

	return commons.HashSHA256(string(data))
}

func TestUseCase_CreateTemplate_Idempotency(t *testing.T) {
	t.Parallel()

	// Register datasource IDs additively (no Reset) AFTER t.Parallel(). This ensures
	// registration occurs after all non-parallel tests (which may call Reset) have
	// completed, preventing races. RegisterDataSourceIDsForTesting is lock-protected
	// and additive; subtests only READ the global state via IsValidDataSourceID.
	pkg.RegisterDataSourceIDsForTesting([]string{"midaz_organization", "midaz_onboarding"})

	templateTest := `
		<?xml version="1.0" encoding="UTF-8"?>
		{% for org in midaz_organization.organization %}
		<Organizacao>
			<CNPJ>{{ org.legal_document }}</CNPJ>
			<NomeLegal>{{ org.legal_name }}</NomeLegal>
			<NomeFantasia>{{ org.doing_business_as }}</NomeFantasia>
			<Endereco>{{ org.address.line1 }}, {{ org.address.city }} - {{ org.address.state }}</Endereco>
		</Organizacao>
		{% endfor %}

		{% for l in midaz_onboarding.ledger %}
		<Ledger>
			<Nome>{{ l.name }}</Nome>
			<Status>{{ l.status }}</Status>
		</Ledger>
		{% endfor %}
	`
	templateTestFileHeader, _ := createFileHeaderFromString(templateTest, "teste_template_XML.tpl")

	outFormat := "xml"
	description := "Template Financeiro"

	templateID := uuid.New()

	templateEntity := &template.Template{
		ID:           templateID,
		OutputFormat: outFormat,
		Description:  description,
		FileName:     fmt.Sprintf("%s.tpl", templateID.String()),
		CreatedAt:    time.Time{},
	}

	// Pre-compute the expected idempotency key based on the request body hash
	expectedHash := hashTemplateIdempotencyInput(t, templateTest, outFormat, description)
	expectedIdempotencyKey := "idempotency:template:" + expectedHash

	// Pre-compute the expected cached response JSON
	cachedResponseJSON, err := json.Marshal(templateEntity)
	require.NoError(t, err, "failed to marshal template entity for cached response")

	idempotencyTTL := constant.IdempotencyTTL

	tests := []struct {
		name           string
		templateFile   string
		outFormat      string
		description    string
		idempotencyKey string
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		errContains    string
		expectedResult *template.Template
		testDesc       string
	}{
		{
			name:           "Success - First call creates template with idempotency lock",
			templateFile:   templateTest,
			outFormat:      outFormat,
			description:    description,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)
				mockDataSourceMongo := mongodb.NewMockRepository(ctrl)
				mockDataSourcePostgres := postgres.NewMockRepository(ctrl)

				// Expect SetNX to be called with the hash-based key BEFORE template creation
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockTempRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(templateEntity, nil)

				mockTemplateStorage.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				// After successful creation, expect the result to be cached in Redis
				mockRedisRepo.EXPECT().
					Set(gomock.Any(), expectedIdempotencyKey, string(cachedResponseJSON), idempotencyTTL).
					Return(nil)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					RedisRepo:         mockRedisRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType:       "mongodb",
							MongoDBRepository:  mockDataSourceMongo,
							PostgresRepository: mockDataSourcePostgres,
							MongoDBName:        "organization",
							Initialized:        true,
						},
						"midaz_onboarding": {
							DatabaseType:       "postgresql",
							PostgresRepository: mockDataSourcePostgres,
							MongoDBRepository:  mockDataSourceMongo,
							MongoDBName:        "ledger",
							Initialized:        true,
							DatabaseConfig:     &postgres.Connection{Connected: true},
						},
					}),
				}
			},
			expectErr:      false,
			expectedResult: templateEntity,
			testDesc: "The first call must acquire the SetNX lock, create the template, " +
				"upload to storage, and cache the response for future duplicates.",
		},
		{
			name:           "Success - Duplicate request returns cached template (no new template created)",
			templateFile:   templateTest,
			outFormat:      outFormat,
			description:    description,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)
				mockDataSourceMongo := mongodb.NewMockRepository(ctrl)
				mockDataSourcePostgres := postgres.NewMockRepository(ctrl)

				// SetNX returns false: key already exists (duplicate request)
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(false, nil)

				// Expect Get to retrieve the cached response
				mockRedisRepo.EXPECT().
					Get(gomock.Any(), expectedIdempotencyKey).
					Return(string(cachedResponseJSON), nil)

				// NO calls to TemplateRepo, storage, or datasource should happen
				// (gomock will fail if unexpected calls are made)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					RedisRepo:         mockRedisRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType:       "mongodb",
							MongoDBRepository:  mockDataSourceMongo,
							PostgresRepository: mockDataSourcePostgres,
							MongoDBName:        "organization",
							Initialized:        true,
						},
						"midaz_onboarding": {
							DatabaseType:       "postgresql",
							PostgresRepository: mockDataSourcePostgres,
							MongoDBRepository:  mockDataSourceMongo,
							MongoDBName:        "ledger",
							Initialized:        true,
							DatabaseConfig:     &postgres.Connection{Connected: true},
						},
					}),
				}
			},
			expectErr:      false,
			expectedResult: templateEntity,
			testDesc: "When SetNX returns false, it means a duplicate request. " +
				"The service must return the cached response without creating a new template.",
		},
		{
			name:         "Success - Different file content creates a different template",
			templateFile: `<html>{{ midaz_organization.organization.legal_name }}</html>`,
			outFormat:    "html",
			description:  "Different Template",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)
				mockDataSourceMongo := mongodb.NewMockRepository(ctrl)
				mockDataSourcePostgres := postgres.NewMockRepository(ctrl)

				// Different body produces a different hash, so SetNX succeeds (new key)
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), gomock.Not(gomock.Eq(expectedIdempotencyKey)), gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockTempRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(templateEntity, nil)

				mockTemplateStorage.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockRedisRepo.EXPECT().
					Set(gomock.Any(), gomock.Not(gomock.Eq(expectedIdempotencyKey)), gomock.Any(), idempotencyTTL).
					Return(nil)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					RedisRepo:         mockRedisRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType:       "mongodb",
							MongoDBRepository:  mockDataSourceMongo,
							PostgresRepository: mockDataSourcePostgres,
							MongoDBName:        "organization",
							Initialized:        true,
						},
						"midaz_onboarding": {
							DatabaseType:       "postgresql",
							PostgresRepository: mockDataSourcePostgres,
							MongoDBRepository:  mockDataSourceMongo,
							MongoDBName:        "ledger",
							Initialized:        true,
							DatabaseConfig:     &postgres.Connection{Connected: true},
						},
					}),
				}
			},
			expectErr:      false,
			expectedResult: templateEntity,
			testDesc: "A request with different template content produces a different hash, " +
				"so SetNX succeeds and a new template is created normally.",
		},
		{
			name:           "Success - Client-provided Idempotency-Key header is used instead of hash",
			templateFile:   templateTest,
			outFormat:      outFormat,
			description:    description,
			idempotencyKey: "client-provided-unique-key-12345",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)
				mockDataSourceMongo := mongodb.NewMockRepository(ctrl)
				mockDataSourcePostgres := postgres.NewMockRepository(ctrl)

				// When client provides an explicit key, it is used instead of hashing the body
				clientIdempotencyKey := "idempotency:template:client-provided-unique-key-12345"

				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), clientIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockTempRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(templateEntity, nil)

				mockTemplateStorage.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockRedisRepo.EXPECT().
					Set(gomock.Any(), clientIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(nil)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					RedisRepo:         mockRedisRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType:       "mongodb",
							MongoDBRepository:  mockDataSourceMongo,
							PostgresRepository: mockDataSourcePostgres,
							MongoDBName:        "organization",
							Initialized:        true,
						},
						"midaz_onboarding": {
							DatabaseType:       "postgresql",
							PostgresRepository: mockDataSourcePostgres,
							MongoDBRepository:  mockDataSourceMongo,
							MongoDBName:        "ledger",
							Initialized:        true,
							DatabaseConfig:     &postgres.Connection{Connected: true},
						},
					}),
				}
			},
			expectErr:      false,
			expectedResult: templateEntity,
			testDesc: "When the client provides an Idempotency-Key header, the service must " +
				"use that key instead of computing a hash from the request body.",
		},
		{
			name:           "Error - Duplicate in-flight request (SetNX false, value still processing)",
			templateFile:   templateTest,
			outFormat:      outFormat,
			description:    description,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)
				mockDataSourceMongo := mongodb.NewMockRepository(ctrl)
				mockDataSourcePostgres := postgres.NewMockRepository(ctrl)

				// SetNX returns false: key already exists
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(false, nil)

				// Get returns "processing": first request is still in-flight
				mockRedisRepo.EXPECT().
					Get(gomock.Any(), expectedIdempotencyKey).
					Return("processing", nil)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					RedisRepo:         mockRedisRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType:       "mongodb",
							MongoDBRepository:  mockDataSourceMongo,
							PostgresRepository: mockDataSourcePostgres,
							MongoDBName:        "organization",
							Initialized:        true,
						},
						"midaz_onboarding": {
							DatabaseType:       "postgresql",
							PostgresRepository: mockDataSourcePostgres,
							MongoDBRepository:  mockDataSourceMongo,
							MongoDBName:        "ledger",
							Initialized:        true,
							DatabaseConfig:     &postgres.Connection{Connected: true},
						},
					}),
				}
			},
			expectErr:      true,
			errContains:    "A duplicate request is currently being processed",
			expectedResult: nil,
			testDesc: "When SetNX returns false and Get returns 'processing', it means the first request " +
				"is still in-flight. The service must return an error indicating a duplicate in-flight request.",
		},
		{
			name:           "Error - Validation failure releases idempotency lock",
			templateFile:   `<script>alert('xss')</script>`,
			outFormat:      outFormat,
			description:    description,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)
				mockDataSourceMongo := mongodb.NewMockRepository(ctrl)
				mockDataSourcePostgres := postgres.NewMockRepository(ctrl)

				scriptHash := hashTemplateIdempotencyInput(t, `<script>alert('xss')</script>`, outFormat, description)
				scriptKey := "idempotency:template:" + scriptHash

				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), scriptKey, gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockRedisRepo.EXPECT().
					Del(gomock.Any(), scriptKey).
					Return(nil)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					RedisRepo:         mockRedisRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType:       "mongodb",
							MongoDBRepository:  mockDataSourceMongo,
							PostgresRepository: mockDataSourcePostgres,
							MongoDBName:        "organization",
							Initialized:        true,
						},
					}),
				}
			},
			expectErr:      true,
			errContains:    constant.ErrScriptTagDetected.Error(),
			expectedResult: nil,
			testDesc:       "Failures after acquiring the idempotency lock must release the key so retries are not blocked.",
		},
		{
			name:           "Success - Cache write failure releases idempotency lock",
			templateFile:   templateTest,
			outFormat:      outFormat,
			description:    description,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)
				mockDataSourceMongo := mongodb.NewMockRepository(ctrl)
				mockDataSourcePostgres := postgres.NewMockRepository(ctrl)

				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(true, nil)

				mockTempRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(templateEntity, nil)

				mockTemplateStorage.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockRedisRepo.EXPECT().
					Set(gomock.Any(), expectedIdempotencyKey, string(cachedResponseJSON), idempotencyTTL).
					Return(errors.New("cache write failed"))

				mockRedisRepo.EXPECT().
					Del(gomock.Any(), expectedIdempotencyKey).
					Return(nil)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					RedisRepo:         mockRedisRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType:       "mongodb",
							MongoDBRepository:  mockDataSourceMongo,
							PostgresRepository: mockDataSourcePostgres,
							MongoDBName:        "organization",
							Initialized:        true,
						},
						"midaz_onboarding": {
							DatabaseType:       "postgresql",
							PostgresRepository: mockDataSourcePostgres,
							MongoDBRepository:  mockDataSourceMongo,
							MongoDBName:        "ledger",
							Initialized:        true,
							DatabaseConfig:     &postgres.Connection{Connected: true},
						},
					}),
				}
			},
			expectErr:      false,
			expectedResult: templateEntity,
			testDesc:       "A cache write failure must not leave the idempotency key stuck in processing state.",
		},
		{
			name:           "Error - Redis SetNX fails",
			templateFile:   templateTest,
			outFormat:      outFormat,
			description:    description,
			idempotencyKey: "",
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockRedisRepo := redis.NewMockRedisRepository(ctrl)
				mockTempRepo := template.NewMockRepository(ctrl)
				mockTemplateStorage := templateSeaweedFS.NewMockRepository(ctrl)
				mockDataSourceMongo := mongodb.NewMockRepository(ctrl)
				mockDataSourcePostgres := postgres.NewMockRepository(ctrl)

				// Redis is unavailable
				mockRedisRepo.EXPECT().
					SetNX(gomock.Any(), expectedIdempotencyKey, gomock.Any(), idempotencyTTL).
					Return(false, constant.ErrInternalServer)

				return &UseCase{
					Logger:            log.NewNop(),
					Tracer:            noop.NewTracerProvider().Tracer("test"),
					TemplateRepo:      mockTempRepo,
					TemplateSeaweedFS: mockTemplateStorage,
					RedisRepo:         mockRedisRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{
						"midaz_organization": {
							DatabaseType:       "mongodb",
							MongoDBRepository:  mockDataSourceMongo,
							PostgresRepository: mockDataSourcePostgres,
							MongoDBName:        "organization",
							Initialized:        true,
						},
						"midaz_onboarding": {
							DatabaseType:       "postgresql",
							PostgresRepository: mockDataSourcePostgres,
							MongoDBRepository:  mockDataSourceMongo,
							MongoDBName:        "ledger",
							Initialized:        true,
							DatabaseConfig:     &postgres.Connection{Connected: true},
						},
					}),
				}
			},
			expectErr:      true,
			errContains:    constant.ErrInternalServer.Error(),
			expectedResult: nil,
			testDesc: "When Redis SetNX fails due to infrastructure error, the service must " +
				"return an error rather than proceeding without idempotency protection.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			tempSvc := tt.mockSetup(ctrl)

			ctx := context.Background()

			// If an idempotency key is provided, inject it into context
			if tt.idempotencyKey != "" {
				ctx = context.WithValue(ctx, constant.IdempotencyKeyCtx, tt.idempotencyKey)
			}

			result, _, err := tempSvc.CreateTemplate(ctx, tt.templateFile, tt.outFormat, tt.description, templateTestFileHeader)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.ID, result.ID)
				assert.Equal(t, tt.expectedResult.OutputFormat, result.OutputFormat)
				assert.Equal(t, tt.expectedResult.Description, result.Description)
			}
		})
	}
}

func TestUseCase_HandleDuplicateTemplateRequest_RedisGetError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:template:test-key").
		Return("", errors.New("redis connection refused"))

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, err := uc.handleDuplicateTemplateRequest(ctx, "idempotency:template:test-key")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis connection refused")
	assert.Nil(t, result)
}

func TestUseCase_HandleDuplicateTemplateRequest_InvalidCachedJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:template:test-key").
		Return("{invalid-json", nil)

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, err := uc.handleDuplicateTemplateRequest(ctx, "idempotency:template:test-key")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal cached template idempotency response")
	assert.Nil(t, result)
}

func TestUseCase_HandleDuplicateTemplateRequest_ReplayedSignal(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateID := uuid.New()
	cachedTemplate := &template.Template{
		ID:           templateID,
		OutputFormat: "xml",
		Description:  "Cached template",
	}

	cachedJSON, err := json.Marshal(cachedTemplate)
	require.NoError(t, err)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:template:test-key").
		Return(string(cachedJSON), nil)

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	replayed := false
	ctx := context.WithValue(context.Background(), constant.IdempotencyReplayedCtx, &replayed)

	result, err := uc.handleDuplicateTemplateRequest(ctx, "idempotency:template:test-key")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, templateID, result.ID)
	assert.True(t, replayed, "replayed flag should be set to true")
}

func TestUseCase_CacheTemplateIdempotencyResult_SetError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Set(gomock.Any(), "idempotency:template:test-key", gomock.Any(), constant.IdempotencyTTL).
		Return(errors.New("redis write failure"))

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	templateResult := &template.Template{
		ID:           uuid.New(),
		OutputFormat: "xml",
		Description:  "Test template",
	}

	ctx := context.Background()

	// This method does not return an error, it only logs. We verify no panic occurs.
	uc.cacheTemplateIdempotencyResult(ctx, "idempotency:template:test-key", templateResult)
}

func TestUseCase_BuildTemplateIdempotencyKey_ClientProvidedKey(t *testing.T) {
	t.Parallel()

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	ctx := context.WithValue(context.Background(), constant.IdempotencyKeyCtx, "my-client-key")

	key, err := uc.buildTemplateIdempotencyKey(ctx, "file-content", "xml", "desc")

	require.NoError(t, err)
	assert.Equal(t, "idempotency:template:my-client-key", key)
}

func TestUseCase_BuildTemplateIdempotencyKey_HashBased(t *testing.T) {
	t.Parallel()

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	ctx := context.Background()

	key, err := uc.buildTemplateIdempotencyKey(ctx, "file-content", "xml", "desc")

	require.NoError(t, err)
	assert.Contains(t, key, "idempotency:template:")
	// Verify the key is deterministic
	key2, err2 := uc.buildTemplateIdempotencyKey(ctx, "file-content", "xml", "desc")
	require.NoError(t, err2)
	assert.Equal(t, key, key2)
}

func TestUseCase_HandleDuplicateTemplateRequest_EmptyStringResponse(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Redis Get returns empty string "" — the first request is still in-flight
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:template:test-key").
		Return("", nil)

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	ctx := context.Background()
	result, err := uc.handleDuplicateTemplateRequest(ctx, "idempotency:template:test-key")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "A duplicate request is currently being processed")
	assert.Nil(t, result)
}

func TestUseCase_HandleDuplicateTemplateRequest_WithoutReplayedPointerInContext(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateID := uuid.New()
	cachedTemplate := &template.Template{
		ID:           templateID,
		OutputFormat: "xml",
		Description:  "Cached template without replayed pointer",
	}

	cachedJSON, err := json.Marshal(cachedTemplate)
	require.NoError(t, err)

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), "idempotency:template:test-key-no-replayed").
		Return(string(cachedJSON), nil)

	uc := &UseCase{
		Logger:    log.NewNop(),
		Tracer:    noop.NewTracerProvider().Tracer("test"),
		RedisRepo: mockRedisRepo,
	}

	// Use plain context.Background() — no replayed pointer injected
	ctx := context.Background()
	result, err := uc.handleDuplicateTemplateRequest(ctx, "idempotency:template:test-key-no-replayed")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, templateID, result.ID)
	assert.Equal(t, "xml", result.OutputFormat)
	assert.Equal(t, "Cached template without replayed pointer", result.Description)
}
