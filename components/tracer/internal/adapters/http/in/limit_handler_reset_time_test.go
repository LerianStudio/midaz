// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// commandBackedLimitService serves CreateLimit through the real create-limit
// command so a request body reaches the domain constructor and its validation;
// every other operation keeps the spy behavior.
type commandBackedLimitService struct {
	*tenantSpyLimitService
	create *command.CreateLimitCommand
}

func (s *commandBackedLimitService) CreateLimit(ctx context.Context, input *command.CreateLimitInput) (*model.Limit, error) {
	return s.create.Execute(ctx, input)
}

// limitAuditRecorder counts the limit audit events the create command records
// inside its transaction; the command calls no other AuditWriter method.
type limitAuditRecorder struct {
	command.AuditWriter
	recorded int
}

func (r *limitAuditRecorder) RecordLimitEventWithTx(
	_ context.Context, _ pgdb.DB, _ model.AuditEventType, _ model.AuditAction, _ uuid.UUID, _, _ map[string]any, _ string,
) error {
	r.recorded++
	return nil
}

// newCommandBackedLimitService wires the create command over mocks. When
// persists is true the mocks accept exactly one atomic insert; otherwise no
// persistence call is allowed, so a rejected body that reached the database
// fails the test.
func newCommandBackedLimitService(t *testing.T, persists bool) *commandBackedLimitService {
	t.Helper()

	ctrl := gomock.NewController(t)
	repo := command.NewMockLimitRepository(ctrl)
	audit := &limitAuditRecorder{}
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	if persists {
		tx := pgdbMocks.NewMockTx(ctrl)
		txBeginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil)
		repo.EXPECT().CreateWithTx(gomock.Any(), tx, gomock.Any()).Return(nil)
		tx.EXPECT().Commit().Return(nil)
		tx.EXPECT().Rollback().AnyTimes()
	}

	create, err := command.NewCreateLimitCommand(repo, testutil.NewDefaultMockClock(), audit, txBeginner)
	require.NoError(t, err)

	t.Cleanup(func() {
		want := 0
		if persists {
			want = 1
		}

		assert.Equal(t, want, audit.recorded, "limit audit events recorded")
	})

	return &commandBackedLimitService{tenantSpyLimitService: &tenantSpyLimitService{}, create: create}
}

func createLimitBodyWith(t *testing.T, fields map[string]any) []byte {
	t.Helper()

	body := map[string]any{
		"name":      "Night Pix Cap",
		"limitType": "DAILY",
		"maxAmount": "1000.00",
		"asset":     "BRL",
		"scopes":    []map[string]any{{"accountId": testutil.MustDeterministicUUID(99).String()}},
	}
	for k, v := range fields {
		body[k] = v
	}

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	return raw
}

func doLimitRequest(t *testing.T, app *fiber.App, method, path string, body []byte) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got), "body must be JSON: %s", string(raw))

	return resp.StatusCode, got
}

func TestCreateLimit_ResetTimeAccepted(t *testing.T) {
	tests := []struct {
		name      string
		fields    map[string]any
		wantReset any // nil means the key must be absent
	}{
		{name: "daily with reset time", fields: map[string]any{"resetTime": "09:00"}, wantReset: "09:00"},
		{name: "single-digit hour is normalized", fields: map[string]any{"resetTime": "9:00"}, wantReset: "09:00"},
		{name: "weekly with reset time", fields: map[string]any{"limitType": "WEEKLY", "resetTime": "09:00"}, wantReset: "09:00"},
		{name: "monthly with reset time", fields: map[string]any{"limitType": "MONTHLY", "resetTime": "09:00"}, wantReset: "09:00"},
		{
			name:      "reset time equal to the window end",
			fields:    map[string]any{"activeTimeStart": "23:00", "activeTimeEnd": "09:00", "resetTime": "09:00"},
			wantReset: "09:00",
		},
		{name: "absent reset time", fields: nil, wantReset: nil},
		{name: "null reset time means absent", fields: map[string]any{"resetTime": nil}, wantReset: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := buildHumaLimitApp(t, newCommandBackedLimitService(t, true), "tenant-alpha")

			status, got := doLimitRequest(t, app, http.MethodPost, "/v1/limits", createLimitBodyWith(t, tc.fields))

			require.Equal(t, http.StatusCreated, status, "body: %v", got)

			reset, present := got["resetTime"]
			if tc.wantReset == nil {
				assert.False(t, present, "resetTime must be omitted when absent, got %v", reset)
				return
			}

			assert.Equal(t, tc.wantReset, reset)
		})
	}
}

func TestCreateLimit_ResetTimeRejected(t *testing.T) {
	tests := []struct {
		name     string
		fields   map[string]any
		wantCode string
	}{
		{name: "hour out of range", fields: map[string]any{"resetTime": "24:00"}, wantCode: constant.ErrTimeOfDayInvalidFormat.Error()},
		{name: "minute out of range", fields: map[string]any{"resetTime": "09:60"}, wantCode: constant.ErrTimeOfDayInvalidFormat.Error()},
		{name: "not HH:MM", fields: map[string]any{"resetTime": "9h"}, wantCode: constant.ErrTimeOfDayInvalidFormat.Error()},
		{name: "not a string", fields: map[string]any{"resetTime": 900}, wantCode: constant.ErrTimeOfDayInvalidFormat.Error()},
		{
			name: "custom period",
			fields: map[string]any{
				"limitType":       "CUSTOM",
				"customStartDate": "2026-11-27T00:00:00Z",
				"customEndDate":   "2026-11-29T00:00:00Z",
				"resetTime":       "09:00",
			},
			wantCode: constant.ErrLimitResetTimeNotAllowed.Error(),
		},
		{
			name: "custom period with a time window",
			fields: map[string]any{
				"limitType":       "CUSTOM",
				"customStartDate": "2026-11-27T00:00:00Z",
				"customEndDate":   "2026-11-29T00:00:00Z",
				"activeTimeStart": "10:00",
				"activeTimeEnd":   "12:00",
				"resetTime":       "09:00",
			},
			wantCode: constant.ErrLimitResetTimeNotAllowed.Error(),
		},
		{
			name:     "per transaction",
			fields:   map[string]any{"limitType": "PER_TRANSACTION", "resetTime": "09:00"},
			wantCode: constant.ErrLimitResetTimeNotAllowed.Error(),
		},
		{
			name:     "inside an overnight window",
			fields:   map[string]any{"activeTimeStart": "23:00", "activeTimeEnd": "09:00", "resetTime": "00:00"},
			wantCode: constant.ErrLimitResetTimeInsideWindow.Error(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := buildHumaLimitApp(t, newCommandBackedLimitService(t, false), "tenant-alpha")

			status, got := doLimitRequest(t, app, http.MethodPost, "/v1/limits", createLimitBodyWith(t, tc.fields))

			assert.Equal(t, http.StatusBadRequest, status, "body: %v", got)
			assert.Equal(t, tc.wantCode, got["code"])
		})
	}
}

// A malformed activeTimeStart keeps answering the generic malformed-body code;
// only resetTime carries the specific time-of-day format code.
func TestCreateLimit_MalformedActiveTimeKeepsMalformedBodyCode(t *testing.T) {
	app := buildHumaLimitApp(t, newCommandBackedLimitService(t, false), "tenant-alpha")

	body := createLimitBodyWith(t, map[string]any{"activeTimeStart": "24:00", "activeTimeEnd": "09:00", "resetTime": "09:00"})
	status, got := doLimitRequest(t, app, http.MethodPost, "/v1/limits", body)

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), got["code"])
}

func TestUpdateLimit_ResetTimeWindowConflictIsBadRequest(t *testing.T) {
	svc := &tenantSpyLimitService{updateErr: constant.ErrLimitResetTimeInsideWindow}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	body := []byte(`{"activeTimeStart":"23:00","activeTimeEnd":"09:00"}`)
	status, got := doLimitRequest(t, app, http.MethodPatch, "/v1/limits/"+testutil.MustDeterministicUUID(5).String(), body)

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, constant.ErrLimitResetTimeInsideWindow.Error(), got["code"])
}
