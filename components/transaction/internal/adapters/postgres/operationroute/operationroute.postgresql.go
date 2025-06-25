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
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Repository provides an interface for operations related to operation route entities.
// It defines methods for creating, retrieving, updating, and deleting operation routes.
type Repository interface {
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error)
	FindByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) (*mmodel.OperationRoute, error)
	Update(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error)
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

	result, err := db.ExecContext(ctx, `INSERT INTO operation_route VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *`,
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.Title,
		&record.Description,
		&record.Type,
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
func (r *OperationRoutePostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation_route")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	query := `SELECT id, organization_id, ledger_id, title, description, type, created_at, updated_at, deleted_at 
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

// Update updates an operation route by its ID.
// It returns the updated operation route if found, otherwise it returns an error.
func (r *OperationRoutePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
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
		return nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())
	}

	return record.ToEntity(), nil
}
