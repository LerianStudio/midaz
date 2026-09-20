// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	nethttp "net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// handlerNow is the injected clock. It carries seconds so the handler's
// floor/ceil snapping is observable in the window the stub receives.
var handlerNow = time.Date(2026, 9, 20, 14, 30, 45, 0, time.UTC)

const (
	validOrg    = "11111111-1111-1111-1111-111111111111"
	validLedger = "22222222-2222-2222-2222-222222222222"
)

// stubDashboardRepo records what the handler asked for and answers a fixed
// body. It is what lets a case assert on the WINDOW the handler derived, which
// is the handler's only real responsibility.
type stubDashboardRepo struct {
	gotOrg    uuid.UUID
	gotLedger uuid.UUID
	gotWindow dashboard.Window
	calls     int
	err       error
}

func (s *stubDashboardRepo) Metrics(_ context.Context, org, ledger uuid.UUID, window dashboard.Window) (*mmodel.DashboardMetrics, error) {
	s.record(org, ledger, window)

	if s.err != nil {
		return nil, s.err
	}

	return &mmodel.DashboardMetrics{
		Total:         3,
		ByStatus:      map[string]int64{constant.APPROVED: 3},
		VolumeByAsset: []mmodel.DashboardAssetVolume{{Asset: "BRL", Amount: decimal.RequireFromString("30.00"), Transactions: 3}},
		WindowStart:   window.From,
		WindowEnd:     window.To,
	}, nil
}

func (s *stubDashboardRepo) Volume(_ context.Context, org, ledger uuid.UUID, window dashboard.Window) (*mmodel.DashboardVolume, error) {
	s.record(org, ledger, window)

	if s.err != nil {
		return nil, s.err
	}

	return &mmodel.DashboardVolume{
		Points:      []mmodel.DashboardVolumePoint{{Date: "2026-09-20", Transactions: 3, ByAsset: nil}},
		WindowStart: window.From,
		WindowEnd:   window.To,
	}, nil
}

func (s *stubDashboardRepo) Assets(_ context.Context, org, ledger uuid.UUID) (*mmodel.DashboardAssets, error) {
	s.record(org, ledger, dashboard.Window{})

	if s.err != nil {
		return nil, s.err
	}

	return &mmodel.DashboardAssets{
		Assets: []mmodel.DashboardAssetPosition{{
			Asset: "BRL", Accounts: 2,
			Available: decimal.RequireFromString("100.00"),
			OnHold:    decimal.Zero,
		}},
	}, nil
}

func (s *stubDashboardRepo) record(org, ledger uuid.UUID, window dashboard.Window) {
	s.calls++
	s.gotOrg = org
	s.gotLedger = ledger
	s.gotWindow = window
}

func newDashboardHandler(stub *stubDashboardRepo) *DashboardHandler {
	return &DashboardHandler{
		Query: &query.UseCase{DashboardRepo: stub},
		Clock: func() time.Time { return handlerNow },
	}
}

func metricsRequest(period, start, end string) *GetDashboardMetricsRequest {
	return &GetDashboardMetricsRequest{
		OrganizationID: validOrg,
		LedgerID:       validLedger,
		Period:         period,
		StartDate:      start,
		EndDate:        end,
	}
}

// =============================================================================
// window derivation
// =============================================================================

func TestDashboardHandler_DefaultsToThirtyDays(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	out, err := handler.GetDashboardMetrics(context.Background(), metricsRequest("", "", ""))
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, dashboard.DefaultPeriod, stub.gotWindow.Period)
	assert.Equal(t, 30*24*time.Hour, stub.gotWindow.To.Sub(stub.gotWindow.From))
}

func TestDashboardHandler_SnapsTheWindowToTheMinute(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	_, err := handler.GetDashboardMetrics(context.Background(), metricsRequest("7d", "", ""))
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 9, 20, 14, 31, 0, 0, time.UTC), stub.gotWindow.To,
		"the end ceils so the most recent minute is never hidden")
	assert.Equal(t, time.Date(2026, 9, 13, 14, 31, 0, 0, time.UTC), stub.gotWindow.From)
}

func TestDashboardHandler_AcceptsEverySupportedPeriod(t *testing.T) {
	for period, length := range map[string]time.Duration{
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
		"90d": 90 * 24 * time.Hour,
	} {
		t.Run(period, func(t *testing.T) {
			stub := &stubDashboardRepo{}
			handler := newDashboardHandler(stub)

			_, err := handler.GetDashboardMetrics(context.Background(), metricsRequest(period, "", ""))
			require.NoError(t, err)

			assert.Equal(t, length, stub.gotWindow.To.Sub(stub.gotWindow.From))
		})
	}
}

func TestDashboardHandler_PassesThePathScopeThrough(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	_, err := handler.GetDashboardMetrics(context.Background(), metricsRequest("", "", ""))
	require.NoError(t, err)

	assert.Equal(t, uuid.MustParse(validOrg), stub.gotOrg)
	assert.Equal(t, uuid.MustParse(validLedger), stub.gotLedger)
}

// =============================================================================
// refusals — the canonical 0498, never a native framework 422
// =============================================================================

func TestDashboardHandler_RefusesUnsupportedPeriod(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	_, err := handler.GetDashboardMetrics(context.Background(), metricsRequest("14d", "", ""))

	require.Error(t, err)
	assert.Equal(t, 0, stub.calls, "a bad window must never reach the database")
	assertDashboardWindowRefusal(t, err)
}

func TestDashboardHandler_RefusesBothForms(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	_, err := handler.GetDashboardMetrics(context.Background(),
		metricsRequest("7d", "2026-09-01T00:00:00Z", "2026-09-10T00:00:00Z"))

	require.Error(t, err)
	assert.Equal(t, 0, stub.calls)
	assertDashboardWindowRefusal(t, err)
}

func TestDashboardHandler_RefusesWindowLongerThanNinetyDays(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	_, err := handler.GetDashboardMetrics(context.Background(),
		metricsRequest("", "2026-01-01T00:00:00Z", "2026-09-01T00:00:00Z"))

	require.Error(t, err)
	assert.Equal(t, 0, stub.calls)
	assertDashboardWindowRefusal(t, err)
}

func TestDashboardHandler_RefusesInvertedRange(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	_, err := handler.GetDashboardMetrics(context.Background(),
		metricsRequest("", "2026-09-10T00:00:00Z", "2026-09-01T00:00:00Z"))

	require.Error(t, err)
	assert.Equal(t, 0, stub.calls)
	assertDashboardWindowRefusal(t, err)
}

func TestDashboardHandler_RefusesBadPathUUID(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	in := metricsRequest("", "", "")
	in.LedgerID = "not-a-uuid"

	_, err := handler.GetDashboardMetrics(context.Background(), in)

	require.Error(t, err)
	assert.Equal(t, 0, stub.calls)
}

// assertDashboardWindowRefusal pins the refusal to the canonical 0498 envelope
// AS A CALLER RECEIVES IT — the rendered problem document, not the internal
// sentinel, because a mapping that loses the code on the way out is exactly the
// failure this guards. The window parameters carry no Huma validation tags
// precisely so this is what a caller sees, rather than a framework 422 nothing
// in the ledger's own catalogue explains.
func assertDashboardWindowRefusal(t *testing.T, err error) {
	t.Helper()

	var detail *pkgHTTP.Detail

	require.ErrorAs(t, err, &detail, "the refusal must render as the canonical problem document")
	assert.Equal(t, constant.ErrInvalidDashboardWindow.Error(), detail.Code)
	assert.Equal(t, constant.EntityDashboard, detail.EntityType)
	assert.Equal(t, nethttp.StatusBadRequest, detail.Status, "an unusable window is a caller error, not a 500")
}

// =============================================================================
// response envelope
// =============================================================================

func TestDashboardHandler_EveryReadCarriesPrivateCacheControl(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	metrics, err := handler.GetDashboardMetrics(context.Background(), metricsRequest("", "", ""))
	require.NoError(t, err)
	assert.Equal(t, "private, max-age=60", metrics.CacheControl)

	volume, err := handler.GetDashboardVolume(context.Background(), &GetDashboardVolumeRequest{
		OrganizationID: validOrg, LedgerID: validLedger,
	})
	require.NoError(t, err)
	assert.Equal(t, "private, max-age=60", volume.CacheControl)

	assets, err := handler.GetDashboardAssets(context.Background(), &GetDashboardAssetsRequest{
		OrganizationID: validOrg, LedgerID: validLedger,
	})
	require.NoError(t, err)
	assert.Equal(t, "private, max-age=60", assets.CacheControl)
}

func TestDashboardHandler_CacheControlMaxAgeMatchesTheServerTTL(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	out, err := handler.GetDashboardMetrics(context.Background(), metricsRequest("", "", ""))
	require.NoError(t, err)

	assert.Contains(t, out.CacheControl, "max-age=60")
	assert.Equal(t, 60*time.Second, dashboard.Granularity,
		"the header and the server-side window granularity must expire together")
}

// TestDashboardHandler_AssetsIgnoresWindowParameters: /assets takes no window
// and documents that it ignores one. A caller that sends period anyway gets its
// position rather than a 400 — refusing would break a console that simply
// forwards its selector to all three reads.
func TestDashboardHandler_AssetsIgnoresWindowParameters(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	out, err := handler.GetDashboardAssets(context.Background(), &GetDashboardAssetsRequest{
		OrganizationID: validOrg, LedgerID: validLedger,
	})

	require.NoError(t, err)
	require.NotNil(t, out.Body)
	assert.Equal(t, 1, stub.calls)
	assert.Len(t, out.Body.Assets, 1)
	assert.True(t, stub.gotWindow.From.IsZero(), "no window is derived for /assets")
}

func TestDashboardHandler_VolumeUsesTheSameWindowRules(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := newDashboardHandler(stub)

	_, err := handler.GetDashboardVolume(context.Background(), &GetDashboardVolumeRequest{
		OrganizationID: validOrg, LedgerID: validLedger, Period: "7d",
	})
	require.NoError(t, err)

	assert.Equal(t, "7d", stub.gotWindow.Period)
	assert.Equal(t, 7*24*time.Hour, stub.gotWindow.To.Sub(stub.gotWindow.From))

	_, err = handler.GetDashboardVolume(context.Background(), &GetDashboardVolumeRequest{
		OrganizationID: validOrg, LedgerID: validLedger, Period: "14d",
	})
	require.Error(t, err)
	assertDashboardWindowRefusal(t, err)
}

func TestDashboardHandler_NilClockUsesRealTime(t *testing.T) {
	stub := &stubDashboardRepo{}
	handler := &DashboardHandler{Query: &query.UseCase{DashboardRepo: stub}}

	before := time.Now().UTC()

	_, err := handler.GetDashboardMetrics(context.Background(), metricsRequest("7d", "", ""))
	require.NoError(t, err)

	assert.False(t, stub.gotWindow.To.Before(before.Truncate(time.Minute)),
		"a handler with no injected clock must still derive a live window")
}

func TestDashboardHandler_RepositoryErrorIsSurfaced(t *testing.T) {
	stub := &stubDashboardRepo{err: pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityDashboard)}
	handler := newDashboardHandler(stub)

	_, err := handler.GetDashboardMetrics(context.Background(), metricsRequest("", "", ""))

	require.Error(t, err)
	assert.Equal(t, 1, stub.calls)
}
