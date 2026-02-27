// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v3/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
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
type Repository interface {
	Create(ctx context.Context, ledger *mmodel.Ledger) (*mmodel.Ledger, error)
	Find(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error)
	FindAll(ctx context.Context, organizationID uuid.UUID, filter http.Pagination, name *string) ([]*mmodel.Ledger, error)
	FindByName(ctx context.Context, organizationID uuid.UUID, name string) (bool, error)
	ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Ledger, error)
	Update(ctx context.Context, organizationID, id uuid.UUID, ledger *mmodel.Ledger) (*mmodel.Ledger, error)
	Delete(ctx context.Context, organizationID, id uuid.UUID) error
	Count(ctx context.Context, organizationID uuid.UUID) (int64, error)
	GetSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (map[string]any, error)
	UpdateSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error)
	ReplaceSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error)
	UpdateSettingsAtomic(ctx context.Context, organizationID, ledgerID uuid.UUID, mergeFn func(existing map[string]any) (map[string]any, error)) (map[string]any, error)
}

// LedgerPostgreSQLRepository is a Postgresql-specific implementation of the LedgerRepository.
type LedgerPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewLedgerPostgreSQLRepository returns a new instance of LedgerPostgresRepository using the given Postgres connection.
func NewLedgerPostgreSQLRepository(pc *libPostgres.PostgresConnection) *LedgerPostgreSQLRepository {
	c := &LedgerPostgreSQLRepository{
		connection: pc,
		tableName:  "ledger",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *LedgerPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
	// GetModulePostgresForTenant returns only ErrTenantContextRequired
	// when no tenant DB is in context; safe to fall through to static connection.
	if db, err := tmcore.GetModulePostgresForTenant(ctx, "onboarding"); err == nil {
		return db, nil
	}

	return r.connection.GetDB()
}

// Create a new Ledger entity into Postgresql and returns it.
func (r *LedgerPostgreSQLRepository) Create(ctx context.Context, ledger *mmodel.Ledger) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_ledger")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

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
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal settings", err)
		logger.Errorf("Failed to marshal settings: %v", err)

		return nil, err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	result, err := db.ExecContext(ctx, `INSERT INTO ledger VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *`,
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
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", err)

			logger.Warnf("Failed to execute update query: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Find retrieves a Ledger entity from the database using the provided ID.
func (r *LedgerPostgreSQLRepository) Find(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_ledger")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	ledger := &LedgerPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	query, args, err := squirrel.Select(ledgerColumnList...).
		From("ledger").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		spanQuery.End()

		return nil, err
	}

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	var settingsJSON []byte
	if err := row.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
		&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt, &settingsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan row", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
	}

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &ledger.Settings); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal settings", err)
			logger.Errorf("Failed to unmarshal settings: %v", err)

			return nil, err
		}
	}

	return ledger.ToEntity(), nil
}

// FindAll retrieves Ledgers entities from the database.
func (r *LedgerPostgreSQLRepository) FindAll(ctx context.Context, organizationID uuid.UUID, filter http.Pagination, name *string) ([]*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_ledgers")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var ledgers []*mmodel.Ledger

	findAll := squirrel.Select(ledgerColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		OrderBy("id " + strings.ToUpper(filter.SortOrder)).
		Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	if name != nil && *name != "" {
		sanitized := http.EscapeSearchMetacharacters(*name)
		findAll = findAll.Where(squirrel.ILike{"name": sanitized + "%"})
	}

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel

		var settingsJSON []byte
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt, &settingsJSON); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		if len(settingsJSON) > 0 {
			if err := json.Unmarshal(settingsJSON, &ledger.Settings); err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal settings", err)
				logger.Errorf("Failed to unmarshal settings: %v", err)

				return nil, err
			}
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return ledgers, nil
}

// FindByName returns error and a boolean indicating if Ledger entities exists by name
func (r *LedgerPostgreSQLRepository) FindByName(ctx context.Context, organizationID uuid.UUID, name string) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_ledger_by_name")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_name.query")

	query, args, err := squirrel.Select(ledgerColumnList...).
		From("ledger").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Expr("LOWER(name) LIKE LOWER(?)", name)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		spanQuery.End()

		return false, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, reflect.TypeOf(mmodel.Ledger{}).Name(), name)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Ledger name conflict", err)

		return true, err
	}

	return false, nil
}

// ListByIDs retrieves Ledgers entities from the database using the provided IDs.
func (r *LedgerPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_ledgers_by_ids")
	defer span.End()

	if len(ids) == 0 {
		return []*mmodel.Ledger{}, nil
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var ledgers []*mmodel.Ledger

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_ledgers_by_ids.query")

	query, args, err := squirrel.Select(ledgerColumnList...).
		From("ledger").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Expr("id = ANY(?)", pq.Array(ids))).
		Where(squirrel.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to build query", err)

		spanQuery.End()

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var ledger LedgerPostgreSQLModel

		var settingsJSON []byte
		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt, &settingsJSON); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			logger.Errorf("Failed to scan row: %v", err)

			return nil, err
		}

		if len(settingsJSON) > 0 {
			if err := json.Unmarshal(settingsJSON, &ledger.Settings); err != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal settings", err)
				logger.Errorf("Failed to unmarshal settings: %v", err)

				return nil, err
			}
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows", err)

		return nil, err
	}

	return ledgers, nil
}

// Update a Ledger entity into Postgresql and returns the Ledger updated.
func (r *LedgerPostgreSQLRepository) Update(ctx context.Context, organizationID, id uuid.UUID, ledger *mmodel.Ledger) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_ledger")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &LedgerPostgreSQLModel{}
	record.FromEntity(ledger)

	var updates []string

	var args []any

	if ledger.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if ledger.OrganizationID != "" {
		updates = append(updates, "organization_id = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.OrganizationID)
	}

	if !ledger.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, id)

	query := `UPDATE ledger SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", err)

			logger.Warnf("Failed to execute update query: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Ledger entity from the database using the provided ID.
func (r *LedgerPostgreSQLRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_ledger")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE ledger SET deleted_at = now() WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`, organizationID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute database query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete ledger. Rows affected is 0", err)

		return err
	}

	return nil
}

// Count retrieves the number of Ledger entities in the database for the given organization ID.
func (r *LedgerPostgreSQLRepository) Count(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_ledgers")
	defer span.End()

	count := int64(0)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return count, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ledger WHERE organization_id = $1 AND deleted_at IS NULL", organizationID).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to query database", err)

		return count, err
	}

	return count, nil
}

// GetSettings retrieves the settings for a ledger by its ID.
func (r *LedgerPostgreSQLRepository) GetSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.get_ledger_settings")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	var settingsJSON []byte

	ctx, spanQuery := tracer.Start(ctx, "postgres.get_settings.query")

	query := `SELECT settings FROM ledger WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`

	row := db.QueryRowContext(ctx, query, organizationID, ledgerID)

	spanQuery.End()

	if err := row.Scan(&settingsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Ledger not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)
		logger.Errorf("Failed to scan row: %v", err)

		return nil, err
	}

	var settings map[string]any

	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &settings); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal settings", err)
			logger.Errorf("Failed to unmarshal settings: %v", err)

			return nil, err
		}
	}

	if settings == nil {
		settings = make(map[string]any)
	}

	return settings, nil
}

// UpdateSettings updates the settings for a ledger using JSONB merge semantics.
// New settings are merged with existing settings using PostgreSQL's || operator.
func (r *LedgerPostgreSQLRepository) UpdateSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_ledger_settings")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	// Normalize nil settings to empty map to prevent json.Marshal producing "null"
	// which would overwrite existing JSONB settings
	if settings == nil {
		settings = map[string]any{}
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal settings", err)
		logger.Errorf("Failed to marshal settings: %v", err)

		return nil, err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update_settings.exec")

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
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Ledger not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to update settings", err)
		logger.Errorf("Failed to update settings: %v", err)

		return nil, err
	}

	var updatedSettings map[string]any

	if len(updatedSettingsJSON) > 0 {
		if err := json.Unmarshal(updatedSettingsJSON, &updatedSettings); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal updated settings", err)
			logger.Errorf("Failed to unmarshal updated settings: %v", err)

			return nil, err
		}
	}

	if updatedSettings == nil {
		updatedSettings = make(map[string]any)
	}

	logger.Infof("Successfully updated settings for ledger %s", ledgerID.String())

	return updatedSettings, nil
}

// ReplaceSettings completely replaces the settings for a ledger.
// Unlike UpdateSettings which merges, this method overwrites the entire settings JSONB.
// Used by the service layer after performing application-level validation and merging.
func (r *LedgerPostgreSQLRepository) ReplaceSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, settings map[string]any) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.replace_ledger_settings")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	// Normalize nil settings to empty map
	if settings == nil {
		settings = map[string]any{}
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal settings", err)
		logger.Errorf("Failed to marshal settings: %v", err)

		return nil, err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.replace_settings.exec")

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
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Ledger not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to replace settings", err)
		logger.Errorf("Failed to replace settings: %v", err)

		return nil, err
	}

	var updatedSettings map[string]any

	if len(updatedSettingsJSON) > 0 {
		if err := json.Unmarshal(updatedSettingsJSON, &updatedSettings); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal updated settings", err)
			logger.Errorf("Failed to unmarshal updated settings: %v", err)

			return nil, err
		}
	}

	if updatedSettings == nil {
		updatedSettings = make(map[string]any)
	}

	logger.Infof("Successfully replaced settings for ledger %s", ledgerID.String())

	return updatedSettings, nil
}

// UpdateSettingsAtomic performs an atomic read-modify-write operation on ledger settings.
// It uses SELECT FOR UPDATE to lock the row, preventing concurrent modifications.
// The mergeFn receives the current settings and returns the merged settings to be written.
// This prevents lost updates under concurrent PATCH requests.
func (r *LedgerPostgreSQLRepository) UpdateSettingsAtomic(ctx context.Context, organizationID, ledgerID uuid.UUID, mergeFn func(existing map[string]any) (map[string]any, error)) (map[string]any, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_ledger_settings_atomic")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	// Begin transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin transaction", err)
		logger.Errorf("Failed to begin transaction: %v", err)

		return nil, err
	}

	// Ensure rollback on error, commit on success
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				logger.Errorf("Failed to rollback transaction: %v", rollbackErr)
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
			err = pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Ledger not found", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan settings", err)
		logger.Errorf("Failed to scan settings: %v", err)

		return nil, err
	}

	// Parse existing settings
	var existingSettings map[string]any

	if len(settingsJSON) > 0 {
		if unmarshalErr := json.Unmarshal(settingsJSON, &existingSettings); unmarshalErr != nil {
			err = unmarshalErr
			libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal existing settings", err)
			logger.Errorf("Failed to unmarshal existing settings: %v", err)

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
		libOpentelemetry.HandleSpanError(&span, "Failed to merge settings", err)
		logger.Errorf("Failed to merge settings: %v", err)

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
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal merged settings", err)
		logger.Errorf("Failed to marshal merged settings: %v", err)

		return nil, err
	}

	// UPDATE with merged settings
	ctx, spanUpdate := tracer.Start(ctx, "postgres.update_settings_in_tx")

	updateQuery := `UPDATE ledger SET settings = $1::jsonb, updated_at = now() WHERE organization_id = $2 AND id = $3 AND deleted_at IS NULL`

	_, execErr := tx.ExecContext(ctx, updateQuery, mergedJSON, organizationID, ledgerID)

	spanUpdate.End()

	if execErr != nil {
		err = execErr
		libOpentelemetry.HandleSpanError(&span, "Failed to update settings", err)
		logger.Errorf("Failed to update settings: %v", err)

		return nil, err
	}

	// Commit transaction
	if commitErr := tx.Commit(); commitErr != nil {
		err = commitErr
		libOpentelemetry.HandleSpanError(&span, "Failed to commit transaction", err)
		logger.Errorf("Failed to commit transaction: %v", err)

		return nil, err
	}

	logger.Infof("Successfully updated settings atomically for ledger %s", ledgerID.String())

	return mergedSettings, nil
}
