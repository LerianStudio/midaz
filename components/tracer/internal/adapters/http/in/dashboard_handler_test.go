// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	nethttp "net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// dashboardServiceStub records the window it was handed so the handler's only
// real job — turning query parameters into a window — can be asserted.
type dashboardServiceStub struct {
	gotWindow model.DashboardWindow
	calls     int
	err       error
}

func (s *dashboardServiceStub) Metrics(_ context.Context, w model.DashboardWindow) (*model.DashboardMetrics, error) {
	s.gotWindow, s.calls = w, s.calls+1

	if s.err != nil {
		return nil, s.err
	}

	return &model.DashboardMetrics{TransactionsProcessed: 12, AmountSavedByAsset: []model.AssetAmount{}}, nil
}

func (s *dashboardServiceStub) Volume(_ context.Context, w model.DashboardWindow) (*model.DashboardVolume, error) {
	s.gotWindow, s.calls = w, s.calls+1

	if s.err != nil {
		return nil, s.err
	}

	return &model.DashboardVolume{Points: []model.VolumePoint{}}, nil
}

func (s *dashboardServiceStub) FraudTypes(_ context.Context, w model.DashboardWindow) (*model.DashboardFraudTypes, error) {
	s.gotWindow, s.calls = w, s.calls+1

	if s.err != nil {
		return nil, s.err
	}

	return &model.DashboardFraudTypes{Types: []model.FraudTypeSlice{}}, nil
}

// dashboardTestNow is a hardcoded instant: the handler resolves a relative
// period against its clock, so the clock must not move under the assertions.
func dashboardTestNow() time.Time {
	return time.Date(2026, 9, 20, 14, 30, 12, 0, time.UTC)
}

func newDashboardTestHandler(t *testing.T) (*DashboardHandler, *dashboardServiceStub) {
	t.Helper()

	stub := &dashboardServiceStub{}

	return NewDashboardHandler(stub, clock.NewFixedClock(dashboardTestNow())), stub
}

func TestDashboardHandler_DefaultsToThirtyDays(t *testing.T) {
	t.Parallel()

	handler, stub := newDashboardTestHandler(t)

	out, err := handler.GetMetricsHuma(context.Background(), &DashboardWindowInputHuma{})
	require.NoError(t, err)
	require.NotNil(t, out.Body)

	assert.Equal(t, model.DashboardDefaultPeriod, stub.gotWindow.Period)
	assert.Equal(t, 30*24*time.Hour, stub.gotWindow.To.Sub(stub.gotWindow.From))
}

func TestDashboardHandler_HonoursNamedPeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		period string
		want   time.Duration
	}{
		{period: "7d", want: 7 * 24 * time.Hour},
		{period: "30d", want: 30 * 24 * time.Hour},
		{period: "90d", want: 90 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			t.Parallel()

			handler, stub := newDashboardTestHandler(t)

			_, err := handler.GetVolumeHuma(context.Background(), &DashboardWindowInputHuma{Period: tt.period})
			require.NoError(t, err)

			assert.Equal(t, tt.want, stub.gotWindow.To.Sub(stub.gotWindow.From))
			assert.Equal(t, dashboardTestNow().Truncate(time.Minute), stub.gotWindow.To,
				"the window ends at the service clock (truncated to the minute), never time.Now()")
		})
	}
}

// An unusable window is rejected BEFORE the database is touched: the whole
// point of the cap is that an oversized request never becomes a query.
func TestDashboardHandler_RejectsBadWindowWithoutQuerying(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input DashboardWindowInputHuma
	}{
		{name: "unsupported period", input: DashboardWindowInputHuma{Period: "45d"}},
		{
			name: "period together with dates",
			input: DashboardWindowInputHuma{
				Period:    "30d",
				StartDate: "2026-09-01T00:00:00Z",
				EndDate:   "2026-09-10T00:00:00Z",
			},
		},
		{
			name: "window longer than the cap",
			input: DashboardWindowInputHuma{
				StartDate: "2026-01-01T00:00:00Z",
				EndDate:   "2026-09-01T00:00:00Z",
			},
		},
		{name: "start date without end date", input: DashboardWindowInputHuma{StartDate: "2026-09-01T00:00:00Z"}},
		{name: "unparseable date", input: DashboardWindowInputHuma{StartDate: "yesterday", EndDate: "today"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, stub := newDashboardTestHandler(t)

			in := tt.input

			_, err := handler.GetMetricsHuma(context.Background(), &in)
			require.Error(t, err)
			assert.Zero(t, stub.calls, "a rejected window must never reach the service")

			_, volErr := handler.GetVolumeHuma(context.Background(), &in)
			require.Error(t, volErr)

			_, fraudErr := handler.GetFraudTypesHuma(context.Background(), &in)
			require.Error(t, fraudErr)
		})
	}
}

// The rejection carries the canonical Midaz code, not a Huma-native 422.
func TestDashboardHandler_BadWindowCarriesCanonicalCode(t *testing.T) {
	t.Parallel()

	handler, _ := newDashboardTestHandler(t)

	_, err := handler.GetMetricsHuma(context.Background(), &DashboardWindowInputHuma{Period: "1y"})
	require.Error(t, err)

	// Assert on the RFC 9457 body the caller actually receives, not on
	// err.Error(): the human sentence is the detail, the machine-readable
	// contract is the code and the status.
	detail, ok := err.(*pkgHTTP.Detail)
	require.True(t, ok, "a rejected window must render as a problem+json detail, got %T", err)

	assert.Equal(t, constant.ErrInvalidDashboardWindow.Error(), detail.Code,
		"the wire must carry 0498, the code the registry pins")
	assert.Equal(t, nethttp.StatusBadRequest, detail.Status,
		"an unusable window is the caller's error, never a 500")
}

func TestDashboardHandler_ExplicitDateRange(t *testing.T) {
	t.Parallel()

	handler, stub := newDashboardTestHandler(t)

	_, err := handler.GetFraudTypesHuma(context.Background(), &DashboardWindowInputHuma{
		StartDate: "2026-09-01T00:00:00Z",
		EndDate:   "2026-09-15T00:00:00Z",
	})
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), stub.gotWindow.From)
	assert.Equal(t, time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), stub.gotWindow.To)
	assert.Empty(t, stub.gotWindow.Period)
}

// Every read advertises the same caching directive, and it agrees with the
// server-side TTL — a proxy holding longer than the server would serve a window
// the server has already recomputed.
func TestDashboardHandler_AllReadsCarryCacheControl(t *testing.T) {
	t.Parallel()

	handler, _ := newDashboardTestHandler(t)
	ctx := context.Background()

	metrics, err := handler.GetMetricsHuma(ctx, &DashboardWindowInputHuma{})
	require.NoError(t, err)

	volume, err := handler.GetVolumeHuma(ctx, &DashboardWindowInputHuma{})
	require.NoError(t, err)

	fraud, err := handler.GetFraudTypesHuma(ctx, &DashboardWindowInputHuma{})
	require.NoError(t, err)

	assert.Equal(t, dashboardCacheControl, metrics.CacheControl)
	assert.Equal(t, dashboardCacheControl, volume.CacheControl)
	assert.Equal(t, dashboardCacheControl, fraud.CacheControl)
	assert.Equal(t, "private, max-age=60", dashboardCacheControl,
		"private because the figures are one tenant's; 60s because that is the cache TTL")
}

// A service failure surfaces as an error rather than an empty 200 the console
// would render as "zero fraud detected".
func TestDashboardHandler_ServiceErrorSurfaces(t *testing.T) {
	t.Parallel()

	stub := &dashboardServiceStub{err: errors.New("database unavailable")}
	handler := NewDashboardHandler(stub, clock.NewFixedClock(dashboardTestNow()))

	_, err := handler.GetMetricsHuma(context.Background(), &DashboardWindowInputHuma{})
	require.Error(t, err)
}

// A nil service leaves the routes unmounted rather than mounting handlers that
// panic on the first request.
func TestNewDashboardHandlerOrNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newDashboardHandlerOrNil(nil, clock.NewFixedClock(dashboardTestNow())))
	assert.NotNil(t, newDashboardHandlerOrNil(&dashboardServiceStub{}, clock.NewFixedClock(dashboardTestNow())))
}
