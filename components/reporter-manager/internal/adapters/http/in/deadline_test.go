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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/reporter/components/manager/internal/services"
	pkg "github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/mongodb/deadline"
	"github.com/LerianStudio/reporter/pkg/mongodb/template"
	pkgHTTP "github.com/LerianStudio/reporter/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func setupDeadlineTestApp(handler *DeadlineHandler) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	return app
}

func setupDeadlineContextMiddleware() fiber.Handler {
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

func boolPtr(b bool) *bool {
	return &b
}

func TestNewDeadlineHandler_NilService(t *testing.T) {
	t.Parallel()

	handler, err := NewDeadlineHandler(nil)

	assert.Nil(t, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service must not be nil")
}

func TestNewDeadlineHandler_ValidService(t *testing.T) {
	t.Parallel()

	svc := &services.UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	handler, err := NewDeadlineHandler(svc)

	assert.NotNil(t, handler)
	require.NoError(t, err)
}

func TestDeadlineHandler_CreateDeadline(t *testing.T) {
	t.Parallel()

	dueDate := time.Now().Add(24 * time.Hour)
	templateID := uuid.New()

	tests := []struct {
		name           string
		body           any
		mockSetup      func(mockDeadlineRepo *deadline.MockRepository, mockTemplateRepo *template.MockRepository)
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name: "Success - Create deadline without templateId",
			body: deadline.CreateDeadlineInput{
				Name:      "Monthly Regulatory Report",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository, mockTemplateRepo *template.MockRepository) {
				mockDeadlineRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&deadline.Deadline{
						ID:        uuid.New(),
						Name:      "Monthly Regulatory Report",
						Type:      "regulatory",
						DueDate:   dueDate,
						Frequency: "monthly",
						Color:     "#FF5733",
						Active:    true,
						Status:    deadline.StatusPending,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil)
			},
			expectedStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var resp deadline.Deadline
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "Monthly Regulatory Report", resp.Name)
				assert.Equal(t, "regulatory", resp.Type)
				assert.Equal(t, "#FF5733", resp.Color)
				assert.True(t, resp.Active)
				assert.Equal(t, deadline.StatusPending, resp.Status)
			},
		},
		{
			name: "Success - Create deadline with templateId fills templateName",
			body: deadline.CreateDeadlineInput{
				Name:       "Monthly Regulatory Report",
				Type:       "regulatory",
				TemplateID: &templateID,
				DueDate:    dueDate,
				Frequency:  "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository, mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					FindByID(gomock.Any(), templateID).
					Return(&template.Template{
						ID:          templateID,
						Description: "Template Financeiro",
					}, nil)

				mockDeadlineRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&deadline.Deadline{
						ID:           uuid.New(),
						Name:         "Monthly Regulatory Report",
						Type:         "regulatory",
						TemplateID:   &templateID,
						TemplateName: "Template Financeiro",
						DueDate:      dueDate,
						Frequency:    "monthly",
						Color:        "#FF5733",
						Active:       true,
						Status:       deadline.StatusPending,
						CreatedAt:    time.Now(),
						UpdatedAt:    time.Now(),
					}, nil)
			},
			expectedStatus: http.StatusCreated,
			checkBody: func(t *testing.T, body []byte) {
				var resp deadline.Deadline
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "Template Financeiro", resp.TemplateName)
				assert.NotNil(t, resp.TemplateID)
			},
		},
		{
			name: "Error - Service returns error on create",
			body: deadline.CreateDeadlineInput{
				Name:      "Monthly Report",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository, mockTemplateRepo *template.MockRepository) {
				mockDeadlineRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "Error - Missing required name field (validation via WithBody)",
			body: deadline.CreateDeadlineInput{
				Name:      "",
				Type:      "regulatory",
				DueDate:   dueDate,
				Frequency: "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository, mockTemplateRepo *template.MockRepository) {
				// WithBody validation catches missing required "name" field
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Error - Template not found when templateId provided",
			body: deadline.CreateDeadlineInput{
				Name:       "Monthly Report",
				Type:       "regulatory",
				TemplateID: &templateID,
				DueDate:    dueDate,
				Frequency:  "monthly",

				Color: "#FF5733",
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository, mockTemplateRepo *template.MockRepository) {
				mockTemplateRepo.EXPECT().
					FindByID(gomock.Any(), templateID).
					Return(nil, errors.New("template not found"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDeadlineRepo := deadline.NewMockRepository(ctrl)
			mockTemplateRepo := template.NewMockRepository(ctrl)

			tt.mockSetup(mockDeadlineRepo, mockTemplateRepo)

			useCase := &services.UseCase{
				Logger:       log.NewNop(),
				Tracer:       noop.NewTracerProvider().Tracer("test"),
				DeadlineRepo: mockDeadlineRepo,
				TemplateRepo: mockTemplateRepo,
			}
			handler := &DeadlineHandler{service: useCase}

			app := setupDeadlineTestApp(handler)
			app.Post("/v1/deadlines", setupDeadlineContextMiddleware(), pkgHTTP.WithBody(new(deadline.CreateDeadlineInput), handler.CreateDeadline))

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/deadlines", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

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

func TestDeadlineHandler_GetAllDeadlines(t *testing.T) {
	t.Parallel()

	dueDate := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(mockDeadlineRepo *deadline.MockRepository)
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:        "Success - Get all deadlines with default pagination",
			queryParams: "",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*deadline.Deadline{
						{
							ID:        uuid.New(),
							Name:      "Monthly Report",
							Type:      "regulatory",
							DueDate:   dueDate,
							Frequency: "monthly",
							Color:     "#FF5733",
							Active:    true,
							Status:    deadline.StatusPending,
						},
					}, nil)

				mockDeadlineRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(1), nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "items")
			},
		},
		{
			name:        "Success - Get all deadlines with custom pagination",
			queryParams: "?limit=5&page=2",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*deadline.Deadline{}, nil)

				mockDeadlineRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "items")
			},
		},
		{
			name:        "Success - Get all deadlines with sort order",
			queryParams: "?sort_order=asc",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*deadline.Deadline{}, nil)

				mockDeadlineRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "items")
			},
		},
		{
			name:        "Success - Empty list response",
			queryParams: "",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*deadline.Deadline{}, nil)

				mockDeadlineRepo.EXPECT().
					Count(gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "items")
			},
		},
		{
			name:        "Error - Repository FindList fails",
			queryParams: "",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "Error - Invalid limit parameter",
			queryParams:    "?limit=abc",
			mockSetup:      func(mockDeadlineRepo *deadline.MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Negative page parameter",
			queryParams:    "?page=-1",
			mockSetup:      func(mockDeadlineRepo *deadline.MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Error - Invalid sort order",
			queryParams:    "?sort_order=invalid",
			mockSetup:      func(mockDeadlineRepo *deadline.MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDeadlineRepo := deadline.NewMockRepository(ctrl)

			tt.mockSetup(mockDeadlineRepo)

			useCase := &services.UseCase{
				Logger:       log.NewNop(),
				Tracer:       noop.NewTracerProvider().Tracer("test"),
				DeadlineRepo: mockDeadlineRepo,
			}
			handler := &DeadlineHandler{service: useCase}

			app := setupDeadlineTestApp(handler)
			app.Get("/v1/deadlines", setupDeadlineContextMiddleware(), handler.GetAllDeadlines)

			req := httptest.NewRequest(http.MethodGet, "/v1/deadlines"+tt.queryParams, nil)
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

func TestDeadlineHandler_UpdateDeadlineByID(t *testing.T) {
	t.Parallel()

	deadlineID := uuid.New()
	newName := "Updated Deadline"
	newColor := "#00FF00"
	activeTrue := true
	activeFalse := false
	newDueDate := time.Now().Add(48 * time.Hour)

	currentDl := func() *deadline.Deadline {
		return &deadline.Deadline{
			ID:        deadlineID,
			Name:      "Existing Deadline",
			Type:      "regulatory",
			DueDate:   time.Now().Add(24 * time.Hour),
			Frequency: "monthly",
			Color:     "#FF5733",
			Active:    true,
			Status:    deadline.StatusPending,
		}
	}

	tests := []struct {
		name           string
		deadlineID     string
		body           any
		mockSetup      func(mockDeadlineRepo *deadline.MockRepository)
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:       "Success - Update deadline name (partial update)",
			deadlineID: deadlineID.String(),
			body: deadline.UpdateDeadlineInput{
				Name: &newName,
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDl(), nil)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(first)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:     deadlineID,
						Name:   newName,
						Active: true,
						Status: deadline.StatusPending,
					}, nil).After(update)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp deadline.Deadline
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, newName, resp.Name)
				assert.Equal(t, deadlineID, resp.ID)
			},
		},
		{
			name:       "Success - Activate deadline",
			deadlineID: deadlineID.String(),
			body: deadline.UpdateDeadlineInput{
				Active: &activeTrue,
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDl(), nil)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(first)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:     deadlineID,
						Name:   "Some Deadline",
						Active: true,
						Status: deadline.StatusPending,
					}, nil).After(update)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "Success - Deactivate deadline",
			deadlineID: deadlineID.String(),
			body: deadline.UpdateDeadlineInput{
				Active: &activeFalse,
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDl(), nil)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(first)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:     deadlineID,
						Name:   "Some Deadline",
						Active: false,
						Status: deadline.StatusPending,
					}, nil).After(update)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "Success - Update multiple fields",
			deadlineID: deadlineID.String(),
			body: deadline.UpdateDeadlineInput{
				Name:    &newName,
				Color:   &newColor,
				DueDate: &newDueDate,
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDl(), nil)

				update := mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil).After(first)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:      deadlineID,
						Name:    newName,
						Color:   newColor,
						DueDate: newDueDate,
						Status:  deadline.StatusPending,
					}, nil).After(update)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Error - Invalid UUID in path",
			deadlineID:     "invalid-uuid",
			body:           deadline.UpdateDeadlineInput{Name: &newName},
			mockSetup:      func(mockDeadlineRepo *deadline.MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:       "Error - Service Update fails",
			deadlineID: deadlineID.String(),
			body: deadline.UpdateDeadlineInput{
				Name: &newName,
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				first := mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(currentDl(), nil)

				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(errors.New("database error")).After(first)
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDeadlineRepo := deadline.NewMockRepository(ctrl)

			tt.mockSetup(mockDeadlineRepo)

			useCase := &services.UseCase{
				Logger:       log.NewNop(),
				Tracer:       noop.NewTracerProvider().Tracer("test"),
				DeadlineRepo: mockDeadlineRepo,
			}
			handler := &DeadlineHandler{service: useCase}

			app := setupDeadlineTestApp(handler)
			app.Patch("/v1/deadlines/:id", setupDeadlineContextMiddleware(), ParsePathParametersUUID, pkgHTTP.WithBody(new(deadline.UpdateDeadlineInput), handler.UpdateDeadlineByID))

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPatch, "/v1/deadlines/"+tt.deadlineID, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

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

func TestDeadlineHandler_DeleteDeadlineByID(t *testing.T) {
	t.Parallel()

	deadlineID := uuid.New()

	tests := []struct {
		name           string
		deadlineID     string
		mockSetup      func(mockDeadlineRepo *deadline.MockRepository)
		expectedStatus int
	}{
		{
			name:       "Success - Soft delete deadline",
			deadlineID: deadlineID.String(),
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					Delete(gomock.Any(), deadlineID).
					Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:       "Error - Deadline not found on delete",
			deadlineID: deadlineID.String(),
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					Delete(gomock.Any(), deadlineID).
					Return(pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", "deadlines"))
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Error - Invalid UUID in path",
			deadlineID:     "invalid-uuid",
			mockSetup:      func(mockDeadlineRepo *deadline.MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:       "Error - Repository Delete fails",
			deadlineID: deadlineID.String(),
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					Delete(gomock.Any(), deadlineID).
					Return(errors.New("database connection error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDeadlineRepo := deadline.NewMockRepository(ctrl)

			tt.mockSetup(mockDeadlineRepo)

			useCase := &services.UseCase{
				Logger:       log.NewNop(),
				Tracer:       noop.NewTracerProvider().Tracer("test"),
				DeadlineRepo: mockDeadlineRepo,
			}
			handler := &DeadlineHandler{service: useCase}

			app := setupDeadlineTestApp(handler)
			app.Delete("/v1/deadlines/:id", setupDeadlineContextMiddleware(), ParsePathParametersUUID, handler.DeleteDeadlineByID)

			req := httptest.NewRequest(http.MethodDelete, "/v1/deadlines/"+tt.deadlineID, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestDeadlineHandler_DeliverDeadline(t *testing.T) {
	t.Parallel()

	deadlineID := uuid.New()

	tests := []struct {
		name           string
		deadlineID     string
		body           any
		mockSetup      func(mockDeadlineRepo *deadline.MockRepository)
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:       "Success - Mark deadline as delivered",
			deadlineID: deadlineID.String(),
			body: deadline.DeliverDeadlineInput{
				Delivered: boolPtr(true),
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil)

				now := time.Now()
				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:          deadlineID,
						Name:        "Monthly Report",
						Status:      deadline.StatusDelivered,
						DeliveredAt: &now,
					}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp deadline.Deadline
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, deadline.StatusDelivered, resp.Status)
				assert.NotNil(t, resp.DeliveredAt)
			},
		},
		{
			name:       "Success - Clear delivered status (delivered=false)",
			deadlineID: deadlineID.String(),
			body: deadline.DeliverDeadlineInput{
				Delivered: boolPtr(false),
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(&deadline.Deadline{
						ID:     deadlineID,
						Name:   "Monthly Report",
						Status: deadline.StatusPending,
					}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp deadline.Deadline
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, deadline.StatusPending, resp.Status)
				assert.Nil(t, resp.DeliveredAt)
			},
		},
		{
			name:           "Error - Invalid UUID in path",
			deadlineID:     "invalid-uuid",
			body:           deadline.DeliverDeadlineInput{Delivered: boolPtr(true)},
			mockSetup:      func(mockDeadlineRepo *deadline.MockRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:       "Error - Service returns error on deliver",
			deadlineID: deadlineID.String(),
			body: deadline.DeliverDeadlineInput{
				Delivered: boolPtr(true),
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:       "Error - FindByID after deliver fails",
			deadlineID: deadlineID.String(),
			body: deadline.DeliverDeadlineInput{
				Delivered: boolPtr(true),
			},
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					Update(gomock.Any(), deadlineID, gomock.Any()).
					Return(nil)

				mockDeadlineRepo.EXPECT().
					FindByID(gomock.Any(), deadlineID).
					Return(nil, errors.New("not found"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDeadlineRepo := deadline.NewMockRepository(ctrl)

			tt.mockSetup(mockDeadlineRepo)

			useCase := &services.UseCase{
				Logger:       log.NewNop(),
				Tracer:       noop.NewTracerProvider().Tracer("test"),
				DeadlineRepo: mockDeadlineRepo,
			}
			handler := &DeadlineHandler{service: useCase}

			app := setupDeadlineTestApp(handler)
			app.Patch("/v1/deadlines/:id/deliver", setupDeadlineContextMiddleware(), ParsePathParametersUUID, pkgHTTP.WithBody(new(deadline.DeliverDeadlineInput), handler.DeliverDeadline))

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPatch, "/v1/deadlines/"+tt.deadlineID+"/deliver", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

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
