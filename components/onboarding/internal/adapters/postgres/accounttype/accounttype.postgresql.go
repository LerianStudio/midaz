package accounttype

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmigration"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

const (
	// argsOffsetForWhereClause represents the offset for WHERE clause arguments in SQL queries
	argsOffsetForWhereClause = 2
)

var accountTypeColumnList = []string{
	"id",
	"organization_id",
	"ledger_id",
	"name",
	"description",
	"key_value",
	"created_at",
	"updated_at",
	"deleted_at",
}

type accountTypeQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func primaryAccountTypeQueryer(db interface {
	PrimaryDBs() []*sql.DB
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
},
) accountTypeQueryer {
	if primaries := db.PrimaryDBs(); len(primaries) > 0 && primaries[0] != nil {
		return primaries[0]
	}

	return db
}

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
	wrapper    *mmigration.MigrationWrapper // For future health checks
	tableName  string
}

// NewAccountTypePostgreSQLRepository creates a new instance of AccountTypePostgreSQLRepository.
func NewAccountTypePostgreSQLRepository(mw *mmigration.MigrationWrapper) *AccountTypePostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "AccountTypePostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "AccountTypePostgreSQLRepository")

	return &AccountTypePostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "account_type",
	}
}

// Create creates a new account type.
// It returns the created account type and an error if the operation fails.
func (r *AccountTypePostgreSQLRepository) Create(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
	assert.NotNil(accountType, "accountType entity must not be nil for Create",
		"repository", "AccountTypePostgreSQLRepository",
		"organizationID", organizationID,
		"ledgerID", ledgerID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_account_type")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	record := &AccountTypePostgreSQLModel{}
	record.FromEntity(accountType)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			validatedErr := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.AccountType{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute insert account type query", validatedErr)

			return nil, validatedErr
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert account type query", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	if rowsAffected == 0 {
		notFoundErr := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to create account type. Rows affected is 0", notFoundErr)

		return nil, notFoundErr
	}

	spanExec.End()

	return record.ToEntity(), nil
}

// FindByID retrieves an account type by its ID.
// It returns the account type if found, otherwise it returns an error.
func (r *AccountTypePostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_type_by_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	var record AccountTypePostgreSQLModel

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_id.query")

	queryDB := primaryAccountTypeQueryer(db)

	row := queryDB.QueryRowContext(ctx, `
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

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	spanQuery.End()

	return record.ToEntity(), nil
}

// FindByKey retrieves an account type by its key within an organization and ledger.
// It returns the account type if found, otherwise it returns an error.
func (r *AccountTypePostgreSQLRepository) FindByKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_account_type_by_key")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	var record AccountTypePostgreSQLModel

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_key.query")

	queryDB := primaryAccountTypeQueryer(db)

	row := queryDB.QueryRowContext(ctx, `
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

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	spanQuery.End()

	return record.ToEntity(), nil
}

// Update updates an account type by its ID.
// It returns the updated account type if found, otherwise it returns an error.
func (r *AccountTypePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_account_type")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
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
		` WHERE organization_id = $` + strconv.Itoa(len(args)-argsOffsetForWhereClause) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL RETURNING ` + strings.Join(accountTypeColumnList, ", ")

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	var updated AccountTypePostgreSQLModel

	err = db.QueryRowContext(ctx, query, args...).Scan(
		&updated.ID,
		&updated.OrganizationID,
		&updated.LedgerID,
		&updated.Name,
		&updated.Description,
		&updated.KeyValue,
		&updated.CreatedAt,
		&updated.UpdatedAt,
		&updated.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			notFoundErr := services.ErrDatabaseItemNotFound

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to update account type. Rows affected is 0", notFoundErr)

			return nil, notFoundErr
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			validatedErr := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.AccountType{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", validatedErr)

			logger.Warnf("Failed to execute update query: %v", validatedErr)

			return nil, validatedErr
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	return updated.ToEntity(), nil
}

// prepareCursorPagination prepares cursor pagination parameters
func (r *AccountTypePostgreSQLRepository) prepareCursorPagination(filter http.Pagination) (libHTTP.Cursor, bool, string, error) {
	decodedCursor := libHTTP.Cursor{}
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !isFirstPage {
		var err error

		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			return libHTTP.Cursor{}, false, "", pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
		}
	}

	return decodedCursor, isFirstPage, orderDirection, nil
}

// buildAccountTypeFindAllQuery constructs the SQL query for finding all account types
func (r *AccountTypePostgreSQLRepository) buildAccountTypeFindAllQuery(organizationID, ledgerID uuid.UUID, filter http.Pagination, decodedCursor libHTTP.Cursor, orderDirection string) (string, []any, string, error) {
	findAll := squirrel.Select("*").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where("deleted_at IS NULL").
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, orderDirection = libHTTP.ApplyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)

	query, args, err := findAll.ToSql()
	if err != nil {
		return "", nil, "", pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	return query, args, orderDirection, nil
}

// FindAll retrieves all account types with cursor pagination.
// It returns the account types, pagination cursor, and an error if the operation fails.
func (r *AccountTypePostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.AccountType, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_account_types")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	decodedCursor, isFirstPage, orderDirection, err := r.prepareCursorPagination(filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	query, args, orderDirection, err := r.buildAccountTypeFindAllQuery(organizationID, ledgerID, filter, decodedCursor, orderDirection)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var accountTypes []*mmodel.AccountType

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	queryDB := primaryAccountTypeQueryer(db)

	rows, err := queryDB.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
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

			return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
		}

		accountTypes = append(accountTypes, record.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	hasPagination := len(accountTypes) > filter.Limit

	accountTypes = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, accountTypes, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(accountTypes) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, accountTypes[0].ID.String(), accountTypes[len(accountTypes)-1].ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
		}
	}

	return accountTypes, cur, nil
}

// ListByIDs retrieves account types by their IDs.
// It returns the account types matching the provided IDs or an error if the operation fails.
func (r *AccountTypePostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_account_types_by_ids")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
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

		logger.Errorf("Failed to execute query: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
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

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
		}

		accountTypes = append(accountTypes, record.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	return accountTypes, nil
}

// Delete performs a soft delete of an account type by its ID.
// It returns an error if the operation fails or if the account type is not found.
func (r *AccountTypePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_account_type")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	query := "UPDATE account_type SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
	args := []any{organizationID, ledgerID, id}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute delete query", err)

		logger.Errorf("Failed to execute delete query: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AccountType{}).Name())
	}

	if rowsAffected == 0 {
		return services.ErrDatabaseItemNotFound
	}

	return nil
}
