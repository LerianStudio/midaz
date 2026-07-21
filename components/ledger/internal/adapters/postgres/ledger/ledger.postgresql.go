// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libPointers "github.com/LerianStudio/lib-commons/v5/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
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

var ledgerColumnList = []string{
	"id",
	"name",
	"organization_id",
	"status",
	"status_description",
	"created_at",
	"updated_at",
	"deleted_at",
	"settings",
}

// Repository provides an interface for operations related to ledger entities.
// It defines methods for creating, finding, updating, and deleting ledgers in the database.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=ledger.postgresql_mock.go --package=ledger . Repository
type Repository interface {
	// Create inserts a new Ledger and returns the persisted row scanned from
	// RETURNING (the source of truth — not the input struct). Maps Postgres
	// constraint violations to business errors via services.ValidatePGError.
	Create(ctx context.Context, ledger *mmodel.Ledger) (*mmodel.Ledger, error)

	// Find retrieves a single Ledger by organization and ID. Excludes
	// soft-deleted rows (deleted_at IS NULL). Returns ErrEntityNotFound
	// (mapped from sql.ErrNoRows) when no matching active row exists.
	Find(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error)

	// FindAll returns a paginated list of active Ledgers for the organization,
	// honoring the filter's date range, sort order, optional EntityIDs, name
	// prefix, and status. Soft-deleted rows are excluded.
	FindAll(ctx context.Context, organizationID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Ledger, error)

	// FindByName reports whether an active Ledger with the given name exists
	// in the organization. Returns (true, ErrLedgerNameConflict) when found,
	// (false, nil) when not found. The boolean answers "does it exist?" — true
	// is the conflict signal callers use to short-circuit creation.
	FindByName(ctx context.Context, organizationID uuid.UUID, name string) (bool, error)

	// ListByIDs returns active Ledgers in the organization whose IDs are in
	// the provided slice, ordered by created_at DESC. Returns an empty slice
	// (not an error) when ids is empty. Soft-deleted rows are excluded.
	ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Ledger, error)

	// Update applies mutable fields (Name, Status) to an active Ledger and
	// returns the persisted row scanned from RETURNING. Only operates on rows
	// where deleted_at IS NULL. Does NOT touch settings — use UpdateSettings /
	// ReplaceSettings / UpdateSettingsAtomic for that. Returns ErrEntityNotFound
	// (mapped from sql.ErrNoRows) when no matching active row exists.
	Update(ctx context.Context, organizationID, id uuid.UUID, ledger *mmodel.Ledger) (*mmodel.Ledger, error)

	// Delete soft-deletes a Ledger by setting deleted_at = now() on rows where
	// deleted_at IS NULL. Returns ErrEntityNotFound when no active row matches.
	Delete(ctx context.Context, organizationID, id uuid.UUID) error

	// Count returns the number of active (deleted_at IS NULL) Ledgers in the
	// organization.
	Count(ctx context.Context, organizationID uuid.UUID) (int64, error)

	// GetSettings returns the settings JSONB of an active Ledger as a map.
	// Returns an empty (non-nil) map when settings are absent or empty.
	// Returns ErrEntityNotFound when the ledger does not exist or is soft-deleted.
	GetSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (map[string]any, error)

	// UpdateSettings merges the provided settings into the ledger's existing
	// settings JSONB and returns the merged result. Merge is shallow at the
	// top level — nested objects are replaced, not deep-merged. Bumps
	// updated_at. Returns ErrEntityNotFound when the ledger is absent or
	// soft-deleted.
	UpdateSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error)

	// ReplaceSettings overwrites the ledger's settings JSONB with the provided
	// map (no merge). Bumps updated_at. Intended for callers that have already
	// performed application-level merging/validation. Returns ErrEntityNotFound
	// when the ledger is absent or soft-deleted.
	ReplaceSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error)

	// UpdateSettingsAtomic performs a read-modify-write under SELECT FOR UPDATE,
	// invoking mergeFn with the current settings and persisting its result.
	// Prevents lost updates under concurrent PATCH requests. mergeFn must be
	// non-nil; returns ErrBadRequest otherwise. Returns ErrEntityNotFound when
	// the ledger is absent or soft-deleted.
	UpdateSettingsAtomic(ctx context.Context, organizationID, ledgerID uuid.UUID, mergeFn func(existing map[string]any) (map[string]any, error)) (map[string]any, error)
}

// LedgerPostgreSQLRepository is a Postgresql-specific implementation of the LedgerRepository.
type LedgerPostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewLedgerPostgreSQLRepository returns a new instance of LedgerPostgresRepository using the given Postgres connection.
func NewLedgerPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *LedgerPostgreSQLRepository {
	c := &LedgerPostgreSQLRepository{
		connection: pc,
		tableName:  "ledger",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *LedgerPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
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

func (r *LedgerPostgreSQLRepository) Create(ctx context.Context, ledger *mmodel.Ledger) (*mmodel.Ledger, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_ledger")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	record := &LedgerPostgreSQLModel{}
	record.FromEntity(ledger)

	// Ensure settings is not nil to avoid inserting JSON null
	if record.Settings == nil {
		record.Settings = make(map[string]any)
	}

	settingsJSON, err := json.Marshal(record.Settings)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal settings", err)
		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	// NOTE (v3.5.4 backport): explicit columns keep this INSERT working when future
	// migrations add columns to ledger. Do not collapse this to table-wide VALUES.
	ledgerColumns := strings.Join(ledgerColumnList, ", ")
	insertQuery := fmt.Sprintf(
		`INSERT INTO ledger (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING %s`,
		ledgerColumns, ledgerColumns,
	)

	inserted := &LedgerPostgreSQLModel{}

	var insertedSettingsJSON []byte

	row := db.QueryRowContext(ctx, insertQuery,
		record.ID,
		record.Name,
		record.OrganizationID,
		record.Status,
		record.StatusDescription,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
		settingsJSON,
	)
	if err := row.Scan(
		&inserted.ID,
		&inserted.Name,
		&inserted.OrganizationID,
		&inserted.Status,
		&inserted.StatusDescription,
		&inserted.CreatedAt,
		&inserted.UpdatedAt,
		&inserted.DeletedAt,
		&insertedSettingsJSON,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr != nil {
			err := services.ValidatePGError(pgErr, constant.EntityLedger)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)

		return nil, err
	}

	if len(insertedSettingsJSON) > 0 {
		if err := json.Unmarshal(insertedSettingsJSON, &inserted.Settings); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal settings", err)
			return nil, err
		}
	}

	return inserted.ToEntity(), nil
}

func (r *LedgerPostgreSQLRepository) Find(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_ledger")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	ledger := &LedgerPostgreSQLModel{}

	_, spanQuery := tracer.Start(ctx, "postgres.find.query")

	query, args, err := squirrel.Select(ledgerColumnList...).
		From("ledger").
		Where(squirrel.Eq{"organization_id": organizationID}).
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

	var settingsJSON []byte
	if err := row.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
		&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt, &settingsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to scan row", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		return nil, err
	}

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &ledger.Settings); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal settings", err)
			return nil, err
		}
	}

	return ledger.ToEntity(), nil
}

func (r *LedgerPostgreSQLRepository) FindAll(ctx context.Context, organizationID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Ledger, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_ledgers")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	pagination := filter.ToOffsetPagination()

	var ledgers []*mmodel.Ledger

	findAll := squirrel.Select(ledgerColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Eq{"deleted_at": nil})

	if !pagination.StartDate.IsZero() {
		findAll = findAll.
			Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(pagination.StartDate, libPointers.Int(0), false)}).
			Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(pagination.EndDate, libPointers.Int(0), true)})
	}

	findAll = findAll.OrderBy("id " + strings.ToUpper(pagination.SortOrder)).
		Limit(libCommons.SafeIntToUint64(pagination.Limit)).
		Offset(libCommons.SafeIntToUint64((pagination.Page - 1) * pagination.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	// Filter by entity IDs when provided (metadata composition)
	if len(filter.EntityIDs) > 0 {
		findAll = findAll.Where(squirrel.Expr("id = ANY(?)", pq.Array(filter.EntityIDs)))
	}

	if filter.Name != nil && *filter.Name != "" {
		sanitized := http.EscapeSearchMetacharacters(*filter.Name)
		findAll = findAll.Where(
			squirrel.Expr("lower(name) LIKE lower(?) || '%' ESCAPE '\\'", sanitized),
		)
	}

	if !libCommons.IsNilOrEmpty(filter.Status) {
		findAll = findAll.Where(squirrel.Expr("status = ?", *filter.Status))
	}

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to query database", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel

		var settingsJSON []byte
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt, &settingsJSON); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			return nil, err
		}

		if len(settingsJSON) > 0 {
			if err := json.Unmarshal(settingsJSON, &ledger.Settings); err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to unmarshal settings", err)
				return nil, err
			}
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		return nil, err
	}

	return ledgers, nil
}

func (r *LedgerPostgreSQLRepository) FindByName(ctx context.Context, organizationID uuid.UUID, name string) (bool, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_ledger_by_name")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return false, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.find_by_name.query")

	query, args, err := squirrel.Select(ledgerColumnList...).
		From("ledger").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Expr("LOWER(name) LIKE LOWER(?)", name)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to build query", err)

		spanQuery.End()

		return false, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to query database", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, constant.EntityLedger, name)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Ledger name conflict", err)

		return true, err
	}

	return false, nil
}

func (r *LedgerPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Ledger, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_ledgers_by_ids")
	defer span.End()

	if len(ids) == 0 {
		return []*mmodel.Ledger{}, nil
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	var ledgers []*mmodel.Ledger

	_, spanQuery := tracer.Start(ctx, "postgres.list_ledgers_by_ids.query")

	query, args, err := squirrel.Select(ledgerColumnList...).
		From("ledger").
		Where(squirrel.Eq{"organization_id": organizationID}).
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
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to query database", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel

		var settingsJSON []byte
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt, &settingsJSON); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			return nil, err
		}

		if len(settingsJSON) > 0 {
			if err := json.Unmarshal(settingsJSON, &ledger.Settings); err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to unmarshal settings", err)
				return nil, err
			}
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows", err)

		return nil, err
	}

	return ledgers, nil
}

func (r *LedgerPostgreSQLRepository) Update(ctx context.Context, organizationID, id uuid.UUID, ledger *mmodel.Ledger) (*mmodel.Ledger, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_ledger")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	record := &LedgerPostgreSQLModel{}
	record.FromEntity(ledger)

	record.UpdatedAt = time.Now()

	builder := squirrel.Update(r.tableName).
		Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	if ledger.Name != "" {
		builder = builder.Set("name", record.Name)
	}

	if !ledger.Status.IsEmpty() {
		builder = builder.Set("status", record.Status).
			Set("status_description", record.StatusDescription)
	}

	builder = builder.Suffix("RETURNING " + strings.Join(ledgerColumnList, ", "))

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build update query", err)

		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	updated := &LedgerPostgreSQLModel{}

	var settingsJSON []byte

	row := db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&updated.ID,
		&updated.Name,
		&updated.OrganizationID,
		&updated.Status,
		&updated.StatusDescription,
		&updated.CreatedAt,
		&updated.UpdatedAt,
		&updated.DeletedAt,
		&settingsJSON,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to update ledger. Rows affected is 0", err)

			return nil, err
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityLedger)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)

		return nil, err
	}

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &updated.Settings); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal settings", err)
			return nil, err
		}
	}

	return updated.ToEntity(), nil
}

func (r *LedgerPostgreSQLRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_ledger")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return err
	}

	_, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE ledger SET deleted_at = now() WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`, organizationID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute database query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete ledger. Rows affected is 0", err)

		return err
	}

	return nil
}

func (r *LedgerPostgreSQLRepository) Count(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_ledgers")
	defer span.End()

	count := int64(0)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return count, err
	}

	_, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ledger WHERE organization_id = $1 AND deleted_at IS NULL", organizationID).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to query database", err)

		return count, err
	}

	return count, nil
}

func (r *LedgerPostgreSQLRepository) GetSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (map[string]any, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.get_ledger_settings")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		return nil, err
	}

	var settingsJSON []byte

	_, spanQuery := tracer.Start(ctx, "postgres.get_settings.query")

	query := `SELECT settings FROM ledger WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`

	row := db.QueryRowContext(ctx, query, organizationID, ledgerID)

	spanQuery.End()

	if err := row.Scan(&settingsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Ledger not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

		return nil, err
	}

	var settings map[string]any

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &settings); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal settings", err)
			return nil, err
		}
	}

	if settings == nil {
		settings = make(map[string]any)
	}

	return settings, nil
}

// Implementation note: merges via PostgreSQL's JSONB || operator (top-level only).
func (r *LedgerPostgreSQLRepository) UpdateSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_ledger_settings")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		return nil, err
	}

	// Normalize nil settings to empty map to prevent json.Marshal producing "null"
	// which would overwrite existing JSONB settings
	if settings == nil {
		settings = map[string]any{}
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal settings", err)
		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.update_settings.exec")

	// Use JSONB merge operator (||) to merge new settings with existing.
	//
	// NOTE: PostgreSQL || performs SHALLOW merge at top level only.
	// Nested objects are replaced entirely, not deep-merged.
	// Example:
	//   existing: {"a": {"x": 1, "y": 2}, "b": 1}
	//   update:   {"a": {"z": 3}}
	//   result:   {"a": {"z": 3}, "b": 1}  // "a" replaced, "b" preserved
	//
	// To update nested keys, clients must read-modify-write the nested object.
	query := `
		UPDATE ledger
		SET settings = settings || $1::jsonb, updated_at = now()
		WHERE organization_id = $2 AND id = $3 AND deleted_at IS NULL
		RETURNING settings
	`

	var updatedSettingsJSON []byte

	err = db.QueryRowContext(ctx, query, settingsJSON, organizationID, ledgerID).Scan(&updatedSettingsJSON)

	spanExec.End()

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Ledger not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to update settings", err)

		return nil, err
	}

	var updatedSettings map[string]any

	if len(updatedSettingsJSON) > 0 {
		if err := json.Unmarshal(updatedSettingsJSON, &updatedSettings); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal updated settings", err)
			return nil, err
		}
	}

	if updatedSettings == nil {
		updatedSettings = make(map[string]any)
	}

	return updatedSettings, nil
}

func (r *LedgerPostgreSQLRepository) ReplaceSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.replace_ledger_settings")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		return nil, err
	}

	// Normalize nil settings to empty map
	if settings == nil {
		settings = map[string]any{}
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal settings", err)
		return nil, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.replace_settings.exec")

	// Direct assignment (=) instead of merge (||) - complete replacement
	query := `
		UPDATE ledger
		SET settings = $1::jsonb, updated_at = now()
		WHERE organization_id = $2 AND id = $3 AND deleted_at IS NULL
		RETURNING settings
	`

	var updatedSettingsJSON []byte

	err = db.QueryRowContext(ctx, query, settingsJSON, organizationID, ledgerID).Scan(&updatedSettingsJSON)

	spanExec.End()

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Ledger not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to replace settings", err)

		return nil, err
	}

	var updatedSettings map[string]any

	if len(updatedSettingsJSON) > 0 {
		if err := json.Unmarshal(updatedSettingsJSON, &updatedSettings); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal updated settings", err)
			return nil, err
		}
	}

	if updatedSettings == nil {
		updatedSettings = make(map[string]any)
	}

	return updatedSettings, nil
}

// Implementation note: uses SELECT FOR UPDATE inside a transaction to lock the row.
func (r *LedgerPostgreSQLRepository) UpdateSettingsAtomic(ctx context.Context, organizationID, ledgerID uuid.UUID, mergeFn func(existing map[string]any) (map[string]any, error)) (map[string]any, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_ledger_settings_atomic")
	defer span.End()

	if mergeFn == nil {
		err := pkg.ValidateBusinessError(constant.ErrBadRequest, "merge function must not be nil")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Nil merge function", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		return nil, err
	}

	// Begin transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to begin transaction", err)
		return nil, err
	}

	// Panic-safe cleanup: rollback on panic (then re-panic) or on error. Do not rollback on success.
	defer func() {
		if tx == nil {
			return
		}

		if r := recover(); r != nil {
			_ = tx.Rollback()

			panic(r)
		}

		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				logger.Log(ctx, libLog.LevelError, "Failed to rollback transaction", libLog.Err(rbErr))
			}
		}
	}()

	// SELECT FOR UPDATE to lock the row
	ctx, spanSelect := tracer.Start(ctx, "postgres.select_settings_for_update")

	var settingsJSON []byte

	selectQuery := `SELECT settings FROM ledger WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL FOR UPDATE`

	row := tx.QueryRowContext(ctx, selectQuery, organizationID, ledgerID)

	spanSelect.End()

	if scanErr := row.Scan(&settingsJSON); scanErr != nil {
		err = scanErr

		if errors.Is(scanErr, sql.ErrNoRows) {
			err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityLedger)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Ledger not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan settings", err)

		return nil, err
	}

	// Parse existing settings
	var existingSettings map[string]any

	if len(settingsJSON) > 0 {
		if unmarshalErr := json.Unmarshal(settingsJSON, &existingSettings); unmarshalErr != nil {
			err = unmarshalErr
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal existing settings", err)

			return nil, err
		}
	}

	if existingSettings == nil {
		existingSettings = make(map[string]any)
	}

	// Apply merge function
	mergedSettings, mergeErr := mergeFn(existingSettings)
	if mergeErr != nil {
		err = mergeErr
		libOpentelemetry.HandleSpanError(span, "Failed to merge settings", err)

		return nil, err
	}

	// Normalize nil to empty map
	if mergedSettings == nil {
		mergedSettings = make(map[string]any)
	}

	// Marshal merged settings
	mergedJSON, marshalErr := json.Marshal(mergedSettings)
	if marshalErr != nil {
		err = marshalErr
		libOpentelemetry.HandleSpanError(span, "Failed to marshal merged settings", err)

		return nil, err
	}

	// UPDATE with merged settings
	ctx, spanUpdate := tracer.Start(ctx, "postgres.update_settings_in_tx")

	updateQuery := `UPDATE ledger SET settings = $1::jsonb, updated_at = now() WHERE organization_id = $2 AND id = $3 AND deleted_at IS NULL`

	_, execErr := tx.ExecContext(ctx, updateQuery, mergedJSON, organizationID, ledgerID)

	spanUpdate.End()

	if execErr != nil {
		err = execErr
		libOpentelemetry.HandleSpanError(span, "Failed to update settings", err)

		return nil, err
	}

	// Commit transaction
	if commitErr := tx.Commit(); commitErr != nil {
		err = commitErr
		libOpentelemetry.HandleSpanError(span, "Failed to commit transaction", err)

		return nil, err
	}

	return mergedSettings, nil
}
