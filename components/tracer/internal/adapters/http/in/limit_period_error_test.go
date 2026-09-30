// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	commandMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

type limitPeriodErrorResponse struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
	Status int    `json:"status"`
}

func TestLimitPeriodErrors_RenderAcrossTransports(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		code   string
		status int
		title  string
		detail string
	}{
		{"time_window_mismatch", constant.ErrLimitTimeWindowMismatch, "0438", http.StatusBadRequest, "Limit Time Window Mismatch", "ActiveTimeStart and activeTimeEnd must both be set or both be nil."},
		{"time_window_zero_width", constant.ErrLimitTimeWindowZeroWidth, "0439", http.StatusBadRequest, "Limit Time Window Zero Width", "ActiveTimeStart cannot equal activeTimeEnd."},
		{"custom_dates_not_allowed", constant.ErrLimitCustomDatesNotAllowed, "0443", http.StatusBadRequest, "Limit Custom Dates Not Allowed", "CustomStartDate/customEndDate only allowed for CUSTOM limitType."},
		{"custom_period_too_long", constant.ErrLimitCustomPeriodTooLong, "0445", http.StatusUnprocessableEntity, "Limit Custom Period Too Long", "Custom period cannot exceed 5 years."},
		{"custom_period_expired", constant.ErrLimitCustomPeriodExpired, "0446", http.StatusUnprocessableEntity, "Limit Custom Period Expired", "Custom period end date must be in the future."},
		{"invalid_custom_start_format", constant.ErrLimitInvalidCustomStartFormat, "0447", http.StatusBadRequest, "Limit Invalid Custom Start Format", "Invalid customStartDate format, expected RFC3339."},
		{"invalid_custom_end_format", constant.ErrLimitInvalidCustomEndFormat, "0448", http.StatusBadRequest, "Limit Invalid Custom End Format", "Invalid customEndDate format, expected RFC3339."},
		{"custom_dates_required", constant.ErrLimitCustomDatesRequired, "0449", http.StatusBadRequest, "Limit Custom Dates Required", "CustomStartDate and customEndDate required for CUSTOM limitType."},
		{"custom_dates_order", constant.ErrLimitCustomDatesOrder, "0450", http.StatusBadRequest, "Limit Custom Dates Order", "CustomStartDate must be before customEndDate."},
	}

	for _, tc := range tests {
		for _, wrapped := range []bool{false, true} {
			for _, transport := range []string{"fiber", "huma"} {
				for _, method := range []string{http.MethodPost, http.MethodPatch} {
					name := fmt.Sprintf("%s/wrapped=%t/%s/%s", tc.name, wrapped, transport, method)
					t.Run(name, func(t *testing.T) {
						ctrl := gomock.NewController(t)
						service := NewMockLimitService(ctrl)
						serviceErr := tc.err
						if wrapped {
							serviceErr = fmt.Errorf("domain validation: %w", tc.err)
						}

						id := testutil.MustDeterministicUUID(700)
						if method == http.MethodPost {
							service.EXPECT().CreateLimit(gomock.Any(), gomock.Any()).Return(nil, serviceErr)
						} else {
							service.EXPECT().UpdateLimit(gomock.Any(), id, gomock.Any()).Return(nil, serviceErr)
						}

						app := buildLimitErrorTestApp(t, service, transport)
						path := "/v1/limits"
						body := []byte(`{"name":"Daily Cap","limitType":"DAILY","maxAmount":"1000.00","asset":"USD","scopes":[{"accountId":"550e8400-e29b-41d4-a716-446655440000"}]}`)
						if method == http.MethodPatch {
							path += "/" + id.String()
							body = []byte(`{"name":"Updated Cap"}`)
						}

						request := httptest.NewRequest(method, path, bytes.NewReader(body))
						request.Header.Set("Content-Type", "application/json")
						response, err := app.Test(request, fiber.TestConfig{Timeout: 0})
						require.NoError(t, err)
						defer response.Body.Close()

						responseBody, err := io.ReadAll(response.Body)
						require.NoError(t, err)
						var got limitPeriodErrorResponse
						require.NoError(t, json.Unmarshal(responseBody, &got), string(responseBody))
						require.Equal(t, tc.status, response.StatusCode)
						require.Equal(t, tc.status, got.Status)
						require.Equal(t, tc.code, got.Code)
						require.Equal(t, tc.title, got.Title)
						require.Equal(t, tc.detail, got.Detail)
					})
				}
			}
		}
	}
}

func TestLimitPeriodErrors_RealCommandsRejectBeforePersistence(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		code   string
		status int
	}{
		{"partial_time_window", map[string]any{"activeTimeStart": "09:00"}, "0438", http.StatusBadRequest},
		{"custom_dates_on_daily_limit", map[string]any{"customStartDate": "2026-03-01T00:00:00Z", "customEndDate": "2026-03-02T00:00:00Z"}, "0443", http.StatusBadRequest},
		{"expired_custom_period", map[string]any{"limitType": "CUSTOM", "customStartDate": "2020-01-01T00:00:00Z", "customEndDate": "2020-01-02T00:00:00Z"}, "0446", http.StatusUnprocessableEntity},
	}

	for _, tc := range tests {
		for _, transport := range []string{"fiber", "huma"} {
			t.Run(tc.name+"/"+transport, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				repository := command.NewMockLimitRepository(ctrl)
				createCommand, err := command.NewCreateLimitCommand(
					repository,
					testutil.NewMockClock(time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)),
					command.NewRecordAuditEventCommand(commandMocks.NewMockAuditEventRepository(ctrl)),
					pgdbMocks.NewMockTxBeginner(ctrl),
				)
				require.NoError(t, err)

				service := NewMockLimitService(ctrl)
				service.EXPECT().CreateLimit(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, input *command.CreateLimitInput) (*model.Limit, error) {
						return createCommand.Execute(ctx, input)
					},
				)

				var body map[string]any
				require.NoError(t, json.Unmarshal(validCreateLimitBody(), &body))
				for key, value := range tc.fields {
					body[key] = value
				}
				rawBody, err := json.Marshal(body)
				require.NoError(t, err)

				app := buildLimitErrorTestApp(t, service, transport)
				request := httptest.NewRequest(http.MethodPost, "/v1/limits", bytes.NewReader(rawBody))
				request.Header.Set("Content-Type", "application/json")
				response, err := app.Test(request, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)
				defer response.Body.Close()

				var got limitPeriodErrorResponse
				require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
				require.Equal(t, tc.status, response.StatusCode)
				require.Equal(t, tc.code, got.Code)
				require.NotEmpty(t, got.Detail)
			})
		}
	}
}

func TestLimitPeriodErrors_RealUpdateCommandRejectsBeforePersistence(t *testing.T) {
	for _, transport := range []string{"fiber", "huma"} {
		t.Run(transport, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			id := testutil.MustDeterministicUUID(701)
			repository := command.NewMockLimitRepository(ctrl)
			repository.EXPECT().GetByID(gomock.Any(), id).Return(validLimit(id), nil)
			updateCommand, err := command.NewUpdateLimitCommand(
				repository,
				testutil.NewMockClock(time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)),
				nil,
				pgdbMocks.NewMockTxBeginner(ctrl),
			)
			require.NoError(t, err)

			service := NewMockLimitService(ctrl)
			service.EXPECT().UpdateLimit(gomock.Any(), id, gomock.Any()).DoAndReturn(
				func(ctx context.Context, id uuid.UUID, input *command.UpdateLimitInput) (*model.Limit, error) {
					return updateCommand.Execute(ctx, id, input)
				},
			)

			app := buildLimitErrorTestApp(t, service, transport)
			request := httptest.NewRequest(http.MethodPatch, "/v1/limits/"+id.String(), bytes.NewBufferString(`{"customStartDate":"invalid"}`))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			defer response.Body.Close()

			var got limitPeriodErrorResponse
			require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			require.Equal(t, "0447", got.Code)
			require.Equal(t, "Invalid customStartDate format, expected RFC3339.", got.Detail)
		})
	}
}

func TestLimitPeriodErrors_PreserveOtherClassificationBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		status     int
		code       string
		title      string
		detail     string
		mustRedact string
	}{
		{
			name:   "typed_business_error_passes_through",
			err:    pkg.ValidateBusinessError(constant.ErrLimitTimeWindowMismatch, constant.EntityLimit),
			status: http.StatusBadRequest,
			code:   "0438",
			title:  "Limit Time Window Mismatch",
			detail: "ActiveTimeStart and activeTimeEnd must both be set or both be nil.",
		},
		{
			name:   "known_conflict_keeps_conflict_status",
			err:    constant.ErrLimitNameAlreadyExists,
			status: http.StatusConflict,
			code:   "0442",
			title:  "Limit Name Already Exists",
			detail: "Limit name already exists.",
		},
		{
			name:       "unknown_technical_error_stays_sanitized_500",
			err:        errors.New("database unavailable: internal host details"),
			status:     http.StatusInternalServerError,
			code:       "0046",
			title:      "Internal Server Error",
			detail:     "internal error",
			mustRedact: "internal host details",
		},
		{
			name:       "time_of_day_decode_error_stays_technical",
			err:        constant.ErrTimeOfDayInvalidFormat,
			status:     http.StatusInternalServerError,
			code:       "0046",
			title:      "Internal Server Error",
			detail:     "internal error",
			mustRedact: constant.ErrTimeOfDayInvalidFormat.Error(),
		},
	}

	for _, tc := range tests {
		for _, transport := range []string{"fiber", "huma"} {
			t.Run(tc.name+"/"+transport, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				service := NewMockLimitService(ctrl)
				service.EXPECT().CreateLimit(gomock.Any(), gomock.Any()).Return(nil, tc.err)
				app := buildLimitErrorTestApp(t, service, transport)

				request := httptest.NewRequest(http.MethodPost, "/v1/limits", bytes.NewReader(validCreateLimitBody()))
				request.Header.Set("Content-Type", "application/json")
				response, err := app.Test(request, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)
				defer response.Body.Close()

				responseBody, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				var got limitPeriodErrorResponse
				require.NoError(t, json.Unmarshal(responseBody, &got), string(responseBody))
				require.Equal(t, tc.status, response.StatusCode)
				require.Equal(t, tc.status, got.Status)
				require.Equal(t, tc.code, got.Code)
				require.Equal(t, tc.title, got.Title)
				require.Equal(t, tc.detail, got.Detail)
				if tc.mustRedact != "" {
					require.NotContains(t, string(responseBody), tc.mustRedact)
				}
			})
		}
	}
}

func TestClassifyLimitServiceError_SpanStatusByErrorClass(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		wantError bool
	}{
		{name: "business_validation", err: constant.ErrLimitTimeWindowMismatch},
		{name: "technical_failure", err: errors.New("database unavailable"), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracing := testutil.SetupTestTracing(t)
			_, span := otel.Tracer("limit-error-test").Start(context.Background(), "limit.error")
			_ = classifyLimitServiceError(span, tc.err)
			span.End()

			spans := tracing.GetSpans()
			require.Len(t, spans, 1)
			if tc.wantError {
				require.Equal(t, codes.Error, spans[0].Status.Code)
				return
			}

			require.NotEqual(t, codes.Error, spans[0].Status.Code)
			require.NotEmpty(t, spans[0].Events, "business validation should add a span event")
		})
	}
}

func buildLimitErrorTestApp(t *testing.T, service LimitService, transport string) *fiber.App {
	t.Helper()

	if transport == "huma" {
		return buildHumaLimitApp(t, service, "")
	}

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	handler := NewLimitHandler(service)
	app.Post("/v1/limits", handler.CreateLimit)
	app.Patch("/v1/limits/:id", handler.UpdateLimit)

	return app
}
