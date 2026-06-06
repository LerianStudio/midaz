// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/template"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"
	templateSeaweedFS "github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs/template"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func createFileHeaderFromString(content, filename string) (*multipart.FileHeader, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	h.Set("Content-Type", "application/tpl")

	part, err := writer.CreatePart(h)
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(part, bytes.NewReader([]byte(content)))
	if err != nil {
		return nil, err
	}

	writer.Close()

	// Parse multipart body to get FileHeader
	r := multipart.NewReader(body, writer.Boundary())
	form, err := r.ReadForm(int64(body.Len()))
	if err != nil {
		return nil, err
	}

	files := form.File["file"]
	if len(files) == 0 {
		return nil, errors.New("no file found in form")
	}

	return files[0], nil
}

func TestUseCase_UpdateTemplateByID(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because ResetRegisteredDataSourceIDsForTesting mutates global state
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Register datasource IDs for testing
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"midaz_organization", "midaz_onboarding"})

	mockTempRepo := template.NewMockRepository(ctrl)
	mockTempSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)
	mockDataSourceMongo := mongodb.NewMockRepository(ctrl)
	mockDataSourcePostgres := postgres.NewMockRepository(ctrl)
	htmlType := "html"
	xmlType := "xml"
	existingMappedFields := map[string]map[string][]string{
		"existing": {
			"table": {"field"},
		},
	}

	externalDataSourcesMap := map[string]pkg.DataSource{}
	externalDataSourcesMap["midaz_organization"] = pkg.DataSource{
		DatabaseType:       "mongodb",
		PostgresRepository: mockDataSourcePostgres,
		MongoDBRepository:  mockDataSourceMongo,
		DatabaseConfig:     nil,
		MongoURI:           "",
		MongoDBName:        "organization",
		Connection:         nil,
		Initialized:        true,
	}

	externalDataSourcesMap["midaz_onboarding"] = pkg.DataSource{
		DatabaseType:       "postgresql",
		PostgresRepository: mockDataSourcePostgres,
		MongoDBRepository:  mockDataSourceMongo,
		DatabaseConfig: &postgres.Connection{
			ConnectionString:   "",
			DBName:             "",
			ConnectionDB:       nil,
			Connected:          true,
			Logger:             nil,
			MaxOpenConnections: 0,
			MaxIdleConnections: 0,
		},
		MongoURI:    "",
		MongoDBName: "ledger",
		Connection:  nil,
		Initialized: true,
	}

	tempSvc := &UseCase{
		Logger:              log.NewNop(),
		Tracer:              noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:        mockTempRepo,
		TemplateSeaweedFS:   mockTempSeaweedFS,
		ExternalDataSources: pkg.NewSafeDataSources(externalDataSourcesMap),
	}

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
	templateTestXMLFileHeader, _ := createFileHeaderFromString(templateTest, "teste_template_XML.tpl")
	invalidUTF8FileHeader, _ := createFileHeaderFromString("0\xc9\xc9", "invalid_utf8.tpl")

	tests := []struct {
		name         string
		templateFile *multipart.FileHeader
		outFormat    string
		description  string
		tempId       uuid.UUID
		mockSetup    func()
		expectErr    bool
		errContains  string
	}{
		{
			name:         "Success - Update outputFormat template",
			templateFile: templateTestXMLFileHeader,
			outFormat:    "xml",
			description:  "Template Atualizado",
			tempId:       uuid.New(),
			mockSetup: func() {
				// First FindByID to get current template (before update)
				mockTempRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						FileName:     "test-template.tpl",
						Description:  "Old Description",
						OutputFormat: "xml",
					}, nil)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&xmlType, existingMappedFields, "Test", nil)

				mockTempRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockTempSeaweedFS.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				// Second FindByID to get updated template (after update)
				mockTempRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						FileName:     "test-template.tpl",
						OutputFormat: "xml",
					}, nil)
			},
			expectErr: false,
		},
		{
			name:         "Error - Update all template fail to find ouputFormat",
			templateFile: templateTestXMLFileHeader,
			description:  "Template Financeiro",
			tempId:       uuid.New(),
			errContains:  constant.ErrInternalServer.Error(),
			mockSetup: func() {
				mockTempRepo.EXPECT().
					FindOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInternalServer)
			},
			expectErr: true,
		},
		{
			name:         "Error - Update all template fail to outputFormat is not equal update file content",
			templateFile: templateTestXMLFileHeader,
			description:  "Template Financeiro",
			tempId:       uuid.New(),
			errContains:  constant.ErrFileContentInvalid.Error(),
			mockSetup: func() {
				htmlTypeP := &htmlType
				mockTempRepo.EXPECT().
					FindOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(htmlTypeP, nil)
			},
			expectErr: true,
		},
		{
			name:         "Error - Update outputFormat template invalid",
			templateFile: templateTestXMLFileHeader,
			outFormat:    "json",
			description:  "Template Financeiro",
			tempId:       uuid.New(),
			errContains:  constant.ErrInvalidOutputFormat.Error(),
			mockSetup:    func() {},
			expectErr:    true,
		},
		{
			name:         "Error - Update outputFormat template where template file content invalid",
			templateFile: templateTestXMLFileHeader,
			outFormat:    "html",
			description:  "Template Financeiro",
			tempId:       uuid.New(),
			errContains:  constant.ErrFileContentInvalid.Error(),
			mockSetup:    func() {},
			expectErr:    true,
		},
		{
			name:         "Error - Update template error",
			templateFile: templateTestXMLFileHeader,
			outFormat:    "xml",
			description:  "Template Atualizado",
			tempId:       uuid.New(),
			errContains:  constant.ErrInternalServer.Error(),
			mockSetup: func() {
				// FindByID to get current template (before update)
				mockTempRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						FileName:     "test-template.tpl",
						Description:  "Old Description",
						OutputFormat: "xml",
					}, nil)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&xmlType, existingMappedFields, "Test", nil)

				mockTempRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(constant.ErrInternalServer)
			},
			expectErr: true,
		},
		{
			name: "Error - Update template with <script> tag",
			templateFile: func() *multipart.FileHeader {
				fh, _ := createFileHeaderFromString(`<html><script>alert('x')</script></html>`, "malicious.tpl")
				return fh
			}(),
			outFormat:   "html",
			description: "Malicious Template",
			tempId:      uuid.New(),
			mockSetup:   func() {},
			expectErr:   true,
			errContains: constant.ErrScriptTagDetected.Error(),
		},
		{
			name:         "Error - GetTemplateByID after update fails",
			templateFile: templateTestXMLFileHeader,
			outFormat:    "xml",
			description:  "Template Atualizado",
			tempId:       uuid.New(),
			errContains:  "template not found after update",
			mockSetup: func() {
				// First FindByID to get current template (before update)
				mockTempRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						FileName:     "test-template.tpl",
						Description:  "Old Description",
						OutputFormat: "xml",
					}, nil)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&xmlType, existingMappedFields, "Test", nil)

				mockTempRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockTempSeaweedFS.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				// Second FindByID (after update) fails
				mockTempRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("template not found after update"))
			},
			expectErr: true,
		},
		{
			name:         "Error - ReadMultipartFile fails",
			templateFile: &multipart.FileHeader{Filename: "broken.tpl", Size: 1},
			outFormat:    "xml",
			description:  "Template Atualizado",
			tempId:       uuid.New(),
			mockSetup:    func() {},
			expectErr:    true,
			errContains:  "open",
		},
		{
			name:         "Error - Storage Put fails",
			templateFile: templateTestXMLFileHeader,
			outFormat:    "xml",
			description:  "Template Atualizado",
			tempId:       uuid.New(),
			errContains:  "storage unavailable",
			mockSetup: func() {
				// FindByID to get current template (before storage upload)
				mockTempRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						FileName:     "test-template.tpl",
						Description:  "Old Description",
						OutputFormat: "xml",
						UpdatedAt:    time.Unix(1700000000, 0),
					}, nil)

				mockTempRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&xmlType, existingMappedFields, "Test", nil)

				mockTempRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockTempSeaweedFS.EXPECT().
					Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("storage unavailable"))

				mockTempRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: true,
		},
		{
			name:         "Error - FindByID fails before update",
			templateFile: templateTestXMLFileHeader,
			outFormat:    "xml",
			description:  "Template Atualizado",
			tempId:       uuid.New(),
			errContains:  "template not found",
			mockSetup: func() {
				// FindByID fails before update
				mockTempRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("template not found"))
			},
			expectErr: true,
		},
		{
			name:         "Success - Description only update (no file)",
			templateFile: nil,
			description:  "Updated Description Only",
			tempId:       uuid.New(),
			mockSetup: func() {
				// No FindByID before update when no file is provided
				mockTempRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				mockTempRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(&template.Template{
						FileName:     "test-template.tpl",
						OutputFormat: "xml",
						Description:  "Updated Description Only",
					}, nil)
			},
			expectErr: false,
		},
		{
			name:         "Error - Invalid UTF-8 in description",
			templateFile: nil,
			description:  "\xb9",
			tempId:       uuid.New(),
			errContains:  "TPL-0061",
			mockSetup:    func() {},
			expectErr:    true,
		},
		{
			name:         "Error - Invalid UTF-8 in template file content",
			templateFile: invalidUTF8FileHeader,
			outFormat:    "txt",
			description:  "Valid description",
			tempId:       uuid.New(),
			errContains:  "TPL-0061",
			mockSetup:    func() {},
			expectErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			_, _, err := tempSvc.UpdateTemplateByID(ctx, tt.outFormat, tt.description, tt.tempId, tt.templateFile)

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUseCase_UpdateTemplateByID_OutputFormatWithoutFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{})

	mockTempRepo := template.NewMockRepository(ctrl)
	mockTempSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	tempSvc := &UseCase{
		Logger:              log.NewNop(),
		Tracer:              noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:        mockTempRepo,
		TemplateSeaweedFS:   mockTempSeaweedFS,
		ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
	}

	// Attempt to update outputFormat without providing a file
	ctx := context.Background()
	_, _, err := tempSvc.UpdateTemplateByID(ctx, "xml", "Updated Desc", uuid.New(), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrOutputFormatWithoutTemplateFile.Error())
}

func TestUseCase_UpdateTemplateByID_NilOutputFormatFromDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"midaz_organization"})

	mockTempRepo := template.NewMockRepository(ctrl)
	mockTempSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)
	mockDataSourceMongo := mongodb.NewMockRepository(ctrl)

	externalDataSourcesMap := map[string]pkg.DataSource{
		"midaz_organization": {
			DatabaseType:      "mongodb",
			MongoDBRepository: mockDataSourceMongo,
			MongoDBName:       "organization",
			Initialized:       true,
		},
	}

	tempSvc := &UseCase{
		Logger:              log.NewNop(),
		Tracer:              noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:        mockTempRepo,
		TemplateSeaweedFS:   mockTempSeaweedFS,
		ExternalDataSources: pkg.NewSafeDataSources(externalDataSourcesMap),
	}

	templateContent := `{% for org in midaz_organization.organization %}<Doc>{{ org.legal_document }}</Doc>{% endfor %}`
	fileHeader, errFh := createFileHeaderFromString(templateContent, "test_template.tpl")
	require.NoError(t, errFh)

	// FindOutputFormatByID returns nil output format
	mockTempRepo.EXPECT().
		FindOutputFormatByID(gomock.Any(), gomock.Any()).
		Return(nil, nil)

	ctx := context.Background()
	_, _, err := tempSvc.UpdateTemplateByID(ctx, "", "Updated Desc", uuid.New(), fileHeader)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "output format not found for template")
}

func TestUseCase_BuildSetFields(t *testing.T) {
	t.Parallel()

	uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	tests := []struct {
		name         string
		description  string
		outputFormat string
		mappedFields map[string]map[string][]string
		expectKeys   []string
	}{
		{
			name:         "All fields provided",
			description:  "Updated description",
			outputFormat: "xml",
			mappedFields: map[string]map[string][]string{"ds": {"table": {"col"}}},
			expectKeys:   []string{"description", "output_format", "mapped_fields", "updated_at"},
		},
		{
			name:         "Only description",
			description:  "New desc",
			outputFormat: "",
			mappedFields: nil,
			expectKeys:   []string{"description", "updated_at"},
		},
		{
			name:         "No optional fields",
			description:  "",
			outputFormat: "",
			mappedFields: nil,
			expectKeys:   []string{"updated_at"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := uc.buildSetFields(tt.description, tt.outputFormat, tt.mappedFields)

			for _, key := range tt.expectKeys {
				assert.Contains(t, result, key)
			}
		})
	}
}
