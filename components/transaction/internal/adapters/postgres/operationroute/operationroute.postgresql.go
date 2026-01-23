package operationroute

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
	poolmanager "github.com/LerianStudio/lib-commons/v2/commons/pool-manager"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
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
	FindTransactionRouteIDs(ctx context.Context, operationRouteID uuid.UUID) ([]uuid.UUID, error)
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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_operation_route")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &OperationRoutePostgreSQLModel{}
	record.FromEntity(operationRoute)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	result, err := db.ExecContext(ctx, `INSERT INTO operation_route(
										id, organization_id, ledger_id, title, description, code, operation_type, account_rule_type, account_rule_valid_if, created_at, updated_at
										) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *`,
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Title,
		&record.Description,
		&record.Code,
		&record.OperationType,
		&record.AccountRuleType,
		&record.AccountRuleValidIf,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", err)

			logger.Errorf("Failed to execute update query: %v", err)

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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation route. Rows affected is 0", err)

		logger.Warnf("Failed to create operation route. Rows affected is 0: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByID retrieves an operation route by its ID.
// It returns the operation route if found, otherwise it returns an error.
func (r *OperationRoutePostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation_route")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	query := `SELECT id, organization_id, ledger_id, title, description, code, operation_type, account_rule_type, account_rule_valid_if, created_at, updated_at, deleted_at 
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
		&operationRoute.Code,
		&operationRoute.OperationType,
		&operationRoute.AccountRuleType,
		&operationRoute.AccountRuleValidIf,
		&operationRoute.CreatedAt,
		&operationRoute.UpdatedAt,
		&operationRoute.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to scan operation route", err)

			logger.Warnf("Failed to scan operation route: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to scan operation route", err)

		logger.Errorf("Failed to scan operation route: %v", err)

		return nil, err
	}

	return operationRoute.ToEntity(), nil
}

// FindByIDs retrieves operation routes by their IDs.
// It returns the operation routes if found, otherwise it returns an error.
func (r *OperationRoutePostgreSQLRepository) FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation_routes_by_ids")
	defer span.End()

	if len(ids) == 0 {
		return []*mmodel.OperationRoute{}, nil
	}

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	query := squirrel.Select("id", "organization_id", "ledger_id", "title", "description", "code", "operation_type", "account_rule_type", "account_rule_valid_if", "created_at", "updated_at", "deleted_at").
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

		logger.Errorf("Failed to execute query: %v", err)

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
			&operationRoute.Code,
			&operationRoute.OperationType,
			&operationRoute.AccountRuleType,
			&operationRoute.AccountRuleValidIf,
			&operationRoute.CreatedAt,
			&operationRoute.UpdatedAt,
			&operationRoute.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan operation route", err)

			logger.Errorf("Failed to scan operation route: %v", err)

			return nil, err
		}

		operationRoutes = append(operationRoutes, operationRoute.ToEntity())
		foundIDs[operationRoute.ID] = true
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

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

		err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name(), missingIDsStr)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation route(s) not found", err)

		logger.Warnf("Operation route(s) not found: %v", missingIDsStr)

		return nil, err
	}

	return operationRoutes, nil
}

// Update updates an operation route by its ID.
// It returns the updated operation route if found, otherwise it returns an error.
func (r *OperationRoutePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_operation_route")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

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

	if operationRoute.Code != "" {
		updates = append(updates, "code = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Code)
	}

	if operationRoute.Account != nil {
		if operationRoute.Account.RuleType != "" {
			updates = append(updates, "account_rule_type = $"+strconv.Itoa(len(args)+1))
			args = append(args, record.AccountRuleType)
		}

		if operationRoute.Account.ValidIf != nil {
			updates = append(updates, "account_rule_valid_if = $"+strconv.Itoa(len(args)+1))
			args = append(args, record.AccountRuleValidIf)
		}
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

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.OperationRoute{}).Name())

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
		err := services.ErrDatabaseItemNotFound

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to update operation route. Rows affected is 0", err)

		logger.Warnf("Failed to update operation route. Rows affected is 0: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete an Operation Route entity from the database (soft delete) using the provided ID.
func (r *OperationRoutePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_operation_route")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	query := "UPDATE operation_route SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL"
	args := []any{organizationID, ledgerID, id}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return err
	}

	spanExec.End()

	return nil
}

// FindAll retrieves all operation routes with pagination.
// It returns a list of operation routes, a cursor pagination object, and an error if the operation fails.
// The function supports filtering by date range and pagination.
func (r *OperationRoutePostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operation_routes")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var operationRoutes []*mmodel.OperationRoute

	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			logger.Errorf("Failed to decode cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select(
		"id", "organization_id", "ledger_id", "title", "description", "operation_type",
		"account_rule_type", "account_rule_valid_if", "created_at", "updated_at", "deleted_at", "code",
	).
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

		logger.Errorf("Failed to build query: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

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
			&operationRoute.OperationType,
			&operationRoute.AccountRuleType,
			&operationRoute.AccountRuleValidIf,
			&operationRoute.CreatedAt,
			&operationRoute.UpdatedAt,
			&operationRoute.DeletedAt,
			&operationRoute.Code,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan operation route", err)

			logger.Errorf("Failed to scan operation route: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		operationRoutes = append(operationRoutes, operationRoute.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(operationRoutes) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	operationRoutes = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, operationRoutes, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(operationRoutes) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, operationRoutes[0].ID.String(), operationRoutes[len(operationRoutes)-1].ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operationRoutes, cur, nil
}

// HasTransactionRouteLinks checks if an operation route is linked to any transaction routes.
// It returns true if the operation route is linked to at least one transaction route, false otherwise.
func (r *OperationRoutePostgreSQLRepository) HasTransactionRouteLinks(ctx context.Context, operationRouteID uuid.UUID) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.has_transaction_route_links")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

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

		logger.Errorf("Failed to scan exists result: %v", err)

		return false, err
	}

	return exists, nil
}

// FindTransactionRouteIDs retrieves all transaction route IDs associated with a specific operation route.
// It returns a slice of transaction route UUIDs that are linked to the given operation route ID.
func (r *OperationRoutePostgreSQLRepository) FindTransactionRouteIDs(ctx context.Context, operationRouteID uuid.UUID) ([]uuid.UUID, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction_route_ids")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	query := `SELECT transaction_route_id FROM operation_transaction_route WHERE operation_route_id = $1 AND deleted_at IS NULL ORDER BY created_at`
	args := []any{operationRouteID}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_transaction_route_ids.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	var transactionRouteIDs []uuid.UUID

	for rows.Next() {
		var transactionRouteID uuid.UUID

		if err := rows.Scan(&transactionRouteID); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan transaction route ID", err)

			logger.Errorf("Failed to scan transaction route ID: %v", err)

			return nil, err
		}

		transactionRouteIDs = append(transactionRouteIDs, transactionRouteID)
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	return transactionRouteIDs, nil
}
