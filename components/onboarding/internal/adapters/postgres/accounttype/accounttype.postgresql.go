package accounttype

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

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
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error)
	FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error)
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
		tableName:  "account_type",
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

// FindByID retrieves an account type by its ID.
// It returns the account type if found, otherwise it returns an error.
func (r *AccountTypePostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_type_by_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var record AccountTypePostgreSQLModel

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_id.query")

	row := db.QueryRowContext(ctx, `
		SELECT 
			id, 
			organization_id, 
			ledger_id, 
			name, 
			description, 
			key_value, 
			created_at, 
			updated_at, 
			deleted_at 
		FROM account_type 
		WHERE id = $1 
			AND organization_id = $2 
			AND ledger_id = $3 
			AND deleted_at IS NULL`,
		id, organizationID, ledgerID)

	err = row.Scan(
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
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to scan account type record", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, services.ErrDatabaseItemNotFound
		}

		return nil, err
	}

	spanQuery.End()

	return record.ToEntity(), nil
}

// Update updates an account type by its ID.
// It returns the updated account type if found, otherwise it returns an error.
func (r *AccountTypePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_account_type")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &AccountTypePostgreSQLModel{}
	record.FromEntity(accountType)

	var updates []string

	var args []any

	if accountType.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if accountType.Description != "" {
		updates = append(updates, "description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Description)
	}

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
	args = append(args, time.Now(), organizationID, ledgerID, id)

	query := `UPDATE account_type SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "account_type_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert account type from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

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
		err := services.ErrDatabaseItemNotFound

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to update account type. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}
