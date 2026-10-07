// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestCreateLimit_DomainValidationErrorsKeepTheirStatus(t *testing.T) {
	tests := []struct {
		name       string
		fields     map[string]any
		wantStatus int
		wantCode   error
	}{
		{
			name:       "time window with only a start",
			fields:     map[string]any{"activeTimeStart": "09:00"},
			wantStatus: http.StatusBadRequest,
			wantCode:   constant.ErrLimitTimeWindowMismatch,
		},
		{
			name:       "time window with only an end",
			fields:     map[string]any{"activeTimeEnd": "17:00"},
			wantStatus: http.StatusBadRequest,
			wantCode:   constant.ErrLimitTimeWindowMismatch,
		},
		{
			name:       "zero-width time window",
			fields:     map[string]any{"activeTimeStart": "09:00", "activeTimeEnd": "09:00"},
			wantStatus: http.StatusBadRequest,
			wantCode:   constant.ErrLimitTimeWindowZeroWidth,
		},
		{
			name:       "custom period with only a start date",
			fields:     map[string]any{"limitType": "CUSTOM", "customStartDate": "2099-01-01T00:00:00Z"},
			wantStatus: http.StatusBadRequest,
			wantCode:   constant.ErrLimitCustomDatesRequired,
		},
		{
			name: "custom period ending before it starts",
			fields: map[string]any{
				"limitType":       "CUSTOM",
				"customStartDate": "2099-01-02T00:00:00Z",
				"customEndDate":   "2099-01-01T00:00:00Z",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   constant.ErrLimitCustomDatesOrder,
		},
		{
			name: "custom period longer than the maximum",
			fields: map[string]any{
				"limitType":       "CUSTOM",
				"customStartDate": "2090-01-01T00:00:00Z",
				"customEndDate":   "2099-01-01T00:00:00Z",
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   constant.ErrLimitCustomPeriodTooLong,
		},
		{
			name: "custom period already over",
			fields: map[string]any{
				"limitType":       "CUSTOM",
				"customStartDate": "2000-01-01T00:00:00Z",
				"customEndDate":   "2000-01-02T00:00:00Z",
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   constant.ErrLimitCustomPeriodExpired,
		},
		{
			name: "custom dates on a non-custom limit",
			fields: map[string]any{
				"customStartDate": "2099-01-01T00:00:00Z",
				"customEndDate":   "2099-01-02T00:00:00Z",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   constant.ErrLimitCustomDatesNotAllowed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := buildHumaLimitApp(t, newCommandBackedLimitService(t, false), "tenant-alpha")

			status, got := doLimitRequest(t, app, http.MethodPost, "/v1/limits", createLimitBodyWith(t, tc.fields))

			assert.Equal(t, tc.wantStatus, status, "body: %v", got)
			assert.Equal(t, tc.wantCode.Error(), got["code"])
		})
	}
}

func TestUpdateLimit_DomainValidationErrorsKeepTheirStatus(t *testing.T) {
	tests := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantCode   error
	}{
		{name: "time window with one side", serviceErr: constant.ErrLimitTimeWindowMismatch, wantStatus: http.StatusBadRequest, wantCode: constant.ErrLimitTimeWindowMismatch},
		{name: "zero-width time window", serviceErr: constant.ErrLimitTimeWindowZeroWidth, wantStatus: http.StatusBadRequest, wantCode: constant.ErrLimitTimeWindowZeroWidth},
		{name: "custom dates required", serviceErr: constant.ErrLimitCustomDatesRequired, wantStatus: http.StatusBadRequest, wantCode: constant.ErrLimitCustomDatesRequired},
		{name: "custom dates order", serviceErr: constant.ErrLimitCustomDatesOrder, wantStatus: http.StatusBadRequest, wantCode: constant.ErrLimitCustomDatesOrder},
		{name: "custom dates not allowed", serviceErr: constant.ErrLimitCustomDatesNotAllowed, wantStatus: http.StatusBadRequest, wantCode: constant.ErrLimitCustomDatesNotAllowed},
		{name: "custom period too long", serviceErr: constant.ErrLimitCustomPeriodTooLong, wantStatus: http.StatusUnprocessableEntity, wantCode: constant.ErrLimitCustomPeriodTooLong},
		{name: "custom period expired", serviceErr: constant.ErrLimitCustomPeriodExpired, wantStatus: http.StatusUnprocessableEntity, wantCode: constant.ErrLimitCustomPeriodExpired},
		{
			name:       "unparseable custom start date",
			serviceErr: fmt.Errorf("%w: %w", constant.ErrLimitInvalidCustomStartFormat, errors.New("parse failure")),
			wantStatus: http.StatusBadRequest,
			wantCode:   constant.ErrLimitInvalidCustomStartFormat,
		},
		{
			name:       "unparseable custom end date",
			serviceErr: fmt.Errorf("%w: %w", constant.ErrLimitInvalidCustomEndFormat, errors.New("parse failure")),
			wantStatus: http.StatusBadRequest,
			wantCode:   constant.ErrLimitInvalidCustomEndFormat,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := buildHumaLimitApp(t, &tenantSpyLimitService{updateErr: tc.serviceErr}, "tenant-alpha")

			body := []byte(`{"activeTimeStart":"09:00","activeTimeEnd":"09:00"}`)
			status, got := doLimitRequest(t, app, http.MethodPatch, "/v1/limits/"+testutil.MustDeterministicUUID(5).String(), body)

			assert.Equal(t, tc.wantStatus, status, "body: %v", got)
			assert.Equal(t, tc.wantCode.Error(), got["code"])
		})
	}
}

// An error the classifier does not recognize stays an opaque 500.
func TestUpdateLimit_UnknownServiceErrorIsInternal(t *testing.T) {
	app := buildHumaLimitApp(t, &tenantSpyLimitService{updateErr: errors.New("connection reset")}, "tenant-alpha")

	status, got := doLimitRequest(t, app, http.MethodPatch, "/v1/limits/"+testutil.MustDeterministicUUID(5).String(), []byte(`{"name":"x"}`))

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, constant.ErrInternalServer.Error(), got["code"])
}
