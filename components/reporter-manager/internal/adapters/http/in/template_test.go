// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/components/reporter-manager/internal/services"
	constant "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/template"
	redisRepo "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"
	templateSeaweedFS "github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs/template"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func setupTemplateTestApp(handler *TemplateHandler) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	return app
}

func setupTemplateContextMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		logger := zap.NewNop().Sugar()
		tracer := noop.NewTracerProvider().Tracer("test")

		ctx := context.WithValue(c.UserContext(), "logger", logger)
		ctx = context.WithValue(ctx, "tracer", tracer)
		ctx = context.WithValue(ctx, "requestId", "test-request-id")

		c.SetUserContext(ctx)

		return c.Next()
	}
}

func createMultipartForm(t *testing.T, filename, content, outputFormat, description string) (*bytes.Buffer, string) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	// Add template file
	part, err := writer.CreateFormFile("template", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)

	// Add outputFormat field
	err = writer.WriteField("outputFormat", outputFormat)
	require.NoError(t, err)

	// Add description field
	err = writer.WriteField("description", description)
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return body, writer.FormDataContentType()
}

func TestTemplateHandler_GetTemplateByID(t *testing.T) {
	t.Parallel()

	templateID := uuid.New()
	templateEntity := &template.Template{
		ID:           templateID,
		OutputFormat: "HTML",
		Description:  "Test Template",
		FileName:     templateID.String() + ".tpl",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	tests := []struct {
		name           string
		templateID     string
		mockSetup      func(mockTemplateRepo *template.MockRepository)
		expectedStatus int
		expectError    bool
	}{
		{
			name:       "Success - Get template by ID",
			templateID: templateID.String(),
			mockSetup: func(mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					FindByID(gomock.Any(), templateID).
					Return(templateEntity, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:       "Error - Template not found",
			templateID: templateID.String(),
			mockSetup: func(mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					FindByID(gomock.Any(), templateID).
					Return(nil, errors.New("template not found"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name:           "Error - Invalid UUID",
			templateID:     "invalid-uuid",
			mockSetup:      func(mockTemplateRepo *template.MockRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTemplateRepo := template.NewMockRepository(ctrl)
			mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

			tt.mockSetup(mockTemplateRepo)

			useCase := &services.UseCase{
				Logger:            log.NewNop(),
				Tracer:            noop.NewTracerProvider().Tracer("test"),
				TemplateRepo:      mockTemplateRepo,
				TemplateSeaweedFS: mockSeaweedFS,
			}
			handler := &TemplateHandler{service: useCase}

			app := setupTemplateTestApp(handler)
			app.Get("/templates/:id", setupTemplateContextMiddleware(), ParsePathParametersUUID, handler.GetTemplateByID)

			req := httptest.NewRequest(http.MethodGet, "/templates/"+tt.templateID, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestTemplateHandler_GetAllTemplates(t *testing.T) {
	t.Parallel()

	templates := []*template.Template{
		{
			ID:           uuid.New(),
			OutputFormat: "HTML",
			Description:  "Test Template 1",
			FileName:     "template1.tpl",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		},
		{
			ID:           uuid.New(),
			OutputFormat: "XML",
			Description:  "Test Template 2",
			FileName:     "template2.tpl",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		},
	}

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(mockTemplateRepo *template.MockRepository)
		expectedStatus int
		expectError    bool
	}{
		{
			name:        "Success - Get all templates with default pagination",
			queryParams: "",
			mockSetup: func(mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(templates, nil)
				mockTemplateRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(len(templates)), nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Success - Get all templates with custom pagination",
			queryParams: "?limit=5&page=2",
			mockSetup: func(mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(templates, nil)
				mockTemplateRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(len(templates)), nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Success - Get all templates with filter",
			queryParams: "?outputFormat=HTML",
			mockSetup: func(mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*template.Template{templates[0]}, nil)
				mockTemplateRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(1), nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Error - Repository error",
			queryParams: "",
			mockSetup: func(mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name:           "Error - Invalid output format",
			queryParams:    "?outputFormat=INVALID",
			mockSetup:      func(mockTemplateRepo *template.MockRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTemplateRepo := template.NewMockRepository(ctrl)
			mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

			tt.mockSetup(mockTemplateRepo)

			useCase := &services.UseCase{
				Logger:            log.NewNop(),
				Tracer:            noop.NewTracerProvider().Tracer("test"),
				TemplateRepo:      mockTemplateRepo,
				TemplateSeaweedFS: mockSeaweedFS,
			}
			handler := &TemplateHandler{service: useCase}

			app := setupTemplateTestApp(handler)
			app.Get("/templates", setupTemplateContextMiddleware(), handler.GetAllTemplates)

			req := httptest.NewRequest(http.MethodGet, "/templates"+tt.queryParams, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestTemplateHandler_GetAllTemplates_NilSliceReturnsEmptyArray(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)
	mockTemplateRepo.EXPECT().FindList(gomock.Any(), gomock.Any()).Return(nil, nil)
	mockTemplateRepo.EXPECT().Count(gomock.Any(), gomock.Any()).Return(int64(0), nil)

	useCase := &services.UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), TemplateRepo: mockTemplateRepo, TemplateSeaweedFS: mockSeaweedFS}
	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Get("/templates", setupTemplateContextMiddleware(), handler.GetAllTemplates)

	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body struct {
		Items []any `json:"items"`
	}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.NotNil(t, body.Items)
	assert.Len(t, body.Items, 0)
}

func TestTemplateHandler_CreateTemplate_SetsReplayHeader(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateID := uuid.New()
	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)
	mockRedis := redisRepo.NewMockRedisRepository(ctrl)

	cachedTemplate := template.Template{ID: templateID, OutputFormat: "html", Description: "cached"}
	cachedBytes, err := json.Marshal(cachedTemplate)
	require.NoError(t, err)

	mockRedis.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return(string(cachedBytes), nil)

	useCase := &services.UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:      mockTemplateRepo,
		TemplateSeaweedFS: mockSeaweedFS,
		RedisRepo:         mockRedis,
	}
	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Post("/templates", setupTemplateContextMiddleware(), handler.CreateTemplate)

	body, contentType := createMultipartForm(t, "template.tpl", "<html>{{ content }}</html>", "html", "Test template")
	req := httptest.NewRequest(http.MethodPost, "/templates", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set(libConstants.IdempotencyKey, "idem-key")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get(libConstants.IdempotencyReplayed))
}

func TestTemplateHandler_DeleteTemplateByID(t *testing.T) {
	t.Parallel()

	templateID := uuid.New()

	tests := []struct {
		name           string
		templateID     string
		mockSetup      func(mockTemplateRepo *template.MockRepository)
		expectedStatus int
		expectError    bool
	}{
		{
			name:       "Success - Delete template",
			templateID: templateID.String(),
			mockSetup: func(mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					Delete(gomock.Any(), templateID, false).
					Return(nil)
			},
			expectedStatus: http.StatusNoContent,
			expectError:    false,
		},
		{
			name:       "Error - Template not found",
			templateID: templateID.String(),
			mockSetup: func(mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					Delete(gomock.Any(), templateID, false).
					Return(errors.New("template not found"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name:           "Error - Invalid UUID",
			templateID:     "invalid-uuid",
			mockSetup:      func(mockTemplateRepo *template.MockRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTemplateRepo := template.NewMockRepository(ctrl)
			mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

			tt.mockSetup(mockTemplateRepo)

			useCase := &services.UseCase{
				Logger:            log.NewNop(),
				Tracer:            noop.NewTracerProvider().Tracer("test"),
				TemplateRepo:      mockTemplateRepo,
				TemplateSeaweedFS: mockSeaweedFS,
			}
			handler := &TemplateHandler{service: useCase}

			app := setupTemplateTestApp(handler)
			app.Delete("/templates/:id", setupTemplateContextMiddleware(), ParsePathParametersUUID, handler.DeleteTemplateByID)

			req := httptest.NewRequest(http.MethodDelete, "/templates/"+tt.templateID, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestTemplateHandler_GetAllTemplates_EmptyResult(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	useCase := &services.UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:      mockTemplateRepo,
		TemplateSeaweedFS: mockSeaweedFS,
	}

	handler := &TemplateHandler{service: useCase}

	mockTemplateRepo.EXPECT().
		FindList(gomock.Any(), gomock.Any()).
		Return([]*template.Template{}, nil)
	mockTemplateRepo.EXPECT().
		Count(gomock.Any(), gomock.Any()).
		Return(int64(0), nil)

	app := setupTemplateTestApp(handler)
	app.Get("/templates", setupTemplateContextMiddleware(), handler.GetAllTemplates)

	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "items")
}

func TestTemplateHandler_CreateTemplate_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		filename       string
		content        string
		outputFormat   string
		description    string
		expectedStatus int
	}{
		{
			name:           "Error - Invalid file format (not .tpl)",
			filename:       "template.txt",
			content:        "some content",
			outputFormat:   "html",
			description:    "Test template",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Invalid output format",
			filename:       "template.tpl",
			content:        "<html>content</html>",
			outputFormat:   "invalid",
			description:    "Test template",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Empty description",
			filename:       "template.tpl",
			content:        "<html>content</html>",
			outputFormat:   "html",
			description:    "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Empty output format",
			filename:       "template.tpl",
			content:        "<html>content</html>",
			outputFormat:   "",
			description:    "Test template",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTemplateRepo := template.NewMockRepository(ctrl)
			mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

			useCase := &services.UseCase{
				Logger:            log.NewNop(),
				Tracer:            noop.NewTracerProvider().Tracer("test"),
				TemplateRepo:      mockTemplateRepo,
				TemplateSeaweedFS: mockSeaweedFS,
			}
			handler := &TemplateHandler{service: useCase}

			app := setupTemplateTestApp(handler)
			app.Post("/templates", setupTemplateContextMiddleware(), handler.CreateTemplate)

			body, contentType := createMultipartForm(t, tt.filename, tt.content, tt.outputFormat, tt.description)

			req := httptest.NewRequest(http.MethodPost, "/templates", body)
			req.Header.Set("Content-Type", contentType)

			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestTemplateHandler_CreateTemplate_EmptyFile(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	useCase := &services.UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:      mockTemplateRepo,
		TemplateSeaweedFS: mockSeaweedFS,
	}

	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Post("/templates", setupTemplateContextMiddleware(), handler.CreateTemplate)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("template", "template.tpl")
	require.NoError(t, err)
	_, err = part.Write([]byte(""))
	require.NoError(t, err)

	err = writer.WriteField("outputFormat", "html")
	require.NoError(t, err)
	err = writer.WriteField("description", "Test description")
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/templates", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestTemplateHandler_CreateTemplate_InvalidUTF8FileContent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	useCase := &services.UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:      mockTemplateRepo,
		TemplateSeaweedFS: mockSeaweedFS,
	}

	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Post("/templates", setupTemplateContextMiddleware(), handler.CreateTemplate)

	// File content carries invalid UTF-8 byte sequences (replay of fuzz seed
	// FuzzTemplate_InvalidTags/59acff2ca3d606b6). Templates are text by
	// definition, so this must be rejected at upload with 0288 / 400 rather
	// than accepted and later 500 during report rendering.
	body, contentType := createMultipartForm(t, "template.tpl", "0\xc9\xc9", "txt", "Test description")

	req := httptest.NewRequest(http.MethodPost, "/templates", body)
	req.Header.Set("Content-Type", contentType)

	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(respBody), constant.ErrInvalidUTF8.Error())
}

func TestTemplateHandler_CreateTemplate_NoFile(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	useCase := &services.UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:      mockTemplateRepo,
		TemplateSeaweedFS: mockSeaweedFS,
	}

	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Post("/templates", setupTemplateContextMiddleware(), handler.CreateTemplate)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	err := writer.WriteField("outputFormat", "html")
	require.NoError(t, err)
	err = writer.WriteField("description", "Test description")
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/templates", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestTemplateHandler_UpdateTemplateByID_ValidationErrors(t *testing.T) {
	t.Parallel()

	templateID := uuid.New()

	tests := []struct {
		name           string
		templateID     string
		filename       string
		content        string
		outputFormat   string
		description    string
		expectedStatus int
	}{
		{
			name:           "Error - Invalid UUID",
			templateID:     "invalid-uuid",
			filename:       "template.tpl",
			content:        "content",
			outputFormat:   "html",
			description:    "Test",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Invalid file format (not .tpl)",
			templateID:     templateID.String(),
			filename:       "template.txt",
			content:        "content",
			outputFormat:   "html",
			description:    "description",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTemplateRepo := template.NewMockRepository(ctrl)
			mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

			useCase := &services.UseCase{
				Logger:            log.NewNop(),
				Tracer:            noop.NewTracerProvider().Tracer("test"),
				TemplateRepo:      mockTemplateRepo,
				TemplateSeaweedFS: mockSeaweedFS,
			}
			handler := &TemplateHandler{service: useCase}

			app := setupTemplateTestApp(handler)
			app.Patch("/templates/:id", setupTemplateContextMiddleware(), ParsePathParametersUUID, handler.UpdateTemplateByID)

			body, contentType := createMultipartForm(t, tt.filename, tt.content, tt.outputFormat, tt.description)

			req := httptest.NewRequest(http.MethodPatch, "/templates/"+tt.templateID, body)
			req.Header.Set("Content-Type", contentType)

			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestNewTemplateHandler_NilService(t *testing.T) {
	t.Parallel()

	handler, err := NewTemplateHandler(nil)

	assert.Nil(t, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service must not be nil")
}

func TestNewTemplateHandler_ValidService(t *testing.T) {
	t.Parallel()

	svc := &services.UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	handler, err := NewTemplateHandler(svc)

	assert.NotNil(t, handler)
	require.NoError(t, err)
}

func TestTemplateHandler_CreateTemplate_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateID := uuid.New()
	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	mockTemplateRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(&template.Template{
			ID:           templateID,
			OutputFormat: "html",
			Description:  "Test template",
			FileName:     templateID.String() + ".tpl",
			CreatedAt:    time.Now(),
		}, nil)

	mockSeaweedFS.EXPECT().
		Put(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	useCase := &services.UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:      mockTemplateRepo,
		TemplateSeaweedFS: mockSeaweedFS,
	}
	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Post("/templates", setupTemplateContextMiddleware(), handler.CreateTemplate)

	body, contentType := createMultipartForm(t, "template.tpl", "<html>{{ content }}</html>", "html", "Test template")

	req := httptest.NewRequest(http.MethodPost, "/templates", body)
	req.Header.Set("Content-Type", contentType)

	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestTemplateHandler_CreateTemplate_ServiceError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	mockTemplateRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("database error"))

	useCase := &services.UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:      mockTemplateRepo,
		TemplateSeaweedFS: mockSeaweedFS,
	}
	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Post("/templates", setupTemplateContextMiddleware(), handler.CreateTemplate)

	body, contentType := createMultipartForm(t, "template.tpl", "<html>{{ content }}</html>", "html", "Test template")

	req := httptest.NewRequest(http.MethodPost, "/templates", body)
	req.Header.Set("Content-Type", contentType)

	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestTemplateHandler_CreateTemplate_FileContentMismatch(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	useCase := &services.UseCase{
		Logger:            log.NewNop(),
		Tracer:            noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:      mockTemplateRepo,
		TemplateSeaweedFS: mockSeaweedFS,
	}
	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Post("/templates", setupTemplateContextMiddleware(), handler.CreateTemplate)

	// XML content with html output format -- format mismatch
	body, contentType := createMultipartForm(t, "template.tpl", "<?xml version=\"1.0\"?><root></root>", "html", "Test template")

	req := httptest.NewRequest(http.MethodPost, "/templates", body)
	req.Header.Set("Content-Type", contentType)

	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestTemplateHandler_UpdateTemplateByID_ServiceError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateID := uuid.New()
	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	// UpdateTemplateByID calls validateOutputFormatAndFile first, then service layer
	// With outputFormat="xml" and file, it will attempt file validation
	mockTemplateRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Any()).
		Return(&template.Template{
			FileName:     "test.tpl",
			Description:  "current description",
			OutputFormat: "xml",
		}, nil)

	mockTemplateRepo.EXPECT().
		FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
		Return(func() *string { s := "xml"; return &s }(), map[string]map[string][]string{"current": {"table": {"field"}}}, "Test", nil)

	mockTemplateRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("database update failed"))

	useCase := &services.UseCase{
		Logger:              log.NewNop(),
		Tracer:              noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:        mockTemplateRepo,
		TemplateSeaweedFS:   mockSeaweedFS,
		ExternalDataSources: nil,
	}
	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Patch("/templates/:id", setupTemplateContextMiddleware(), ParsePathParametersUUID, handler.UpdateTemplateByID)

	body, contentType := createMultipartForm(t, "template.tpl", "<?xml version=\"1.0\"?><root></root>", "xml", "Updated description")

	req := httptest.NewRequest(http.MethodPatch, "/templates/"+templateID.String(), body)
	req.Header.Set("Content-Type", contentType)

	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestTemplateHandler_UpdateTemplateByID_NoFile(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	templateID := uuid.New()
	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockSeaweedFS := templateSeaweedFS.NewMockRepository(ctrl)

	// Update without file, only description
	mockTemplateRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	mockTemplateRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Any()).
		Return(&template.Template{
			ID:           templateID,
			FileName:     "test.tpl",
			OutputFormat: "xml",
			Description:  "Updated description only",
		}, nil)

	useCase := &services.UseCase{
		Logger:              log.NewNop(),
		Tracer:              noop.NewTracerProvider().Tracer("test"),
		TemplateRepo:        mockTemplateRepo,
		TemplateSeaweedFS:   mockSeaweedFS,
		ExternalDataSources: nil,
	}
	handler := &TemplateHandler{service: useCase}

	app := setupTemplateTestApp(handler)
	app.Patch("/templates/:id", setupTemplateContextMiddleware(), ParsePathParametersUUID, handler.UpdateTemplateByID)

	// Send form without template file field
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	err := writer.WriteField("description", "Updated description only")
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/templates/"+templateID.String(), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
