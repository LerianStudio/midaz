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
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// Repository provides an interface for operations related to account type entities.
//
//go:generate mockgen --destination=accounttype.postgresql_mock.go --package=accounttype . Repository
type Repository interface {
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error)
	FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error)
	FindByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.AccountType, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.AccountType, libHTTP.CursorPagination, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.AccountType, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
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
		strings.ToLower(record.KeyValue),
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

// FindByKey retrieves an account type by its key within an organization and ledger.
// It returns the account type if found, otherwise it returns an error.
func (r *AccountTypePostgreSQLRepository) FindByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.AccountType, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_type_by_key")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var record AccountTypePostgreSQLModel

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_key.query")

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
		WHERE key_value = $1 
			AND organization_id = $2 
			AND ledger_id = $3 
			AND deleted_at IS NULL`,
		strings.ToLower(key), organizationID, ledgerID)

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

// FindAll retrieves all account types with cursor pagination.
// It returns the account types, pagination cursor, and an error if the operation fails.
func (r *AccountTypePostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.AccountType, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_account_types")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var accountTypes []*mmodel.AccountType

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
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
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
		var record AccountTypePostgreSQLModel
		if err := rows.Scan(
			&record.ID,
			&record.OrganizationID,
			&record.LedgerID,
			&record.Name,
			&record.Description,
			&record.KeyValue,
			&record.CreatedAt,
			&record.UpdatedAt,
			&record.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan account type record", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		accountTypes = append(accountTypes, record.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(accountTypes) > filter.Limit

	accountTypes = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, accountTypes, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(accountTypes) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, accountTypes[0].ID.String(), accountTypes[len(accountTypes)-1].ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return accountTypes, cur, nil
}

// ListByIDs retrieves account types by their IDs.
// It returns the account types matching the provided IDs or an error if the operation fails.
func (r *AccountTypePostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.AccountType, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_account_types_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var accountTypes []*mmodel.AccountType

	ctx, spanQuery := tracer.Start(ctx, "postgres.list_by_ids.query")

	query := `SELECT 
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
	WHERE organization_id = $1 
		AND ledger_id = $2 
		AND id = ANY($3) 
		AND deleted_at IS NULL 
	ORDER BY created_at DESC`

	rows, err := db.QueryContext(ctx, query, organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var record AccountTypePostgreSQLModel
		if err := rows.Scan(
			&record.ID,
			&record.OrganizationID,
			&record.LedgerID,
			&record.Name,
			&record.Description,
			&record.KeyValue,
			&record.CreatedAt,
			&record.UpdatedAt,
			&record.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan account type record", err)

			return nil, err
		}

		accountTypes = append(accountTypes, record.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	return accountTypes, nil
}

// Delete performs a soft delete of an account type by its ID.
// It returns an error if the operation fails or if the account type is not found.
func (r *AccountTypePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_account_type")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	query := "UPDATE account_type SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
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
