package settings

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Repository provides an interface for operations related to settings entities.
//
//go:generate mockgen --destination=settings.postgresql_mock.go --package=settings . Repository
type Repository interface {
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, settings *mmodel.Settings) (*mmodel.Settings, error)
	FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Settings, error)
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
		&record.Value,
		&record.Description,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert settings query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Settings{}).Name())
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

	row := db.QueryRowContext(ctx, `SELECT id, organization_id, ledger_id, key, value, description, created_at, updated_at, deleted_at FROM settings WHERE id = $1 AND organization_id = $2 AND ledger_id = $3 AND deleted_at IS NULL`,
		id, organizationID, ledgerID)

	err = row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Key,
		&record.Value,
		&record.Description,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to scan settings record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, services.ValidatePGError(&pgconn.PgError{Code: "02000"}, reflect.TypeOf(mmodel.Settings{}).Name())
		}

		return nil, err
	}

	spanQuery.End()

	return record.ToEntity(), nil
}
