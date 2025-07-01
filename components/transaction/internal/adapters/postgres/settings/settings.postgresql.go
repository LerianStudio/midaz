package settings

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Repository provides an interface for operations related to settings entities.
//
//go:generate mockgen --destination=settings.postgresql_mock.go --package=settings . Repository
type Repository interface {
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, settings *mmodel.Settings) (*mmodel.Settings, error)
	FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Settings, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, settings *mmodel.Settings) (*mmodel.Settings, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Settings, libHTTP.CursorPagination, error)
}

// SettingsPostgreSQLRepository is a PostgreSQL implementation of the SettingsRepository.
type SettingsPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewSettingsPostgreSQLRepository creates a new instance of SettingsPostgreSQLRepository.
func NewSettingsPostgreSQLRepository(pc *libPostgres.PostgresConnection) *SettingsPostgreSQLRepository {
	c := &SettingsPostgreSQLRepository{
		connection: pc,
		tableName:  "settings",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create creates a new setting.
// It returns the created setting and an error if the operation fails.
func (r *SettingsPostgreSQLRepository) Create(ctx context.Context, organizationID, ledgerID uuid.UUID, settings *mmodel.Settings) (*mmodel.Settings, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_settings")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &SettingsPostgreSQLModel{}
	record.FromEntity(settings)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "settings_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert settings record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO settings VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Key,
		&record.Active,
		&record.Description,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert settings query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Settings{}).Name(), record.Key)
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Settings{}).Name())

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to create settings. Rows affected is 0", err)

		return nil, err
	}

	spanExec.End()

	return record.ToEntity(), nil
}

// FindByID retrieves a setting by its ID.
// It returns the setting if found, otherwise it returns an error.
func (r *SettingsPostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Settings, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_settings_by_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var record SettingsPostgreSQLModel

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_id.query")

	row := db.QueryRowContext(ctx, `SELECT id, organization_id, ledger_id, key, active, description, created_at, updated_at, deleted_at FROM settings WHERE id = $1 AND organization_id = $2 AND ledger_id = $3 AND deleted_at IS NULL`,
		id, organizationID, ledgerID)

	err = row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Key,
		&record.Active,
		&record.Description,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to scan settings record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, services.ErrDatabaseItemNotFound
		}

		return nil, err
	}

	spanQuery.End()

	return record.ToEntity(), nil
}

// Update updates a setting by its ID.
// It returns the updated setting if found, otherwise it returns an error.
func (r *SettingsPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, settings *mmodel.Settings) (*mmodel.Settings, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_settings")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &SettingsPostgreSQLModel{}
	record.FromEntity(settings)

	var updates []string

	var args []any

	if settings.Active != nil {
		updates = append(updates, "active = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Active)
	}

	if settings.Description != "" {
		updates = append(updates, "description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Description)
	}

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
	args = append(args, time.Now(), organizationID, ledgerID, id)

	query := `UPDATE settings SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "settings_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert settings from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Settings{}).Name(), record.Key)
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := services.ErrDatabaseItemNotFound

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to update settings. Rows affected is 0", err)

		return nil, err
	}

	spanExec.End()

	updatedSettings, err := r.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get updated settings", err)

		return nil, err
	}

	return updatedSettings, nil
}

// Delete performs a soft delete of a setting by its ID.
// It returns an error if the operation fails or if the setting is not found.
func (r *SettingsPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_settings")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	query := "UPDATE settings SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
	args := []any{organizationID, ledgerID, id}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute delete query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		return services.ErrDatabaseItemNotFound
	}

	return nil
}

// FindAll retrieves all settings with cursor pagination.
// It returns a list of settings, a cursor pagination object, and an error if the operation fails.
func (r *SettingsPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Settings, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_settings")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	settings := make([]*mmodel.Settings, 0)

	decodedCursor := libHTTP.Cursor{}
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !isFirstPage {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDate(filter.StartDate, libPointers.Int(-1))}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDate(filter.EndDate, libPointers.Int(1))}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var record SettingsPostgreSQLModel
		if err := rows.Scan(
			&record.ID,
			&record.OrganizationID,
			&record.LedgerID,
			&record.Key,
			&record.Active,
			&record.Description,
			&record.CreatedAt,
			&record.UpdatedAt,
			&record.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan settings record", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		settings = append(settings, record.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(settings) > filter.Limit

	settings = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, settings, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(settings) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, settings[0].ID.String(), settings[len(settings)-1].ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return settings, cur, nil
}
