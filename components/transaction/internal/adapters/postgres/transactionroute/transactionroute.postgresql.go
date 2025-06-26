package transactionroute

import (
	"context"
	"errors"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
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
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_transaction_route")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &TransactionRoutePostgreSQLModel{}
	record.FromEntity(transactionRoute)

	// Begin transaction for atomicity
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin transaction", err)

		return nil, err
	}

	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				libOpentelemetry.HandleSpanError(&span, "Failed to rollback transaction", rollbackErr)
			}
		}
	}()

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "transaction_route_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert transaction_route record from entity to JSON string", err)

		return nil, err
	}

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
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert transaction route query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

		libOpentelemetry.HandleSpanError(&spanExec, "Failed to create transaction route. Rows affected is 0", err)

		return nil, err
	}

	spanExec.End()

	// Insert operation route relations
	if len(transactionRoute.OperationRoutes) > 0 {
		_, spanRelations := tracer.Start(ctx, "postgres.create.operation_relations")
		defer spanRelations.End()

		for _, operationRoute := range transactionRoute.OperationRoutes {
			relationID := uuid.New()

			_, err := tx.ExecContext(ctx, `INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at) VALUES ($1, $2, $3, $4)`,
				relationID,
				operationRoute.ID,
				record.ID,
				record.CreatedAt,
			)
			if err != nil {
				libOpentelemetry.HandleSpanError(&spanRelations, "Failed to insert operation route relation", err)

				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) {
					return nil, services.ValidatePGError(pgErr, "operation_transaction_route")
				}

				return nil, err
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit transaction", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByID retrieves a transaction route by its ID including its operation routes.
// It returns the transaction route if found, otherwise it returns an error.
func (r *TransactionRoutePostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) (*mmodel.TransactionRoute, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction_route_by_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	// First get the transaction route in a subquery
	subQuery := squirrel.Select("*").
		From("transaction_route").
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": id}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	// Main query with LEFT JOIN to operation routes
	mainQuery := squirrel.Select("*").
		FromSelect(subQuery, "tr").
		LeftJoin("operation_transaction_route otr ON tr.id = otr.transaction_route_id AND otr.deleted_at IS NULL").
		LeftJoin("operation_route or_data ON otr.operation_route_id = or_data.id AND or_data.deleted_at IS NULL").
		OrderBy("or_data.created_at").
		PlaceholderFormat(squirrel.Dollar)

	sql, args, err := mainQuery.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_by_id.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

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
			&opRoute.Type,
			&opRoute.CreatedAt,
			&opRoute.UpdatedAt,
			&opRoute.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan transaction route", err)

			return nil, err
		}

		// Set transaction route data (will be the same for all rows)
		if transactionRoute == nil {
			transactionRoute = tr.ToEntity()
			transactionRoute.OperationRoutes = make([]mmodel.OperationRoute, 0)
		}

		// Add operation route if it exists and hasn't been added yet
		// Check for non-zero UUID to determine if operation route exists (LEFT JOIN might return zeros)
		nilUUID := uuid.UUID{}
		if opRoute.ID != nilUUID && !operationRoutesMap[opRoute.ID] {
			operationRoute := opRoute.ToEntity()
			transactionRoute.OperationRoutes = append(transactionRoute.OperationRoutes, *operationRoute)
			operationRoutesMap[opRoute.ID] = true
		}
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)

		return nil, err
	}

	if transactionRoute == nil {
		return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	return transactionRoute, nil
}
