// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package segment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v5/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

var segmentColumnList = []string{
	"id",
	"name",
	"ledger_id",
	"organization_id",
	"status",
	"status_description",
	"created_at",
	"updated_at",
	"deleted_at",
}

// Repository provides an interface for operations related to segment entities.
// It defines methods for creating, finding, updating, and deleting segments in the database.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=segment.postgresql_mock.go --package=segment . Repository
type Repository interface {
	// Create persists a new segment and returns the stored entity.
	Create(ctx context.Context, segment *mmodel.Segment) (*mmodel.Segment, error)
	// ExistsByName reports whether a non-deleted segment name already exists in an organization ledger.
	ExistsByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error)
	// FindAll retrieves non-deleted segments for an organization ledger using pagination filters.
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Segment, error)
	// FindByIDs retrieves non-deleted segments matching the provided IDs in an organization ledger.
	FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Segment, error)
	// Find retrieves one non-deleted segment by ID in an organization ledger.
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Segment, error)
	// Update applies mutable fields to a non-deleted segment and returns the updated entity.
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, segment *mmodel.Segment) (*mmodel.Segment, error)
	// Delete soft-deletes a segment by ID in an organization ledger.
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	// Count returns the number of non-deleted segments in an organization ledger.
	Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error)
}

// SegmentPostgreSQLRepository is a Postgresql-specific implementation of the Repository.
type SegmentPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewSegmentPostgreSQLRepository returns a new instance of SegmentPostgreSQLRepository using the given Postgres connection.
func NewSegmentPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *SegmentPostgreSQLRepository {
	c := &SegmentPostgreSQLRepository{
		connection: pc,
		tableName:  "segment",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (p *SegmentPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
	// Module-specific connection (from middleware WithModule)
	if db := tmcore.GetPGContext(ctx, constant.ModuleOnboarding); db != nil {
		return db, nil
	}

	// Generic connection fallback (single-module services)
	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if p.requireTenant {
		return nil, fmt.Errorf("tenant postgres connection missing from context")
	}

	if p.connection == nil {
		return nil, fmt.Errorf("postgres connection not configured")
	}

	return p.connection.Resolver(ctx)
}

func (p *SegmentPostgreSQLRepository) Create(ctx context.Context, segment *mmodel.Segment) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_segment")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db, err := p.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &SegmentPostgreSQLModel{}
	record.FromEntity(segment)

	query, args, err := squirrel.Insert(p.tableName).
		Columns(segmentColumnList...).
		Values(
			record.ID,
			record.Name,
			record.LedgerID,
			record.OrganizationID,
			record.Status,
			record.StatusDescription,
			record.CreatedAt,
			record.UpdatedAt,
			record.DeletedAt,
		).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntitySegment)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute create query", err)
			logger.Log(ctx, libLog.LevelError, "Failed to execute create query", libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute create query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute create query", libLog.Err(err))

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get rows affected", libLog.Err(err))

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntitySegment)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create segment. Rows affected is 0", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to create segment. Rows affected is 0", libLog.Err(err))

		return nil, err
	}

	return record.ToEntity(), nil
}

func (p *SegmentPostgreSQLRepository) ExistsByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.exists_segment_by_name")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return false, err
	}

	db, err := p.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return false, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.exists_segment_by_name.query")
	defer spanQuery.End()

	query, args, err := squirrel.Select(segmentColumnList...).
		From(p.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("name LIKE ?", name)).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return false, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return false, err
	}
	defer rows.Close()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrDuplicateSegmentName, constant.EntitySegment, name, ledgerID)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check segment name existence", err)
		logger.Log(ctx, libLog.LevelWarn, "Segment name already exists")

		return true, err
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to scan rows", err)
		logger.Log(ctx, libLog.LevelError, "Failed to scan rows", libLog.Err(err))

		return false, err
	}

	return false, nil
}

func (p *SegmentPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_segments")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db, err := p.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	var segments []*mmodel.Segment

	findAll := squirrel.Select(segmentColumnList...).
		From(p.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		OrderBy("id " + strings.ToUpper(filter.SortOrder)).
		Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var segment SegmentPostgreSQLModel
		if err := rows.Scan(&segment.ID, &segment.Name, &segment.LedgerID, &segment.OrganizationID,
			&segment.Status, &segment.StatusDescription, &segment.CreatedAt, &segment.UpdatedAt, &segment.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)
			logger.Log(ctx, libLog.LevelError, "Failed to scan row", libLog.Err(err))

			return nil, err
		}

		segments = append(segments, segment.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to scan rows", err)
		logger.Log(ctx, libLog.LevelError, "Failed to scan rows", libLog.Err(err))

		return nil, err
	}

	return segments, nil
}

func (p *SegmentPostgreSQLRepository) FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_segments_by_ids")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db, err := p.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	var segments []*mmodel.Segment

	_, spanQuery := tracer.Start(ctx, "postgres.find_segments_by_ids.query")
	defer spanQuery.End()

	query, args, err := squirrel.Select(segmentColumnList...).
		From(p.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var segment SegmentPostgreSQLModel
		if err := rows.Scan(&segment.ID, &segment.Name, &segment.LedgerID, &segment.OrganizationID,
			&segment.Status, &segment.StatusDescription, &segment.CreatedAt, &segment.UpdatedAt, &segment.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)
			logger.Log(ctx, libLog.LevelError, "Failed to scan row", libLog.Err(err))

			return nil, err
		}

		segments = append(segments, segment.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to scan rows", err)
		logger.Log(ctx, libLog.LevelError, "Failed to scan rows", libLog.Err(err))

		return nil, err
	}

	return segments, nil
}

func (p *SegmentPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_segment")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db, err := p.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	segment := &SegmentPostgreSQLModel{}

	_, spanQuery := tracer.Start(ctx, "postgres.find.query")
	defer spanQuery.End()

	query, args, err := squirrel.Select(segmentColumnList...).
		From(p.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return nil, err
	}

	row := db.QueryRowContext(ctx, query, args...)

	if err := row.Scan(&segment.ID, &segment.Name, &segment.LedgerID, &segment.OrganizationID,
		&segment.Status, &segment.StatusDescription, &segment.CreatedAt, &segment.UpdatedAt, &segment.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntitySegment)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to scan row", err)
			logger.Log(ctx, libLog.LevelWarn, "Segment not found", libLog.Err(err), libLog.String("segment_id", id.String()))

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to scan row", libLog.Err(err), libLog.String("segment_id", id.String()))

		return nil, err
	}

	return segment.ToEntity(), nil
}

func (p *SegmentPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, prd *mmodel.Segment) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_segment")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db, err := p.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &SegmentPostgreSQLModel{}
	record.FromEntity(prd)

	update := squirrel.Update(p.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	if prd.Name != "" {
		update = update.Set("name", record.Name)
	}

	if !prd.Status.IsEmpty() {
		update = update.
			Set("status", record.Status).
			Set("status_description", record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	query, args, err := update.Set("updated_at", record.UpdatedAt).ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err), libLog.String("segment_id", id.String()))

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntitySegment)
			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)
			logger.Log(ctx, libLog.LevelError, "Failed to execute update query", libLog.Err(err), libLog.String("segment_id", id.String()))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute update query", libLog.Err(err), libLog.String("segment_id", id.String()))

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get rows affected", libLog.Err(err), libLog.String("segment_id", id.String()))

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntitySegment)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update segment. Rows affected is 0", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to update segment. Rows affected is 0", libLog.Err(err), libLog.String("segment_id", id.String()))

		return nil, err
	}

	return record.ToEntity(), nil
}

func (p *SegmentPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_segment")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return err
	}

	db, err := p.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return err
	}

	query, args, err := squirrel.Update(p.tableName).
		Set("deleted_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err), libLog.String("segment_id", id.String()))

		return err
	}

	_, spanExec := tracer.Start(ctx, "postgres.delete.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute delete query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute delete query", libLog.Err(err), libLog.String("segment_id", id.String()))

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get rows affected", libLog.Err(err), libLog.String("segment_id", id.String()))

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntitySegment)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete segment. Rows affected is 0", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to delete segment. Rows affected is 0", libLog.Err(err), libLog.String("segment_id", id.String()))

		return err
	}

	return nil
}

func (p *SegmentPostgreSQLRepository) Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_segments")
	defer span.End()

	count := int64(0)
	if err := ctx.Err(); err != nil {
		return count, err
	}

	db, err := p.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return count, err
	}

	query, args, err := squirrel.Select("COUNT(*)").
		From(p.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return count, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return count, err
	}

	return count, nil
}
