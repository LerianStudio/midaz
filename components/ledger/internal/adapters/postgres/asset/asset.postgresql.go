// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package asset

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libPointers "github.com/LerianStudio/lib-commons/v5/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

var assetColumnList = []string{
	"id",
	"name",
	"type",
	"code",
	"status",
	"status_description",
	"ledger_id",
	"organization_id",
	"created_at",
	"updated_at",
	"deleted_at",
}

// Repository provides an interface for operations related to asset entities.
// It defines methods for creating, finding, updating, and deleting assets in the database.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=asset.postgresql_mock.go --package=asset . Repository
type Repository interface {
	Create(ctx context.Context, asset *mmodel.Asset) (*mmodel.Asset, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Asset, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Asset, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Asset, error)
	FindByNameOrCode(ctx context.Context, organizationID, ledgerID uuid.UUID, name, code string) (bool, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, asset *mmodel.Asset) (*mmodel.Asset, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error)
}

// AssetPostgreSQLRepository is a Postgresql-specific implementation of the AssetRepository.
type AssetPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewAssetPostgreSQLRepository returns a new instance of AssetPostgreSQLRepository using the given Postgres connection.
func NewAssetPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *AssetPostgreSQLRepository {
	c := &AssetPostgreSQLRepository{
		connection: pc,
		tableName:  "asset",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *AssetPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
	// Module-specific connection (from middleware WithModule)
	if db := tmcore.GetPGContext(ctx, constant.ModuleOnboarding); db != nil {
		return db, nil
	}

	// Generic connection fallback (single-module services)
	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if r.requireTenant {
		return nil, fmt.Errorf("tenant postgres connection missing from context")
	}

	if r.connection == nil {
		return nil, fmt.Errorf("postgres connection not available")
	}

	return r.connection.Resolver(ctx)
}

// Create a new asset entity into Postgresql and returns it.
func (r *AssetPostgreSQLRepository) Create(ctx context.Context, asset *mmodel.Asset) (*mmodel.Asset, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_asset")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetPostgreSQLModel{}
	record.FromEntity(asset)

	_, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	insertQuery := `INSERT INTO asset VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING ` + strings.Join(assetColumnList, ", ")

	inserted := &AssetPostgreSQLModel{}

	row := db.QueryRowContext(ctx, insertQuery,
		record.ID,
		record.Name,
		record.Type,
		record.Code,
		record.Status,
		record.StatusDescription,
		record.LedgerID,
		record.OrganizationID,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
	)
	if err := row.Scan(
		&inserted.ID,
		&inserted.Name,
		&inserted.Type,
		&inserted.Code,
		&inserted.Status,
		&inserted.StatusDescription,
		&inserted.LedgerID,
		&inserted.OrganizationID,
		&inserted.CreatedAt,
		&inserted.UpdatedAt,
		&inserted.DeletedAt,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityAsset)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute insert query", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute insert query", err)

		return nil, err
	}

	return inserted.ToEntity(), nil
}

// FindByNameOrCode retrieves Asset entities by name or code from the database.
func (r *AssetPostgreSQLRepository) FindByNameOrCode(ctx context.Context, organizationID, ledgerID uuid.UUID, name, code string) (bool, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset_by_name_or_code")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return false, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_by_name_or_code.query")

	query, args, err := squirrel.Select(assetColumnList...).
		From("asset").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Or{squirrel.Expr("name LIKE ?", name), squirrel.Eq{"code": code}}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		spanQuery.End()

		return false, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrAssetNameOrCodeDuplicate, constant.EntityAsset)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Asset name or code already exists", err)

		return true, err
	}

	return false, nil
}

// FindAll retrieves Asset entities from the database with soft-deleted records.
func (r *AssetPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Asset, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_assets")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	var assets []*mmodel.Asset

	findAll := squirrel.Select(assetColumnList...).
		From(r.tableName).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		OrderBy("id " + strings.ToUpper(filter.SortOrder)).
		Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var asset AssetPostgreSQLModel
		if err := rows.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
			&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			return nil, err
		}

		assets = append(assets, asset.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to scan rows", err)

		return nil, err
	}

	return assets, nil
}

// ListByIDs retrieves Assets entities from the database using the provided IDs.
func (r *AssetPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Asset, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_assets_by_ids")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	var assets []*mmodel.Asset

	_, spanQuery := tracer.Start(ctx, "postgres.list_assets_by_ids.query")

	query, args, err := squirrel.Select(assetColumnList...).
		From("asset").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		spanQuery.End()

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var asset AssetPostgreSQLModel
		if err := rows.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
			&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			return nil, err
		}

		assets = append(assets, asset.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to scan rows", err)

		return nil, err
	}

	return assets, nil
}

// Find retrieves an Asset entity from the database using the provided ID.
func (r *AssetPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Asset, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_asset")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	asset := &AssetPostgreSQLModel{}

	_, spanQuery := tracer.Start(ctx, "postgres.find.query")

	query, args, err := squirrel.Select(assetColumnList...).
		From("asset").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		spanQuery.End()

		return nil, err
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(&asset.ID, &asset.Name, &asset.Type, &asset.Code, &asset.Status, &asset.StatusDescription,
		&asset.LedgerID, &asset.OrganizationID, &asset.CreatedAt, &asset.UpdatedAt, &asset.DeletedAt); err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAsset)
		}

		return nil, err
	}

	return asset.ToEntity(), nil
}

// Update an Asset entity into Postgresql and returns the Asset updated.
func (r *AssetPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, asset *mmodel.Asset) (*mmodel.Asset, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_asset")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AssetPostgreSQLModel{}
	record.FromEntity(asset)

	record.UpdatedAt = time.Now()

	builder := squirrel.Update(r.tableName).
		Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	if asset.Name != "" {
		builder = builder.Set("name", record.Name)
	}

	if !asset.Status.IsEmpty() {
		builder = builder.Set("status", record.Status).
			Set("status_description", record.StatusDescription)
	}

	builder = builder.Suffix("RETURNING " + strings.Join(assetColumnList, ", "))

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build update query", err)

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	updated := &AssetPostgreSQLModel{}

	row := db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&updated.ID,
		&updated.Name,
		&updated.Type,
		&updated.Code,
		&updated.Status,
		&updated.StatusDescription,
		&updated.LedgerID,
		&updated.OrganizationID,
		&updated.CreatedAt,
		&updated.UpdatedAt,
		&updated.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAsset)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to update asset. Rows affected is 0", err)

			return nil, err
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityAsset)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)

		return nil, err
	}

	return updated.ToEntity(), nil
}

// Delete removes an Asset entity from the database using the provided IDs.
func (r *AssetPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_asset")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return err
	}

	_, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE asset SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute delete query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAsset)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete asset. Rows affected is 0", err)

		return err
	}

	return nil
}

// Count retrieves the total count of Asset entities from the database.
func (r *AssetPostgreSQLRepository) Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_assets")
	defer span.End()

	count := int64(0)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return count, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM asset WHERE organization_id = $1 AND ledger_id = $2 AND deleted_at IS NULL",
		organizationID, ledgerID).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		return count, err
	}

	return count, nil
}
