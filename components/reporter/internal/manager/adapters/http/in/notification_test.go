// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/components/reporter/internal/manager/services"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/deadline"

	midazHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func setupNotificationTestApp(handler *NotificationHandler) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return midazHTTP.WithError(c, err)
		},
	})

	return app
}

func setupNotificationContextMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		tracer := noop.NewTracerProvider().Tracer("test")

		ctx := ctxutil.ContextWithLogger(c.UserContext(), log.NewNop())
		ctx = ctxutil.ContextWithTracer(ctx, tracer)

		c.SetUserContext(ctx)

		return c.Next()
	}
}

func TestNewNotificationHandler_NilService(t *testing.T) {
	t.Parallel()

	handler, err := NewNotificationHandler(nil)

	assert.Nil(t, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service must not be nil")
}

func TestNewNotificationHandler_ValidService(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svc := &services.UseCase{
		Logger:       log.NewNop(),
		Tracer:       noop.NewTracerProvider().Tracer("test"),
		DeadlineRepo: deadline.NewMockRepository(ctrl),
	}

	handler, err := NewNotificationHandler(svc)

	assert.NotNil(t, handler)
	require.NoError(t, err)
}

func TestNotificationHandler_GetNotifications(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(24 * time.Hour)

	// Helper to create deadline pointers with specific due dates relative to now.
	makeDeadline := func(id uuid.UUID, name, typ string, dueDate time.Time, frequency string, notifyDaysBefore int, color string) *deadline.Deadline {
		return &deadline.Deadline{
			ID:               id,
			Name:             name,
			Type:             typ,
			DueDate:          dueDate,
			Frequency:        frequency,
			Active:           true,
			NotifyDaysBefore: notifyDaysBefore,
			Color:            color,
			Status:           deadline.ComputeStatus(dueDate, nil, now),
			CreatedAt:        now.Add(-30 * 24 * time.Hour),
			UpdatedAt:        now.Add(-1 * 24 * time.Hour),
		}
	}

	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(mockDeadlineRepo *deadline.MockRepository)
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:        "Success - Returns notifications with mixed severities ordered by urgency",
			queryParams: "",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindActiveNotifiable(gomock.Any()).
					Return([]*deadline.Deadline{
						// Info (due in 10 days, notifyDaysBefore=15) — intentionally first to test sort
						makeDeadline(id3, "Annual Audit", "regulatory", now.Add(10*24*time.Hour), "annual", 15, "#3b82f6"),
						// Overdue (due yesterday)
						makeDeadline(id1, "CADOC 4010", "regulatory", now.Add(-24*time.Hour), "monthly", 5, "#ef4444"),
						// Warning (due in 3 days)
						makeDeadline(id2, "Tax Report", "custom", now.Add(3*24*time.Hour), "monthly", 5, "#f59e0b"),
					}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp NotificationResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, 3, resp.Total)
				require.Len(t, resp.Items, 3)

				// First item should be overdue (most urgent)
				assert.Equal(t, id1.String(), resp.Items[0].ID)
				assert.Equal(t, "overdue", resp.Items[0].Severity)
				assert.Less(t, resp.Items[0].DaysUntilDue, 0)

				// Second item should be warning
				assert.Equal(t, id2.String(), resp.Items[1].ID)
				assert.Equal(t, "warning", resp.Items[1].Severity)
				assert.Equal(t, 3, resp.Items[1].DaysUntilDue)

				// Third item should be info
				assert.Equal(t, id3.String(), resp.Items[2].ID)
				assert.Equal(t, "info", resp.Items[2].Severity)
				assert.Equal(t, 10, resp.Items[2].DaysUntilDue)
			},
		},
		{
			name:        "Success - Returns empty list when no notifications",
			queryParams: "",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindActiveNotifiable(gomock.Any()).
					Return([]*deadline.Deadline{}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp NotificationResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, 0, resp.Total)
				assert.Empty(t, resp.Items)
			},
		},
		{
			name:        "Success - Custom limit parameter",
			queryParams: "?limit=5",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindActiveNotifiable(gomock.Any()).
					Return([]*deadline.Deadline{
						makeDeadline(id1, "CADOC 4010", "regulatory", now.Add(-24*time.Hour), "monthly", 5, "#ef4444"),
					}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp NotificationResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, 1, resp.Total)
				require.Len(t, resp.Items, 1)
			},
		},
		{
			name:        "Success - Limit at maximum boundary (100)",
			queryParams: "?limit=100",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindActiveNotifiable(gomock.Any()).
					Return([]*deadline.Deadline{}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp NotificationResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, 0, resp.Total)
			},
		},
		{
			name:        "Success - Limit at minimum boundary (1)",
			queryParams: "?limit=1",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindActiveNotifiable(gomock.Any()).
					Return([]*deadline.Deadline{
						makeDeadline(id1, "CADOC 4010", "regulatory", now.Add(-24*time.Hour), "monthly", 5, "#ef4444"),
					}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp NotificationResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, 1, resp.Total)
				require.Len(t, resp.Items, 1)
			},
		},
		{
			name:        "Success - Only overdue deadlines",
			queryParams: "",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindActiveNotifiable(gomock.Any()).
					Return([]*deadline.Deadline{
						// Overdue 2 intentionally first to test that sort puts most-overdue first
						makeDeadline(id2, "Overdue 2", "regulatory", now.Add(-1*24*time.Hour), "monthly", 5, "#ef4444"),
						makeDeadline(id1, "Overdue 1", "regulatory", now.Add(-5*24*time.Hour), "monthly", 5, "#ef4444"),
					}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp NotificationResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, 2, resp.Total)
				// Most overdue first (most negative daysUntilDue)
				assert.Equal(t, "overdue", resp.Items[0].Severity)
				assert.Equal(t, "overdue", resp.Items[1].Severity)
				assert.Less(t, resp.Items[0].DaysUntilDue, resp.Items[1].DaysUntilDue)
			},
		},
		{
			name:        "Error 400 - Limit is zero",
			queryParams: "?limit=0",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				// No repository calls expected for validation errors
			},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Contains(t, resp["message"], "limit must be between")
			},
		},
		{
			name:        "Error 400 - Limit is negative",
			queryParams: "?limit=-1",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				// No repository calls expected for validation errors
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error 400 - Limit exceeds maximum (101)",
			queryParams: "?limit=101",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				// No repository calls expected for validation errors
			},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Contains(t, resp["message"], "limit must be between")
			},
		},
		{
			name:        "Error 400 - Limit is not a number",
			queryParams: "?limit=abc",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				// No repository calls expected for validation errors
			},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Contains(t, resp["message"], "limit must be an integer")
			},
		},
		{
			name:        "Error 400 - Limit is a float",
			queryParams: "?limit=5.5",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				// No repository calls expected for validation errors
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "Error 400 - Limit is special characters",
			queryParams: "?limit=<script>",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				// No repository calls expected for validation errors
			},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Contains(t, resp["message"], "limit must be an integer")
			},
		},
		{
			name:        "Success - Empty limit string uses default",
			queryParams: "?limit=",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				// Empty string treated as absent, default limit = 10
				mockDeadlineRepo.EXPECT().
					FindActiveNotifiable(gomock.Any()).
					Return([]*deadline.Deadline{}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp NotificationResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, 0, resp.Total)
			},
		},
		{
			name:        "Error 500 - Repository returns error",
			queryParams: "",
			mockSetup: func(mockDeadlineRepo *deadline.MockRepository) {
				mockDeadlineRepo.EXPECT().
					FindActiveNotifiable(gomock.Any()).
					Return(nil, errors.New("database connection error"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "0046", resp["code"])
				assert.Equal(t, "Internal Server Error", resp["title"])
				assert.Contains(t, resp["message"], "The server encountered an unexpected error")
			},
		},
	}

	for _, tt := range tests {
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

			handler, err := NewNotificationHandler(useCase)
			require.NoError(t, err)

			app := setupNotificationTestApp(handler)
			app.Get("/v1/deadlines/notifications", setupNotificationContextMiddleware(), handler.GetNotifications)

			req := httptest.NewRequest(http.MethodGet, "/v1/deadlines/notifications"+tt.queryParams, nil)
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

// TestComputeNotificationSeverity tests the severity computation logic in isolation.
func TestComputeNotificationSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		daysUntilDue     int
		expectedSeverity string
	}{
		{
			name:             "Overdue - 5 days past due",
			daysUntilDue:     -5,
			expectedSeverity: "overdue",
		},
		{
			name:             "Overdue - 1 day past due",
			daysUntilDue:     -1,
			expectedSeverity: "overdue",
		},
		{
			name:             "Warning - due today",
			daysUntilDue:     0,
			expectedSeverity: "warning",
		},
		{
			name:             "Warning - due in 1 day",
			daysUntilDue:     1,
			expectedSeverity: "warning",
		},
		{
			name:             "Warning - due in 7 days",
			daysUntilDue:     7,
			expectedSeverity: "warning",
		},
		{
			name:             "Info - due in 8 days",
			daysUntilDue:     8,
			expectedSeverity: "info",
		},
		{
			name:             "Info - due in 30 days",
			daysUntilDue:     30,
			expectedSeverity: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			severity := ComputeNotificationSeverity(tt.daysUntilDue)
			assert.Equal(t, tt.expectedSeverity, severity)
		})
	}
}

// TestComputeDaysUntilDue tests the days-until-due computation.
func TestComputeDaysUntilDue(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(24 * time.Hour)

	tests := []struct {
		name         string
		dueDate      time.Time
		now          time.Time
		expectedDays int
	}{
		{
			name:         "Due today",
			dueDate:      now,
			now:          now,
			expectedDays: 0,
		},
		{
			name:         "Due tomorrow",
			dueDate:      now.Add(24 * time.Hour),
			now:          now,
			expectedDays: 1,
		},
		{
			name:         "Overdue by 1 day",
			dueDate:      now.Add(-24 * time.Hour),
			now:          now,
			expectedDays: -1,
		},
		{
			name:         "Due in 10 days",
			dueDate:      now.Add(10 * 24 * time.Hour),
			now:          now,
			expectedDays: 10,
		},
		{
			name:         "Overdue by 30 days",
			dueDate:      now.Add(-30 * 24 * time.Hour),
			now:          now,
			expectedDays: -30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			days := ComputeDaysUntilDue(tt.dueDate, tt.now)
			assert.Equal(t, tt.expectedDays, days)
		})
	}
}
