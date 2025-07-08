package accounttype

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Repository provides an interface for operations related to account type entities.
//
//go:generate mockgen --destination=accounttype.postgresql_mock.go --package=accounttype . Repository
type Repository interface {
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error)
}

// AccountTypePostgreSQLRepository is a PostgreSQL implementation of the AccountTypeRepository.
type AccountTypePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewAccountTypePostgreSQLRepository creates a new instance of AccountTypePostgreSQLRepository.
func NewAccountTypePostgreSQLRepository(pc *libPostgres.PostgresConnection) *AccountTypePostgreSQLRepository {
	c := &AccountTypePostgreSQLRepository{
		connection: pc,
		tableName:  "account_types",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create creates a new account type.
// It returns the created account type and an error if the operation fails.
func (r *AccountTypePostgreSQLRepository) Create(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_account_type")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AccountTypePostgreSQLModel{}
	record.FromEntity(accountType)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "account_type_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert account type record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO account_type VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Name,
		&record.Description,
		&record.KeyValue,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert account type query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.AccountType{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to create account type. Rows affected is 0", err)

		return nil, err
	}

	spanExec.End()

	return record.ToEntity(), nil
}
