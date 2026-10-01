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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace/noop"
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

// Codes and statuses are literal contract locks; messages come from the canonical
// registry so these tests also check that the adapter preserves its safe text.
var limitPeriodErrorCases = []struct {
	name      string
	body      string
	limitType model.LimitType
	sentinel  error
	code      string
	status    int
}{
	{
		name: "partial_time_window", body: `{"activeTimeStart":"09:00"}`,
		limitType: model.LimitTypeDaily, sentinel: constant.ErrLimitTimeWindowMismatch,
		code: "0438", status: http.StatusBadRequest,
	},
	{
		name: "zero_width_window", body: `{"activeTimeStart":"09:00","activeTimeEnd":"09:00"}`,
		limitType: model.LimitTypeDaily, sentinel: constant.ErrLimitTimeWindowZeroWidth,
		code: "0439", status: http.StatusBadRequest,
	},
	{
		name: "dates_on_daily_limit", body: `{"customStartDate":"2026-03-01T00:00:00Z","customEndDate":"2026-03-02T00:00:00Z"}`,
		limitType: model.LimitTypeDaily, sentinel: constant.ErrLimitCustomDatesNotAllowed,
		code: "0443", status: http.StatusBadRequest,
	},
	{
		name: "period_too_long", body: `{"customStartDate":"2026-03-01T00:00:00Z","customEndDate":"2032-03-01T00:00:00Z"}`,
		limitType: model.LimitTypeCustom, sentinel: constant.ErrLimitCustomPeriodTooLong,
		code: "0445", status: http.StatusUnprocessableEntity,
	},
	{
		name: "expired_period", body: `{"customStartDate":"2020-01-01T00:00:00Z","customEndDate":"2020-01-02T00:00:00Z"}`,
		limitType: model.LimitTypeCustom, sentinel: constant.ErrLimitCustomPeriodExpired,
		code: "0446", status: http.StatusUnprocessableEntity,
	},
	{
		name: "invalid_start_date", body: `{"customStartDate":"invalid","customEndDate":"2026-03-02T00:00:00Z"}`,
		limitType: model.LimitTypeCustom, sentinel: constant.ErrLimitInvalidCustomStartFormat,
		code: "0447", status: http.StatusBadRequest,
	},
	{
		name: "invalid_end_date", body: `{"customStartDate":"2026-03-01T00:00:00Z","customEndDate":"invalid"}`,
		limitType: model.LimitTypeCustom, sentinel: constant.ErrLimitInvalidCustomEndFormat,
		code: "0448", status: http.StatusBadRequest,
	},
	{
		name: "missing_end_date", body: `{"customStartDate":"2026-03-01T00:00:00Z"}`,
		limitType: model.LimitTypeCustom, sentinel: constant.ErrLimitCustomDatesRequired,
		code: "0449", status: http.StatusBadRequest,
	},
	{
		name: "reversed_dates", body: `{"customStartDate":"2026-03-02T00:00:00Z","customEndDate":"2026-03-01T00:00:00Z"}`,
		limitType: model.LimitTypeCustom, sentinel: constant.ErrLimitCustomDatesOrder,
		code: "0450", status: http.StatusBadRequest,
	},
}

func TestLimitPeriodErrors_RealCommands(t *testing.T) {
	// Huma registration changes global hooks; keep these tests serial.
	for _, transport := range []string{"fiber", "huma"} {
		for _, method := range []string{http.MethodPost, http.MethodPatch} {
			for _, tc := range limitPeriodErrorCases {
				t.Run(transport+"/"+method+"/"+tc.name, func(t *testing.T) {
					ctrl := gomock.NewController(t)
					id := testutil.MustDeterministicUUID(700)
					now := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
					repo := command.NewMockLimitRepository(ctrl)
					audit := command.NewRecordAuditEventCommand(commandMocks.NewMockAuditEventRepository(ctrl))
					tx := pgdbMocks.NewMockTxBeginner(ctrl)
					service := NewMockLimitService(ctrl)
					path := "/v1/limits"
					body := []byte(tc.body)

					// No BeginTx, persistence or audit expectations: any write fails the test.
					if method == http.MethodPost {
						cmd, err := command.NewCreateLimitCommand(repo, testutil.NewMockClock(now), audit, tx)
						require.NoError(t, err)
						service.EXPECT().CreateLimit(gomock.Any(), gomock.Any()).DoAndReturn(cmd.Execute)

						var input map[string]any
						require.NoError(t, json.Unmarshal(body, &input))
						input["name"], input["limitType"] = "Period cap", tc.limitType
						input["asset"], input["maxAmount"] = "USD", "1000.00"
						input["scopes"] = []model.Scope{{AccountID: &id}}
						body, err = json.Marshal(input)
						require.NoError(t, err)
					} else {
						start, end := now, now.AddDate(0, 1, 0)
						limit := &model.Limit{
							ID: id, Name: "Period cap", Asset: "USD", LimitType: tc.limitType,
							MaxAmount: decimal.NewFromInt(1000), Status: model.LimitStatusDraft,
							Scopes: []model.Scope{{AccountID: &id}}, CreatedAt: now, UpdatedAt: now,
						}
						if tc.limitType == model.LimitTypeCustom {
							limit.CustomStartDate, limit.CustomEndDate = &start, &end
						}
						repo.EXPECT().GetByID(gomock.Any(), id).Return(limit, nil)
						cmd, err := command.NewUpdateLimitCommand(repo, testutil.NewMockClock(now), audit, tx)
						require.NoError(t, err)
						service.EXPECT().UpdateLimit(gomock.Any(), id, gomock.Any()).DoAndReturn(cmd.Execute)
						path += "/" + id.String()
					}

					got := requestLimitError(t, buildLimitErrorTestApp(t, service, transport), method, path, body)
					want, ok := pkgHTTP.ProblemDetail(pkg.ValidateBusinessError(tc.sentinel, constant.EntityLimit))
					require.True(t, ok, "the registry sentinel must have a canonical HTTP mapping")
					require.Equal(t, tc.status, got.Status)
					require.Equal(t, tc.code, got.Code)
					require.Equal(t, want.Title, got.Title)
					require.Equal(t, want.Detail.Detail, got.Detail.Detail)
				})
			}
		}
	}
}

func TestLimitPeriodErrors_WrappedSentinels(t *testing.T) {
	_, span := noop.NewTracerProvider().Tracer("test").Start(context.Background(), "limit.error")
	defer span.End()

	for _, tc := range limitPeriodErrorCases {
		t.Run(tc.name, func(t *testing.T) {
			err := fmt.Errorf("private parsing context: %w", tc.sentinel)
			got := classifyLimitServiceError(span, err)
			want := pkg.ValidateBusinessError(tc.sentinel, constant.EntityLimit)
			require.Equal(t, want, got, "preserve the registry error, without exposing wrapped context")
		})
	}
}

func TestLimitPeriodErrors_ClassificationBoundaries(t *testing.T) {
	tracing := testutil.SetupTestTracing(t)
	_, span := otel.Tracer("limit-error-test").Start(context.Background(), "limit.error")
	typed := pkg.ValidateBusinessError(constant.ErrLimitTimeWindowMismatch, constant.EntityLimit)
	classified := classifyLimitServiceError(span, typed)
	span.End()

	var want pkg.ValidationError
	require.ErrorAs(t, typed, &want)
	require.Equal(t, want, classified)
	spans := tracing.GetSpans()
	require.Len(t, spans, 1)
	require.Empty(t, spans[0].Events, "typed business errors are passed through without reclassification")
	require.NotEqual(t, codes.Error, spans[0].Status.Code)

	for _, transport := range []string{"fiber", "huma"} {
		for _, tc := range []struct {
			name   string
			err    error
			code   string
			status int
		}{
			{"known_conflict", constant.ErrLimitNameAlreadyExists, "0442", http.StatusConflict},
			{"database_failure", errors.New("private database details"), "0046", http.StatusInternalServerError},
			{"storage_decode_failure", fmt.Errorf("decode: %w", constant.ErrTimeOfDayInvalidFormat), "0046", http.StatusInternalServerError},
		} {
			t.Run(transport+"/"+tc.name, func(t *testing.T) {
				service := NewMockLimitService(gomock.NewController(t))
				service.EXPECT().CreateLimit(gomock.Any(), gomock.Any()).Return(nil, tc.err)
				got := requestLimitError(t, buildLimitErrorTestApp(t, service, transport), http.MethodPost, "/v1/limits", validCreateLimitBody())
				require.Equal(t, tc.status, got.Status)
				require.Equal(t, tc.code, got.Code)
				if tc.status == http.StatusInternalServerError {
					require.Equal(t, "Internal Server Error", got.Title)
					require.Equal(t, "internal error", got.Detail.Detail)
				}
			})
		}
	}
}

func TestClassifyLimitServiceError_SpanStatusByErrorClass(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status codes.Code
	}{
		{"business_validation", constant.ErrLimitTimeWindowMismatch, codes.Unset},
		{"technical_failure", errors.New("database unavailable"), codes.Error},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracing := testutil.SetupTestTracing(t)
			_, span := otel.Tracer("limit-error-test").Start(context.Background(), "limit.error")
			_ = classifyLimitServiceError(span, tc.err)
			span.End()

			spans := tracing.GetSpans()
			require.Len(t, spans, 1)
			require.Equal(t, tc.status, spans[0].Status.Code)
			require.Len(t, spans[0].Events, 1, "record the failure once")
		})
	}
}

func requestLimitError(t *testing.T, app *fiber.App, method, path string, body []byte) pkgHTTP.Detail {
	t.Helper()

	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })

	var got pkgHTTP.Detail
	require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
	require.Equal(t, response.StatusCode, got.Status)
	require.Equal(t, "application/problem+json", response.Header.Get("Content-Type"))
	if got.Status < http.StatusInternalServerError {
		require.Equal(t, constant.EntityLimit, got.EntityType)
	}

	return got
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
