package transactionroute

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
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Repository provides an interface for operations related to transaction route entities.
// It defines methods for creating transaction routes.
//
//go:generate mockgen --destination=transactionroute.postgresql_mock.go --package=transactionroute . Repository
type Repository interface {
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionRoute *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error)
	FindByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) (*mmodel.TransactionRoute, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transactionRoute *mmodel.TransactionRoute, toAdd, toRemove []uuid.UUID) (*mmodel.TransactionRoute, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID, toRemove []uuid.UUID) error
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.TransactionRoute, libHTTP.CursorPagination, error)
}

// TransactionRoutePostgreSQLRepository is a PostgreSQL implementation of the TransactionRouteRepository.
type TransactionRoutePostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewTransactionRoutePostgreSQLRepository creates a new instance of TransactionRoutePostgreSQLRepository.
func NewTransactionRoutePostgreSQLRepository(pc *libPostgres.PostgresConnection) *TransactionRoutePostgreSQLRepository {
	c := &TransactionRoutePostgreSQLRepository{
		connection: pc,
		tableName:  "transaction_route",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create creates a new transaction route and its operation route relations.
// It returns the created transaction route and an error if the operation fails.
// Uses database transactions to ensure atomicity - if any operation route relation fails, the entire operation is rolled back.
func (r *TransactionRoutePostgreSQLRepository) Create(ctx context.Context, organizationID, ledgerID uuid.UUID, transactionRoute *mmodel.TransactionRoute) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_transaction_route")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	record := &TransactionRoutePostgreSQLModel{}
	record.FromEntity(transactionRoute)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin transaction", err)

		logger.Errorf("Failed to begin transaction: %v", err)

		return nil, err
	}

	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to rollback transaction", rollbackErr)

				logger.Errorf("Failed to rollback transaction: %v", rollbackErr)
			}
		}
	}()

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	// Insert transaction route
	result, err := tx.ExecContext(ctx, `INSERT INTO transaction_route VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Title,
		&record.Description,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.DeletedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute insert transaction route query", err)

			logger.Errorf("Failed to execute insert transaction route query: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert transaction route query", err)

		logger.Errorf("Failed to execute insert transaction route query: %v", err)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to create transaction route. Rows affected is 0", err)

		logger.Warnf("Failed to create transaction route. Rows affected is 0: %v", err)

		return nil, err
	}

	spanExec.End()

	// Insert operation route relations
	if len(transactionRoute.OperationRoutes) > 0 {
		_, spanRelations := tracer.Start(ctx, "postgres.create.operation_relations")
		defer spanRelations.End()

		for _, operationRoute := range transactionRoute.OperationRoutes {
			relationID := libCommons.GenerateUUIDv7()

			_, err := tx.ExecContext(ctx, `INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at) VALUES ($1, $2, $3, $4)`,
				relationID,
				operationRoute.ID,
				record.ID,
				record.CreatedAt,
			)
			if err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) {
					err := services.ValidatePGError(pgErr, "operation_transaction_route")

					libOpentelemetry.HandleSpanBusinessErrorEvent(&spanRelations, "Failed to insert operation route relation", err)

					logger.Errorf("Failed to insert operation route relation: %v", err)

					return nil, err
				}

				libOpentelemetry.HandleSpanError(&spanRelations, "Failed to insert operation route relation", err)

				logger.Errorf("Failed to insert operation route relation: %v", err)

				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit transaction", err)

		logger.Errorf("Failed to commit transaction: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByID retrieves a transaction route by its ID including its operation routes.
// It returns the transaction route if found, otherwise it returns an error.
func (r *TransactionRoutePostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction_route_by_id")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	subQuery := squirrel.Select(
		"id", "organization_id", "ledger_id", "title", "description", "created_at", "updated_at", "deleted_at",
	).
		From("transaction_route").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	mainQuery := squirrel.Select(
		"tr.id", "tr.organization_id", "tr.ledger_id", "tr.title", "tr.description", "tr.created_at", "tr.updated_at", "tr.deleted_at",
		"otr.id", "otr.operation_route_id", "otr.transaction_route_id", "otr.created_at", "otr.deleted_at",
		"or_data.id", "or_data.organization_id", "or_data.ledger_id", "or_data.title", "or_data.description", "or_data.operation_type",
		"or_data.account_rule_type", "or_data.account_rule_valid_if", "or_data.created_at", "or_data.updated_at", "or_data.deleted_at", "or_data.code",
	).
		FromSelect(subQuery, "tr").
		LeftJoin("operation_transaction_route otr ON tr.id = otr.transaction_route_id AND otr.deleted_at IS NULL").
		LeftJoin("operation_route or_data ON otr.operation_route_id = or_data.id AND or_data.deleted_at IS NULL").
		OrderBy("or_data.created_at").
		PlaceholderFormat(squirrel.Dollar)

	sqlQuery, args, err := mainQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		logger.Errorf("Failed to build query: %v", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_id.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		logger.Errorf("Failed to execute query: %v", err)

		return nil, err
	}
	defer rows.Close()

	var transactionRoute *mmodel.TransactionRoute

	operationRoutesMap := make(map[uuid.UUID]bool) // To avoid duplicate operation routes

	for rows.Next() {
		var tr TransactionRoutePostgreSQLModel

		var otr struct {
			ID                 uuid.UUID
			OperationRouteID   uuid.UUID
			TransactionRouteID uuid.UUID
			CreatedAt          time.Time
			DeletedAt          *time.Time
		}

		var opRoute operationroute.OperationRoutePostgreSQLModel

		if err := rows.Scan(
			// Transaction route fields
			&tr.ID,
			&tr.OrganizationID,
			&tr.LedgerID,
			&tr.Title,
			&tr.Description,
			&tr.CreatedAt,
			&tr.UpdatedAt,
			&tr.DeletedAt,
			// Operation transaction route relation fields
			&otr.ID,
			&otr.OperationRouteID,
			&otr.TransactionRouteID,
			&otr.CreatedAt,
			&otr.DeletedAt,
			// Operation route fields
			&opRoute.ID,
			&opRoute.OrganizationID,
			&opRoute.LedgerID,
			&opRoute.Title,
			&opRoute.Description,
			&opRoute.OperationType,
			&opRoute.AccountRuleType,
			&opRoute.AccountRuleValidIf,
			&opRoute.CreatedAt,
			&opRoute.UpdatedAt,
			&opRoute.DeletedAt,
			&opRoute.Code,
		); err != nil {
			errMsg := "Failed to scan transaction route"

			if errors.Is(err, sql.ErrNoRows) {
				err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

				libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction route not found", err)

				logger.Warnf("%s: %v", "Transaction route not found", err)

				return nil, err
			}

			libOpentelemetry.HandleSpanError(&span, errMsg, err)

			logger.Errorf("%s: %v", errMsg, err)

			return nil, err
		}

		if transactionRoute == nil {
			transactionRoute = tr.ToEntity()
			transactionRoute.OperationRoutes = make([]mmodel.OperationRoute, 0)
		}

		nilUUID := uuid.UUID{}
		if opRoute.ID != nilUUID && !operationRoutesMap[opRoute.ID] {
			operationRoute := opRoute.ToEntity()
			transactionRoute.OperationRoutes = append(transactionRoute.OperationRoutes, *operationRoute)
			operationRoutesMap[opRoute.ID] = true
		}
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, err
	}

	if transactionRoute == nil {
		err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

		logMsg := "Transaction route not found"

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, logMsg, err)

		logger.Warnf("%s: %v", logMsg, err)

		return nil, err
	}

	return transactionRoute, nil
}

// Update updates a transaction route by its ID and manages its operation route relationships.
// It returns the updated transaction route and an error if the operation fails.
// If the transaction route has operation routes, it will update the relationships atomically.
func (r *TransactionRoutePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transactionRoute *mmodel.TransactionRoute, toAdd, toRemove []uuid.UUID) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_transaction_route")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin transaction", err)

		logger.Errorf("Failed to begin transaction: %v", err)

		return nil, err
	}

	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to rollback transaction", rollbackErr)

				logger.Errorf("Failed to rollback transaction: %v", rollbackErr)
			}
		}
	}()

	record := &TransactionRoutePostgreSQLModel{}
	record.FromEntity(transactionRoute)

	var updates []string

	var args []any

	if transactionRoute.Title != "" {
		updates = append(updates, "title = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Title)
	}

	if transactionRoute.Description != "" {
		updates = append(updates, "description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Description)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))
	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE transaction_route SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to execute update query", err)

			logger.Errorf("Failed to execute update query: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		logger.Errorf("Failed to execute update query: %v", err)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to get rows affected", err)

		logger.Errorf("Failed to get rows affected: %v", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := services.ErrDatabaseItemNotFound

		libOpentelemetry.HandleSpanBusinessErrorEvent(&spanExec, "Failed to update transaction route. Rows affected is 0", err)

		logger.Errorf("Failed to update transaction route. Rows affected is 0: %v", err)

		return nil, err
	}

	spanExec.End()

	if len(toAdd) > 0 || len(toRemove) > 0 {
		err = r.updateOperationRouteRelationships(ctx, tx, id, toAdd, toRemove)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to update operation route relationships", err)

			logger.Errorf("Failed to update operation route relationships: %v", err)

			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit transaction", err)

		logger.Errorf("Failed to commit transaction: %v", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete deletes a transaction route by its ID and manages its operation route relationships.
// It returns an error if the operation fails.
// If the transaction route has operation routes, it will delete the relationships atomically.
func (r *TransactionRoutePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID, toRemove []uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_transaction_route")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")
	defer spanExec.End()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin transaction", err)

		logger.Errorf("Failed to begin transaction: %v", err)

		return err
	}

	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to rollback transaction", rollbackErr)

				logger.Errorf("Failed to rollback transaction: %v", rollbackErr)
			}
		}
	}()

	_, err = tx.ExecContext(ctx, `UPDATE transaction_route SET deleted_at = NOW() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute delete query", err)

		logger.Errorf("Failed to execute delete query: %v", err)

		return err
	}

	err = r.updateOperationRouteRelationships(ctx, tx, id, make([]uuid.UUID, 0), toRemove)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to update operation route relationships", err)

		logger.Errorf("Failed to update operation route relationships: %v", err)

		return err
	}

	if err := tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit transaction", err)

		logger.Errorf("Failed to commit transaction: %v", err)

		return err
	}

	return nil
}

// FindAll retrieves all transaction routes with pagination.
// It returns a list of transaction routes, a cursor pagination object, and an error if the operation fails.
// The function supports filtering by date range and pagination.
func (r *TransactionRoutePostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.TransactionRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_transaction_routes")
	defer span.End()

	db, err := poolmanager.GetPostgresForTenant(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var transactionRoutes []*mmodel.TransactionRoute

	decodedCursor := libHTTP.Cursor{PointsNext: true}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode cursor", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select(
		"id", "organization_id", "ledger_id", "title", "description", "created_at", "updated_at", "deleted_at",
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
		var transactionRoute TransactionRoutePostgreSQLModel

		if err := rows.Scan(
			&transactionRoute.ID,
			&transactionRoute.OrganizationID,
			&transactionRoute.LedgerID,
			&transactionRoute.Title,
			&transactionRoute.Description,
			&transactionRoute.CreatedAt,
			&transactionRoute.UpdatedAt,
			&transactionRoute.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan transaction route", err)

			logger.Errorf("Failed to scan transaction route: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		transactionRoutes = append(transactionRoutes, transactionRoute.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	hasPagination := len(transactionRoutes) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor) || !hasPagination && !decodedCursor.PointsNext

	transactionRoutes = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.PointsNext, transactionRoutes, filter.Limit, orderDirection)

	cur := libHTTP.CursorPagination{}
	if len(transactionRoutes) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.PointsNext, transactionRoutes[0].ID.String(), transactionRoutes[len(transactionRoutes)-1].ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to calculate cursor", err)

			logger.Errorf("Failed to calculate cursor: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return transactionRoutes, cur, nil
}

// updateOperationRouteRelationships handles the complex logic of updating operation route relationships within an existing transaction
func (r *TransactionRoutePostgreSQLRepository) updateOperationRouteRelationships(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, transactionRouteID uuid.UUID, toAdd, toRemove []uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctxSpan, span := tracer.Start(ctx, "postgres.update_operation_route_relationships")
	defer span.End()

	// Soft delete relationships that should be removed
	if len(toRemove) > 0 {
		ctxDelete, spanDelete := tracer.Start(ctxSpan, "postgres.soft_delete_relationships")
		defer spanDelete.End()

		placeholders := make([]string, len(toRemove))
		deleteArgs := make([]any, len(toRemove)+1)

		for i, id := range toRemove {
			placeholders[i] = "$" + strconv.Itoa(i+2)
			deleteArgs[i+1] = id
		}

		deleteArgs[0] = transactionRouteID

		deleteQuery := `UPDATE operation_transaction_route 
						SET deleted_at = NOW() 
						WHERE transaction_route_id = $1 
						AND operation_route_id IN (` + strings.Join(placeholders, ",") + `) 
						AND deleted_at IS NULL`

		_, err := tx.ExecContext(ctxDelete, deleteQuery, deleteArgs...)
		if err != nil {
			libOpentelemetry.HandleSpanError(&spanDelete, "Failed to soft delete operation route relationships", err)

			logger.Errorf("Failed to soft delete operation route relationships: %v", err)

			return err
		}

		spanDelete.End()
	}

	// Create new relationships
	if len(toAdd) > 0 {
		ctxCreate, spanCreate := tracer.Start(ctxSpan, "postgres.create_relationships")
		defer spanCreate.End()

		for _, operationRouteID := range toAdd {
			relationID := libCommons.GenerateUUIDv7()
			now := time.Now()

			_, err := tx.ExecContext(ctxCreate, `INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at) VALUES ($1, $2, $3, $4)`,
				relationID,
				operationRouteID,
				transactionRouteID,
				now,
			)
			if err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) {
					err := services.ValidatePGError(pgErr, "operation_transaction_route")

					libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreate, "Failed to create operation route relationship", err)

					logger.Errorf("Failed to create operation route relationship: %v", err)

					return err
				}

				libOpentelemetry.HandleSpanError(&spanCreate, "Failed to create operation route relationship", err)

				logger.Errorf("Failed to create operation route relationship: %v", err)

				return err
			}
		}

		spanCreate.End()
	}

	return nil
}
