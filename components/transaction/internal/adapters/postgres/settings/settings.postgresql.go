package settings

import (
	"context"
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
// It defines methods for creating settings.
//
//go:generate mockgen --destination=settings.postgresql_mock.go --package=settings . Repository
type Repository interface {
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, settings *mmodel.Settings) (*mmodel.Settings, error)
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
