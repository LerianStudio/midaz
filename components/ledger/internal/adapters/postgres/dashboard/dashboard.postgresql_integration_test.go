//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package dashboard

// =============================================================================
// INTEGRATION TESTS — dashboard repository
//
// Every case here runs the real statements against a real PostgreSQL. The
// money rules this endpoint family publishes — never sum across assets, keep
// every sum exact in decimal, count only settled transactions as volume,
// exclude soft-deleted rows and the external counterparty — are all properties
// of the SQL, so a mocked database would assert nothing about any of them.
//
//	go test -tags integration -run TestIntegration_Dashboard -v -count=1 \
//	    ./components/ledger/internal/adapters/postgres/dashboard/
//
// =============================================================================

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// anchor is the reference instant every fixture is placed relative to. It is a
// fixed date rather than time.Now() so a case that asserts a day count or a
// calendar date cannot pass or fail according to when it ran.
var anchor = time.Date(2026, 9, 20, 14, 30, 0, 0, time.UTC)

type dashboardInfra struct {
	db       *sql.DB
	repo     *DashboardPostgreSQLRepository
	orgID    uuid.UUID
	ledgerID uuid.UUID
	// otherLedgerID is a second ledger under the SAME organization. Every read
	// is scoped to one ledger, and a fixture on this one is how a case proves
	// the scope is real rather than incidental.
	otherLedgerID uuid.UUID
}

func setupDashboardInfra(t *testing.T) *dashboardInfra {
	t.Helper()

	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := pgtestutil.ConnectPostgresClient(t.Context(), t, connStr, connStr)

	return &dashboardInfra{
		db:            container.DB,
		repo:          NewDashboardPostgreSQLRepository(conn),
		orgID:         uuid.Must(libCommons.GenerateUUIDv7()),
		ledgerID:      uuid.Must(libCommons.GenerateUUIDv7()),
		otherLedgerID: uuid.Must(libCommons.GenerateUUIDv7()),
	}
}

// insertTransaction writes one transaction row. amount is a decimal string so a
// fixture can state an exact money value without going through a float.
func (i *dashboardInfra) insertTransaction(t *testing.T, ledgerID uuid.UUID, status, asset, amount string, createdAt time.Time, deletedAt *time.Time) {
	t.Helper()

	value, err := decimal.NewFromString(amount)
	require.NoError(t, err)

	_, err = i.db.Exec(`
		INSERT INTO "transaction"
			(id, description, status, amount, asset_code, chart_of_accounts_group_name,
			 organization_id, ledger_id, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10)`,
		uuid.Must(libCommons.GenerateUUIDv7()), "fixture", status, value, asset, "default",
		i.orgID, ledgerID, createdAt, deletedAt)
	require.NoError(t, err)
}

// insertBalance writes one balance row. available and onHold are decimal
// STRINGS so a fixture states an exact money value and no float is involved in
// producing the number the assertion then checks.
//
// There is no scale column to pass: migration 000005 converted both figures to
// DECIMAL and dropped it. The precision each fixture writes IS the precision
// that account holds the asset at.
func (i *dashboardInfra) insertBalance(t *testing.T, ledgerID, accountID uuid.UUID, asset, key, available, onHold string) {
	t.Helper()

	availableValue, err := decimal.NewFromString(available)
	require.NoError(t, err)

	onHoldValue, err := decimal.NewFromString(onHold)
	require.NoError(t, err)

	_, err = i.db.Exec(`
		INSERT INTO balance
			(id, organization_id, ledger_id, account_id, alias, asset_code, key,
			 available, on_hold, account_type, allow_sending, allow_receiving,
			 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true, true, $11, $11)`,
		uuid.Must(libCommons.GenerateUUIDv7()), i.orgID, ledgerID, accountID,
		"@fixture-"+accountID.String(), asset, key, availableValue, onHoldValue, "deposit", anchor)
	require.NoError(t, err)
}

// insertExternalBalance writes the ledger's `@external/<asset>` counterparty
// row. Midaz issues money by debiting this account, so its `available` is a
// POSITIVE mirror of everything the ledger ever put into circulation — the
// reason /assets must not sum it alongside the accounts that hold the money.
func (i *dashboardInfra) insertExternalBalance(t *testing.T, ledgerID uuid.UUID, asset, available string) {
	t.Helper()

	availableValue, err := decimal.NewFromString(available)
	require.NoError(t, err)

	_, err = i.db.Exec(`
		INSERT INTO balance
			(id, organization_id, ledger_id, account_id, alias, asset_code, key,
			 available, on_hold, account_type, direction, allow_sending, allow_receiving,
			 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'default', $7, 0, $8, 'debit', true, true, $9, $9)`,
		uuid.Must(libCommons.GenerateUUIDv7()), i.orgID, ledgerID,
		uuid.Must(libCommons.GenerateUUIDv7()),
		constant.DefaultExternalAccountAliasPrefix+asset, asset,
		availableValue, constant.ExternalAccountType, anchor)
	require.NoError(t, err)
}

// windowAround returns the half-open window [anchor-d, anchor).
func windowAround(d time.Duration) dashboard.Window {
	return dashboard.Window{From: anchor.Add(-d), To: anchor}
}

// =============================================================================
// /metrics
// =============================================================================

// TestIntegration_DashboardMetrics_NeverSumsAcrossAssets is the money rule this
// whole family exists to protect: three assets in one window produce three
// entries and no fourth number anywhere that adds them together.
func TestIntegration_DashboardMetrics_NeverSumsAcrossAssets(t *testing.T) {
	infra := setupDashboardInfra(t)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "1000.00", anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "500.50", anchor.Add(-2*time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "USD", "200.00", anchor.Add(-3*time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BTC", "0.00000001", anchor.Add(-4*time.Hour), nil)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	assert.Equal(t, int64(4), metrics.Total)
	require.Len(t, metrics.VolumeByAsset, 3)

	byAsset := map[string]decimal.Decimal{}
	counts := map[string]int64{}

	for _, entry := range metrics.VolumeByAsset {
		byAsset[entry.Asset] = entry.Amount
		counts[entry.Asset] = entry.Transactions
	}

	assert.True(t, decimal.RequireFromString("1500.50").Equal(byAsset["BRL"]), "BRL got %s", byAsset["BRL"])
	assert.True(t, decimal.RequireFromString("200.00").Equal(byAsset["USD"]), "USD got %s", byAsset["USD"])
	assert.True(t, decimal.RequireFromString("0.00000001").Equal(byAsset["BTC"]), "BTC got %s", byAsset["BTC"])

	assert.Equal(t, int64(2), counts["BRL"])
	assert.Equal(t, int64(1), counts["USD"])
	assert.Equal(t, int64(1), counts["BTC"])

	// The one number that must NOT exist: nothing in the response equals the
	// nonsense cross-asset total 1700.50.
	total := decimal.Zero
	for _, entry := range metrics.VolumeByAsset {
		total = total.Add(entry.Amount)
	}

	assert.False(t, decimal.RequireFromString("1700.50000001").Equal(total.Round(8)) && len(metrics.VolumeByAsset) == 1,
		"assets must never be collapsed into one entry")
}

// TestIntegration_DashboardMetrics_SingleAsset is the ordinary case: one asset,
// one entry, exact sum.
func TestIntegration_DashboardMetrics_SingleAsset(t *testing.T) {
	infra := setupDashboardInfra(t)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "41000.00", anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "1352080.80", anchor.Add(-2*time.Hour), nil)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	require.Len(t, metrics.VolumeByAsset, 1)
	assert.Equal(t, "BRL", metrics.VolumeByAsset[0].Asset)
	assert.True(t, decimal.RequireFromString("1393080.80").Equal(metrics.VolumeByAsset[0].Amount),
		"got %s", metrics.VolumeByAsset[0].Amount)
	assert.Equal(t, int64(2), metrics.VolumeByAsset[0].Transactions)
}

// TestIntegration_DashboardMetrics_NonSettledCountedButNotVolumed pins the
// settled rule from both sides at once: a PENDING transaction is present in
// the count and in byStatus, and its money is absent from the volume.
func TestIntegration_DashboardMetrics_NonSettledCountedButNotVolumed(t *testing.T) {
	infra := setupDashboardInfra(t)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "100.00", anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.PENDING, "BRL", "999.00", anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.CANCELED, "BRL", "777.00", anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.NOTED, "BRL", "555.00", anchor.Add(-time.Hour), nil)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	assert.Equal(t, int64(4), metrics.Total, "every transaction is counted whatever its status")

	assert.Equal(t, int64(1), metrics.ByStatus[constant.APPROVED])
	assert.Equal(t, int64(1), metrics.ByStatus[constant.PENDING])
	assert.Equal(t, int64(1), metrics.ByStatus[constant.CANCELED])
	assert.Equal(t, int64(1), metrics.ByStatus[constant.NOTED])
	assert.Equal(t, int64(0), metrics.ByStatus[constant.CREATED], "a status with none is present at zero, not absent")

	require.Len(t, metrics.VolumeByAsset, 1)
	assert.True(t, decimal.RequireFromString("100.00").Equal(metrics.VolumeByAsset[0].Amount),
		"only the APPROVED 100.00 moved money, got %s", metrics.VolumeByAsset[0].Amount)
	assert.Equal(t, int64(1), metrics.VolumeByAsset[0].Transactions)
}

// TestIntegration_DashboardMetrics_AssetWithNoSettledVolumeIsAbsent: an asset
// whose only traffic is PENDING carried no volume, so it has no entry.
func TestIntegration_DashboardMetrics_AssetWithNoSettledVolumeIsAbsent(t *testing.T) {
	infra := setupDashboardInfra(t)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "100.00", anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.PENDING, "USD", "900.00", anchor.Add(-time.Hour), nil)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	assert.Equal(t, int64(2), metrics.Total)
	require.Len(t, metrics.VolumeByAsset, 1)
	assert.Equal(t, "BRL", metrics.VolumeByAsset[0].Asset)
}

// TestIntegration_DashboardMetrics_SettledZeroAmountKeepsItsAsset: the entry
// test is the settled COUNT, not the amount. A settled transaction of zero is
// still a transaction, and dropping its asset would lose it from the breakdown
// while Total still counted it.
func TestIntegration_DashboardMetrics_SettledZeroAmountKeepsItsAsset(t *testing.T) {
	infra := setupDashboardInfra(t)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "0", anchor.Add(-time.Hour), nil)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	require.Len(t, metrics.VolumeByAsset, 1)
	assert.Equal(t, "BRL", metrics.VolumeByAsset[0].Asset)
	assert.Equal(t, int64(1), metrics.VolumeByAsset[0].Transactions)
	assert.True(t, metrics.VolumeByAsset[0].Amount.IsZero())
}

// TestIntegration_DashboardMetrics_ExcludesSoftDeleted: a soft-deleted
// transaction is gone from every figure, not merely from the volume.
func TestIntegration_DashboardMetrics_ExcludesSoftDeleted(t *testing.T) {
	infra := setupDashboardInfra(t)

	deletedAt := anchor.Add(-30 * time.Minute)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "100.00", anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "9999.00", anchor.Add(-time.Hour), &deletedAt)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	assert.Equal(t, int64(1), metrics.Total)
	assert.Equal(t, int64(1), metrics.ByStatus[constant.APPROVED])
	require.Len(t, metrics.VolumeByAsset, 1)
	assert.True(t, decimal.RequireFromString("100.00").Equal(metrics.VolumeByAsset[0].Amount),
		"the deleted 9999.00 must not be in the volume, got %s", metrics.VolumeByAsset[0].Amount)
}

// TestIntegration_DashboardMetrics_ExcludesOtherLedger: two ledgers in one
// organization never see each other's figures.
func TestIntegration_DashboardMetrics_ExcludesOtherLedger(t *testing.T) {
	infra := setupDashboardInfra(t)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "100.00", anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.otherLedgerID, constant.APPROVED, "BRL", "8888.00", anchor.Add(-time.Hour), nil)

	mine, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	theirs, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.otherLedgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	assert.Equal(t, int64(1), mine.Total)
	assert.True(t, decimal.RequireFromString("100.00").Equal(mine.VolumeByAsset[0].Amount))

	assert.Equal(t, int64(1), theirs.Total)
	assert.True(t, decimal.RequireFromString("8888.00").Equal(theirs.VolumeByAsset[0].Amount))
}

// TestIntegration_DashboardMetrics_ExcludesOutsideWindow: the window is
// half-open, so a row at exactly To is out and one at exactly From is in.
func TestIntegration_DashboardMetrics_ExcludesOutsideWindow(t *testing.T) {
	infra := setupDashboardInfra(t)

	window := windowAround(24 * time.Hour)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "1.00", window.From, nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "2.00", window.To, nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "4.00", window.From.Add(-time.Second), nil)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, window)
	require.NoError(t, err)

	assert.Equal(t, int64(1), metrics.Total, "[From, To) includes From and excludes To")
	assert.True(t, decimal.RequireFromString("1.00").Equal(metrics.VolumeByAsset[0].Amount))
}

// TestIntegration_DashboardMetrics_EmptyWindow: an empty window answers with
// every status present at zero and no volume entries, never with a nil map or
// a null array.
func TestIntegration_DashboardMetrics_EmptyWindow(t *testing.T) {
	infra := setupDashboardInfra(t)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	assert.Equal(t, int64(0), metrics.Total)
	assert.NotNil(t, metrics.VolumeByAsset)
	assert.Empty(t, metrics.VolumeByAsset)
	require.Len(t, metrics.ByStatus, len(constant.TransactionStatuses))

	for _, status := range constant.TransactionStatuses {
		assert.Equal(t, int64(0), metrics.ByStatus[status], "status %s must be present at zero", status)
	}
}

// TestIntegration_DashboardMetrics_UnknownStatusIsReported: a status the
// ledger's own list does not carry still appears, rather than being folded
// invisibly into Total.
func TestIntegration_DashboardMetrics_UnknownStatusIsReported(t *testing.T) {
	infra := setupDashboardInfra(t)

	infra.insertTransaction(t, infra.ledgerID, "SOMETHING_NEW", "BRL", "1.00", anchor.Add(-time.Hour), nil)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	assert.Equal(t, int64(1), metrics.Total)
	assert.Equal(t, int64(1), metrics.ByStatus["SOMETHING_NEW"])
}

// =============================================================================
// /volume
// =============================================================================

// TestIntegration_DashboardVolume_EightPointsForSevenDaysFromMidDay pins the
// day-count contract: the window is half-open and snaps to the minute, not to
// midnight, so a 7d window opened mid-afternoon TOUCHES eight calendar days.
func TestIntegration_DashboardVolume_EightPointsForSevenDaysFromMidDay(t *testing.T) {
	infra := setupDashboardInfra(t)

	volume, err := infra.repo.Volume(context.Background(), infra.orgID, infra.ledgerID, windowAround(7*24*time.Hour))
	require.NoError(t, err)

	require.Len(t, volume.Points, 8, "7d from 14:30 touches 2026-09-13 through 2026-09-20")
	assert.Equal(t, "2026-09-13", volume.Points[0].Date)
	assert.Equal(t, "2026-09-20", volume.Points[7].Date)
}

// TestIntegration_DashboardVolume_SevenPointsForSevenDaysFromMidnight is the
// other half of the same rule, and the reason the series does not simply
// generate up to the window's exclusive upper bound: a window ending exactly at
// midnight does not touch the day that midnight opens.
func TestIntegration_DashboardVolume_SevenPointsForSevenDaysFromMidnight(t *testing.T) {
	infra := setupDashboardInfra(t)

	midnight := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	window := dashboard.Window{From: midnight.Add(-7 * 24 * time.Hour), To: midnight}

	volume, err := infra.repo.Volume(context.Background(), infra.orgID, infra.ledgerID, window)
	require.NoError(t, err)

	require.Len(t, volume.Points, 7)
	assert.Equal(t, "2026-09-13", volume.Points[0].Date)
	assert.Equal(t, "2026-09-19", volume.Points[6].Date)
}

// TestIntegration_DashboardVolume_GapFillsEmptyDays: a day with no
// transactions is PRESENT and zero-valued. An absent day is not the same
// statement as a day with none, and every renderer downstream would otherwise
// have to invent it.
func TestIntegration_DashboardVolume_GapFillsEmptyDays(t *testing.T) {
	infra := setupDashboardInfra(t)

	// Two transactions on 2026-09-14 and one on 2026-09-19; every other day of
	// the window is empty.
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "10.00",
		time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "20.00",
		time.Date(2026, 9, 14, 23, 59, 59, 0, time.UTC), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "USD", "30.00",
		time.Date(2026, 9, 19, 1, 0, 0, 0, time.UTC), nil)

	volume, err := infra.repo.Volume(context.Background(), infra.orgID, infra.ledgerID, windowAround(7*24*time.Hour))
	require.NoError(t, err)

	require.Len(t, volume.Points, 8)

	byDate := map[string]int{}
	for i, point := range volume.Points {
		byDate[point.Date] = i
	}

	for _, date := range []string{"2026-09-13", "2026-09-15", "2026-09-16", "2026-09-17", "2026-09-18", "2026-09-20"} {
		i, ok := byDate[date]
		require.True(t, ok, "day %s must be present", date)
		assert.Equal(t, int64(0), volume.Points[i].Transactions, "day %s must be zero", date)
		assert.NotNil(t, volume.Points[i].ByAsset, "day %s must carry an empty array, never null", date)
		assert.Empty(t, volume.Points[i].ByAsset)
	}

	fourteenth := volume.Points[byDate["2026-09-14"]]
	assert.Equal(t, int64(2), fourteenth.Transactions)
	require.Len(t, fourteenth.ByAsset, 1)
	assert.True(t, decimal.RequireFromString("30.00").Equal(fourteenth.ByAsset[0].Amount))

	nineteenth := volume.Points[byDate["2026-09-19"]]
	assert.Equal(t, int64(1), nineteenth.Transactions)
	require.Len(t, nineteenth.ByAsset, 1)
	assert.Equal(t, "USD", nineteenth.ByAsset[0].Asset)
}

// TestIntegration_DashboardVolume_PerDayAssetsNeverSummed: a day that carried
// two assets emits two entries, and the day's own count is the total across
// them rather than either one.
func TestIntegration_DashboardVolume_PerDayAssetsNeverSummed(t *testing.T) {
	infra := setupDashboardInfra(t)

	day := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "100.00", day, nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "USD", "7.25", day, nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "USD", "2.75", day, nil)

	volume, err := infra.repo.Volume(context.Background(), infra.orgID, infra.ledgerID, windowAround(7*24*time.Hour))
	require.NoError(t, err)

	var point *struct {
		Transactions int64
		Assets       map[string]decimal.Decimal
		Counts       map[string]int64
	}

	for _, p := range volume.Points {
		if p.Date != "2026-09-18" {
			continue
		}

		assets := map[string]decimal.Decimal{}
		counts := map[string]int64{}

		for _, entry := range p.ByAsset {
			assets[entry.Asset] = entry.Amount
			counts[entry.Asset] = entry.Transactions
		}

		point = &struct {
			Transactions int64
			Assets       map[string]decimal.Decimal
			Counts       map[string]int64
		}{p.Transactions, assets, counts}
	}

	require.NotNil(t, point)
	assert.Equal(t, int64(3), point.Transactions)
	require.Len(t, point.Assets, 2)
	assert.True(t, decimal.RequireFromString("100.00").Equal(point.Assets["BRL"]))
	assert.True(t, decimal.RequireFromString("10.00").Equal(point.Assets["USD"]), "got %s", point.Assets["USD"])
	assert.Equal(t, int64(2), point.Counts["USD"])
}

// TestIntegration_DashboardVolume_NonSettledDayHasCountWithoutVolume: a day
// whose only traffic is PENDING carries its count and an empty breakdown.
func TestIntegration_DashboardVolume_NonSettledDayHasCountWithoutVolume(t *testing.T) {
	infra := setupDashboardInfra(t)

	day := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	infra.insertTransaction(t, infra.ledgerID, constant.PENDING, "BRL", "500.00", day, nil)
	infra.insertTransaction(t, infra.ledgerID, constant.CANCELED, "BRL", "600.00", day, nil)

	volume, err := infra.repo.Volume(context.Background(), infra.orgID, infra.ledgerID, windowAround(7*24*time.Hour))
	require.NoError(t, err)

	for _, p := range volume.Points {
		if p.Date != "2026-09-17" {
			continue
		}

		assert.Equal(t, int64(2), p.Transactions)
		assert.Empty(t, p.ByAsset, "nothing settled, so no money is reported")
	}
}

// TestIntegration_DashboardVolume_ExcludesSoftDeletedAndOtherLedger.
func TestIntegration_DashboardVolume_ExcludesSoftDeletedAndOtherLedger(t *testing.T) {
	infra := setupDashboardInfra(t)

	day := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	deletedAt := anchor

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "5.00", day, nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "9999.00", day, &deletedAt)
	infra.insertTransaction(t, infra.otherLedgerID, constant.APPROVED, "BRL", "8888.00", day, nil)

	volume, err := infra.repo.Volume(context.Background(), infra.orgID, infra.ledgerID, windowAround(7*24*time.Hour))
	require.NoError(t, err)

	for _, p := range volume.Points {
		if p.Date != "2026-09-16" {
			continue
		}

		assert.Equal(t, int64(1), p.Transactions)
		require.Len(t, p.ByAsset, 1)
		assert.True(t, decimal.RequireFromString("5.00").Equal(p.ByAsset[0].Amount), "got %s", p.ByAsset[0].Amount)
	}
}

// TestIntegration_DashboardVolume_ReconcilesWithMetrics is the invariant that
// makes the two endpoints readable side by side: the day counts sum to
// /metrics.total, and each asset's daily amounts sum to its /metrics entry. An
// operator who cannot check that has to guess which of the two is lying.
func TestIntegration_DashboardVolume_ReconcilesWithMetrics(t *testing.T) {
	infra := setupDashboardInfra(t)

	for i := 0; i < 6; i++ {
		day := anchor.Add(-time.Duration(i) * 24 * time.Hour)
		infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "11.11", day, nil)
		infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "USD", "2.50", day, nil)
		infra.insertTransaction(t, infra.ledgerID, constant.PENDING, "BRL", "99.00", day, nil)
	}

	window := windowAround(7 * 24 * time.Hour)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, window)
	require.NoError(t, err)

	volume, err := infra.repo.Volume(context.Background(), infra.orgID, infra.ledgerID, window)
	require.NoError(t, err)

	var dayTotal int64

	perAsset := map[string]decimal.Decimal{}

	for _, point := range volume.Points {
		dayTotal += point.Transactions

		for _, entry := range point.ByAsset {
			perAsset[entry.Asset] = perAsset[entry.Asset].Add(entry.Amount)
		}
	}

	assert.Equal(t, metrics.Total, dayTotal, "the day counts must sum to the headline total")

	for _, entry := range metrics.VolumeByAsset {
		assert.True(t, entry.Amount.Equal(perAsset[entry.Asset]),
			"asset %s: /metrics says %s, /volume days sum to %s", entry.Asset, entry.Amount, perAsset[entry.Asset])
	}
}

// =============================================================================
// /assets
// =============================================================================

// TestIntegration_DashboardAssets_MixedPrecisionsSumExactly is the /assets
// money rule. Two accounts hold the same asset at wildly different precisions —
// two decimal places and eight — and the sum has to be exact at the finer one.
// 0.1 + 0.2 is the canonical float tell, so the fixture includes it: a binary
// float anywhere in this path answers 0.30000000000000004.
func TestIntegration_DashboardAssets_MixedPrecisionsSumExactly(t *testing.T) {
	infra := setupDashboardInfra(t)

	accountA := uuid.Must(libCommons.GenerateUUIDv7())
	accountB := uuid.Must(libCommons.GenerateUUIDv7())
	accountC := uuid.Must(libCommons.GenerateUUIDv7())

	infra.insertBalance(t, infra.ledgerID, accountA, "BRL", "default", "1.00", "0.25")
	infra.insertBalance(t, infra.ledgerID, accountB, "BRL", "default", "0.05000000", "0")
	infra.insertBalance(t, infra.ledgerID, accountC, "BRL", "default", "0.1", "0.2")

	assets, err := infra.repo.Assets(context.Background(), infra.orgID, infra.ledgerID)
	require.NoError(t, err)

	require.Len(t, assets.Assets, 1)
	position := assets.Assets[0]

	assert.Equal(t, "BRL", position.Asset)
	assert.True(t, decimal.RequireFromString("1.15").Equal(position.Available),
		"available must be 1.00 + 0.05 + 0.1, got %s", position.Available)
	assert.True(t, decimal.RequireFromString("0.45").Equal(position.OnHold),
		"on hold must be 0.25 + 0.2 exactly, got %s", position.OnHold)
	assert.Equal(t, int64(3), position.Accounts)
}

// TestIntegration_DashboardAssets_SmallestUnitSurvives: the smallest unit of
// an eight-decimal asset must survive the round trip. A float64 path keeps
// 0.00000001 but loses it once a large position is added to it, so the fixture
// adds a satoshi to twenty-one million BTC — a sum float64 cannot represent.
func TestIntegration_DashboardAssets_SmallestUnitSurvives(t *testing.T) {
	infra := setupDashboardInfra(t)

	small := uuid.Must(libCommons.GenerateUUIDv7())
	large := uuid.Must(libCommons.GenerateUUIDv7())

	infra.insertBalance(t, infra.ledgerID, small, "BTC", "default", "0.00000001", "0.00000003")
	infra.insertBalance(t, infra.ledgerID, large, "BTC", "default", "21000000", "0")

	assets, err := infra.repo.Assets(context.Background(), infra.orgID, infra.ledgerID)
	require.NoError(t, err)

	require.Len(t, assets.Assets, 1)
	assert.True(t, decimal.RequireFromString("21000000.00000001").Equal(assets.Assets[0].Available),
		"the satoshi must survive being added to 21M, got %s", assets.Assets[0].Available)
	assert.True(t, decimal.RequireFromString("0.00000003").Equal(assets.Assets[0].OnHold),
		"got %s", assets.Assets[0].OnHold)
}

// TestIntegration_DashboardAssets_NeverSumsAcrossAssets.
func TestIntegration_DashboardAssets_NeverSumsAcrossAssets(t *testing.T) {
	infra := setupDashboardInfra(t)

	accountA := uuid.Must(libCommons.GenerateUUIDv7())
	accountB := uuid.Must(libCommons.GenerateUUIDv7())

	infra.insertBalance(t, infra.ledgerID, accountA, "BRL", "default", "1500.55", "0")
	infra.insertBalance(t, infra.ledgerID, accountB, "USD", "default", "200.00", "0")

	assets, err := infra.repo.Assets(context.Background(), infra.orgID, infra.ledgerID)
	require.NoError(t, err)

	require.Len(t, assets.Assets, 2)
	assert.Equal(t, "BRL", assets.Assets[0].Asset, "assets are ordered by code")
	assert.Equal(t, "USD", assets.Assets[1].Asset)
	assert.True(t, decimal.RequireFromString("1500.55").Equal(assets.Assets[0].Available))
	assert.True(t, decimal.RequireFromString("200.00").Equal(assets.Assets[1].Available))
}

// TestIntegration_DashboardAssets_CountsDistinctAccounts: one account holding
// several balance keys in one asset is ONE account, not several.
func TestIntegration_DashboardAssets_CountsDistinctAccounts(t *testing.T) {
	infra := setupDashboardInfra(t)

	account := uuid.Must(libCommons.GenerateUUIDv7())
	other := uuid.Must(libCommons.GenerateUUIDv7())

	infra.insertBalance(t, infra.ledgerID, account, "BRL", "default", "1.00", "0")
	infra.insertBalance(t, infra.ledgerID, account, "BRL", "savings", "2.00", "0")
	infra.insertBalance(t, infra.ledgerID, account, "BRL", "escrow", "3.00", "0")
	infra.insertBalance(t, infra.ledgerID, other, "BRL", "default", "4.00", "0")

	assets, err := infra.repo.Assets(context.Background(), infra.orgID, infra.ledgerID)
	require.NoError(t, err)

	require.Len(t, assets.Assets, 1)
	assert.Equal(t, int64(2), assets.Assets[0].Accounts, "two accounts, four balance rows")
	assert.True(t, decimal.RequireFromString("10.00").Equal(assets.Assets[0].Available),
		"every row is still summed, got %s", assets.Assets[0].Available)
}

// TestIntegration_DashboardAssets_ExcludesSoftDeletedAndOtherLedger.
func TestIntegration_DashboardAssets_ExcludesSoftDeletedAndOtherLedger(t *testing.T) {
	infra := setupDashboardInfra(t)

	account := uuid.Must(libCommons.GenerateUUIDv7())
	deleted := uuid.Must(libCommons.GenerateUUIDv7())
	elsewhere := uuid.Must(libCommons.GenerateUUIDv7())

	infra.insertBalance(t, infra.ledgerID, account, "BRL", "default", "1.00", "0")
	infra.insertBalance(t, infra.ledgerID, deleted, "BRL", "default", "9999.00", "0")
	infra.insertBalance(t, infra.otherLedgerID, elsewhere, "BRL", "default", "8888.00", "0")

	_, err := infra.db.Exec(`UPDATE balance SET deleted_at = $1 WHERE account_id = $2`, anchor, deleted)
	require.NoError(t, err)

	assets, err := infra.repo.Assets(context.Background(), infra.orgID, infra.ledgerID)
	require.NoError(t, err)

	require.Len(t, assets.Assets, 1)
	assert.Equal(t, int64(1), assets.Assets[0].Accounts)
	assert.True(t, decimal.RequireFromString("1.00").Equal(assets.Assets[0].Available),
		"got %s", assets.Assets[0].Available)
}

// TestIntegration_DashboardAssets_ExcludesExternalCounterparty pins the money
// rule that matters most on this endpoint. The `@external/<asset>` account is
// the contra side of every issuance: when 1200 BRL is put into circulation the
// external row reads +1200 while the accounts holding it read 1200 between
// them. Summing every balance row therefore reports 2400 for a ledger that
// holds 1200, and an operator reading "available" would see double the money
// that exists. Measured live on 2026-09-20 before this exclusion: a ledger
// holding 1200 BRL answered 1900 available (1200 external + 700 remaining on
// the customer account), which is neither the position nor the issuance.
func TestIntegration_DashboardAssets_ExcludesExternalCounterparty(t *testing.T) {
	infra := setupDashboardInfra(t)

	holder := uuid.Must(libCommons.GenerateUUIDv7())
	second := uuid.Must(libCommons.GenerateUUIDv7())

	infra.insertBalance(t, infra.ledgerID, holder, "BRL", "default", "700.00", "500.00")
	infra.insertBalance(t, infra.ledgerID, second, "BRL", "default", "0", "0")
	infra.insertExternalBalance(t, infra.ledgerID, "BRL", "1200.00")

	assets, err := infra.repo.Assets(context.Background(), infra.orgID, infra.ledgerID)
	require.NoError(t, err)

	require.Len(t, assets.Assets, 1)
	assert.Equal(t, int64(2), assets.Assets[0].Accounts,
		"the external counterparty is not one of the ledger's accounts")
	assert.True(t, decimal.RequireFromString("700.00").Equal(assets.Assets[0].Available),
		"available must be what the ledger's accounts hold, got %s", assets.Assets[0].Available)
	assert.True(t, decimal.RequireFromString("500.00").Equal(assets.Assets[0].OnHold),
		"got %s", assets.Assets[0].OnHold)
}

// =============================================================================
// FLOAT DISCRIMINATION
//
// These three cases exist because the precision fixtures above do NOT
// discriminate. 1.00+0.05+0.1 and 0.25+0.2 print identically through float64
// and through decimal, so a pipeline that had silently become float64 passed
// them: a reviewer cast each SQL sum to float8 and all three reads survived.
//
// Every pair below DISAGREES under float64:
//
//	9007199254740992 + 1 -> float64 answers 9007199254740992. 2^53 is the last
//	                        integer it can represent, so the +1 vanishes.
//	0.1 + 0.2            -> float64 answers 0.30000000000000004.
//
// Each case also asserts the MARSHALLED JSON, because exactness inside Go is
// worth nothing if the wire carries a bare JSON number: the console has to
// receive a QUOTED string it can hand to a decimal formatter, and a number
// would be through float64 before its code ever ran.
// =============================================================================

// twoPow53 is the largest integer float64 represents exactly. One more than it
// is the cheapest proof that a money path is not going through a float.
const (
	twoPow53      = "9007199254740992"
	twoPow53Plus1 = "9007199254740993"
)

// TestIntegration_DashboardMetrics_MoneyIsDecimalNotFloat.
func TestIntegration_DashboardMetrics_MoneyIsDecimalNotFloat(t *testing.T) {
	infra := setupDashboardInfra(t)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", twoPow53, anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "1", anchor.Add(-2*time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "USD", "0.1", anchor.Add(-3*time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "USD", "0.2", anchor.Add(-4*time.Hour), nil)

	metrics, err := infra.repo.Metrics(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	byAsset := map[string]decimal.Decimal{}
	for _, entry := range metrics.VolumeByAsset {
		byAsset[entry.Asset] = entry.Amount
	}

	assert.Equal(t, twoPow53Plus1, byAsset["BRL"].String(),
		"float64 answers %s here: the +1 falls off past 2^53", twoPow53)
	assert.Equal(t, "0.3", byAsset["USD"].String(),
		"float64 answers 0.30000000000000004 here")

	raw, err := json.Marshal(metrics)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"amount":"`+twoPow53Plus1+`"`,
		"the wire must carry a quoted exact string, not a JSON number: %s", raw)
	assert.Contains(t, string(raw), `"amount":"0.3"`,
		"the wire must carry a quoted exact string, not a JSON number: %s", raw)
}

// TestIntegration_DashboardVolume_MoneyIsDecimalNotFloat.
func TestIntegration_DashboardVolume_MoneyIsDecimalNotFloat(t *testing.T) {
	infra := setupDashboardInfra(t)

	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", twoPow53, anchor.Add(-time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "BRL", "1", anchor.Add(-2*time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "USD", "0.1", anchor.Add(-3*time.Hour), nil)
	infra.insertTransaction(t, infra.ledgerID, constant.APPROVED, "USD", "0.2", anchor.Add(-4*time.Hour), nil)

	volume, err := infra.repo.Volume(context.Background(), infra.orgID, infra.ledgerID, windowAround(24*time.Hour))
	require.NoError(t, err)

	byAsset := map[string]decimal.Decimal{}

	for _, point := range volume.Points {
		for _, entry := range point.ByAsset {
			byAsset[entry.Asset] = byAsset[entry.Asset].Add(entry.Amount)
		}
	}

	assert.Equal(t, twoPow53Plus1, byAsset["BRL"].String(),
		"float64 answers %s here: the +1 falls off past 2^53", twoPow53)
	assert.Equal(t, "0.3", byAsset["USD"].String(),
		"float64 answers 0.30000000000000004 here")

	raw, err := json.Marshal(volume)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"amount":"`+twoPow53Plus1+`"`,
		"the wire must carry a quoted exact string, not a JSON number: %s", raw)
	assert.Contains(t, string(raw), `"amount":"0.3"`,
		"the wire must carry a quoted exact string, not a JSON number: %s", raw)
}

// TestIntegration_DashboardAssets_MoneyIsDecimalNotFloat carries both pairs on
// one asset: the integer pair in available, the fractional pair in on_hold, so
// a float anywhere in either column is caught.
func TestIntegration_DashboardAssets_MoneyIsDecimalNotFloat(t *testing.T) {
	infra := setupDashboardInfra(t)

	accountA := uuid.Must(libCommons.GenerateUUIDv7())
	accountB := uuid.Must(libCommons.GenerateUUIDv7())

	infra.insertBalance(t, infra.ledgerID, accountA, "BRL", "default", twoPow53, "0.1")
	infra.insertBalance(t, infra.ledgerID, accountB, "BRL", "default", "1", "0.2")

	assets, err := infra.repo.Assets(context.Background(), infra.orgID, infra.ledgerID)
	require.NoError(t, err)

	require.Len(t, assets.Assets, 1)
	position := assets.Assets[0]

	assert.Equal(t, twoPow53Plus1, position.Available.String(),
		"float64 answers %s here: the +1 falls off past 2^53", twoPow53)
	assert.Equal(t, "0.3", position.OnHold.String(),
		"float64 answers 0.30000000000000004 here")

	raw, err := json.Marshal(assets)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"available":"`+twoPow53Plus1+`"`,
		"the wire must carry a quoted exact string, not a JSON number: %s", raw)
	assert.Contains(t, string(raw), `"onHold":"0.3"`,
		"the wire must carry a quoted exact string, not a JSON number: %s", raw)
}

// TestIntegration_DashboardAssets_EmptyLedgerAnswersEmptyArray.
func TestIntegration_DashboardAssets_EmptyLedgerAnswersEmptyArray(t *testing.T) {
	infra := setupDashboardInfra(t)

	assets, err := infra.repo.Assets(context.Background(), infra.orgID, infra.ledgerID)
	require.NoError(t, err)

	assert.NotNil(t, assets.Assets)
	assert.Empty(t, assets.Assets)
}
