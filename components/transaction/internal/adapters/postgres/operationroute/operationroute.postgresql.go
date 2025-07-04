package operationroute

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

// Repository provides an interface for operations related to operation route entities.
// It defines methods for creating, retrieving, updating, and deleting operation routes.
//
//go:generate mockgen --destination=operationroute.postgresql_mock.go --package=operationroute . Repository
type Repository interface {
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error)
	FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.OperationRoute, error)
	FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.OperationRoute, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error)
	HasTransactionRouteLinks(ctx context.Context, operationRouteID uuid.UUID) (bool, error)
}

// OperationRoutePostgreSQLRepository is a PostgreSQL implementation of the OperationRouteRepository.
type OperationRoutePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewOperationRoutePostgreSQLRepository creates a new instance of OperationRoutePostgreSQLRepository.
func NewOperationRoutePostgreSQLRepository(pc *libPostgres.PostgresConnection) *OperationRoutePostgreSQLRepository {
	c := &OperationRoutePostgreSQLRepository{
		connection: pc,
		tableName:  "operation_route",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create creates a new operation route in the database.
func (r *OperationRoutePostgreSQLRepository) Create(ctx context.Context, organizationID, ledgerID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_operation_route")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &OperationRoutePostgreSQLModel{}
	record.FromEntity(operationRoute)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "operation_route_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert operation_route record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO operation_route VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *`,
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Title,
		&record.Description,
		&record.Type,
		&record.AccountTypes,
		&record.AccountAlias,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		}

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to create operation route. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByID retrieves an operation route by its ID.
// It returns the operation route if found, otherwise it returns an error.
func (r *OperationRoutePostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation_route")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	query := `SELECT id, organization_id, ledger_id, title, description, type, account_types, account_alias, created_at, updated_at, deleted_at 
		FROM operation_route 
		WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`
	args := []any{organizationID, ledgerID, id}

	operationRoute := &OperationRoutePostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(
		&operationRoute.ID,
		&operationRoute.OrganizationID,
		&operationRoute.LedgerID,
		&operationRoute.Title,
		&operationRoute.Description,
		&operationRoute.Type,
		&operationRoute.AccountTypes,
		&operationRoute.AccountAlias,
		&operationRoute.CreatedAt,
		&operationRoute.UpdatedAt,
		&operationRoute.DeletedAt,
	); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan operation route", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		}

		return nil, err
	}

	return operationRoute.ToEntity(), nil
}

// FindByIDs retrieves operation routes by their IDs.
// It returns the operation routes if found, otherwise it returns an error.
func (r *OperationRoutePostgreSQLRepository) FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.OperationRoute, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation_routes_by_ids")
	defer span.End()

	if len(ids) == 0 {
		return []*mmodel.OperationRoute{}, nil
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	query := squirrel.Select("id", "organization_id", "ledger_id", "title", "description", "type", "account_types", "account_alias", "created_at", "updated_at", "deleted_at").
		From("operation_route").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": ids}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	findByIDsSql, args, err := query.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_ids.query")

	rows, err := db.QueryContext(ctx, findByIDsSql, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	var operationRoutes []*mmodel.OperationRoute

	foundIDs := make(map[uuid.UUID]bool)

	for rows.Next() {
		operationRoute := &OperationRoutePostgreSQLModel{}

		if err := rows.Scan(
			&operationRoute.ID,
			&operationRoute.OrganizationID,
			&operationRoute.LedgerID,
			&operationRoute.Title,
			&operationRoute.Description,
			&operationRoute.Type,
			&operationRoute.AccountTypes,
			&operationRoute.AccountAlias,
			&operationRoute.CreatedAt,
			&operationRoute.UpdatedAt,
			&operationRoute.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan operation route", err)

			return nil, err
		}

		operationRoutes = append(operationRoutes, operationRoute.ToEntity())
		foundIDs[operationRoute.ID] = true
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	var missingIDs []string

	for _, id := range ids {
		if !foundIDs[id] {
			missingIDs = append(missingIDs, id.String())
		}
	}

	if len(missingIDs) > 0 {
		missingIDsStr := strings.Join(missingIDs, ", ")

		libOpentelemetry.HandleSpanError(&span, "Operation route(s) not found", errors.New("operation route not found"))

		return nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name(), missingIDsStr)
	}

	return operationRoutes, nil
}

// Update updates an operation route by its ID.
// It returns the updated operation route if found, otherwise it returns an error.
func (r *OperationRoutePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_operation_route")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &OperationRoutePostgreSQLModel{}
	record.FromEntity(operationRoute)

	var updates []string

	var args []any

	if operationRoute.Title != "" {
		updates = append(updates, "title = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Title)
	}

	if operationRoute.Description != "" {
		updates = append(updates, "description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Description)
	}

	if operationRoute.AccountTypes != nil {
		updates = append(updates, "account_types = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.AccountTypes)
	}

	if operationRoute.AccountAlias != "" {
		updates = append(updates, "account_alias = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.AccountAlias)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE operation_route SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "operation_route_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert operation_route record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		}

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := services.ErrDatabaseItemNotFound

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to update operation route. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete an Operation Route entity from the database (soft delete) using the provided ID.
func (r *OperationRoutePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_operation_route")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	query := "UPDATE operation_route SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
	args := []any{organizationID, ledgerID, id}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute query", err)

		return pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())
	}

	spanExec.End()

	return nil
}

// FindAll retrieves all operation routes with pagination.
// It returns a list of operation routes, a cursor pagination object, and an error if the operation fails.
// The function supports filtering by date range and pagination.
func (r *OperationRoutePostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operation_routes")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	operationRoutes := make([]*mmodel.OperationRoute, 0)

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
		var operationRoute OperationRoutePostgreSQLModel

		if err := rows.Scan(
			&operationRoute.ID,
			&operationRoute.OrganizationID,
			&operationRoute.LedgerID,
			&operationRoute.Title,
			&operationRoute.Description,
			&operationRoute.Type,
			&operationRoute.AccountTypes,
			&operationRoute.AccountAlias,
			&operationRoute.CreatedAt,
			&operationRoute.UpdatedAt,
			&operationRoute.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan operation route", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		operationRoutes = append(operationRoutes, operationRoute.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(operationRoutes) > filter.Limit

	operationRoutes = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, operationRoutes, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(operationRoutes) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, operationRoutes[0].ID.String(), operationRoutes[len(operationRoutes)-1].ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operationRoutes, cur, nil
}

// HasTransactionRouteLinks checks if an operation route is linked to any transaction routes.
// It returns true if the operation route is linked to at least one transaction route, false otherwise.
func (r *OperationRoutePostgreSQLRepository) HasTransactionRouteLinks(ctx context.Context, operationRouteID uuid.UUID) (bool, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.has_transaction_route_links")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return false, err
	}

	query := `SELECT EXISTS(SELECT 1 FROM operation_transaction_route WHERE operation_route_id = $1 AND deleted_at IS NULL)`
	args := []any{operationRouteID}

	ctx, spanQuery := tracer.Start(ctx, "postgres.has_transaction_route_links.query")

	var exists bool

	row := db.QueryRowContext(ctx, query, args...)

	spanQuery.End()

	if err := row.Scan(&exists); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan exists result", err)

		return false, err
	}

	return exists, nil
}
