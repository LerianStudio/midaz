// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operationroute

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v5/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
)

// Repository provides the persistence contract for operation routes and their transaction-route links.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=operationroute.postgresql_mock.go --package=operationroute . Repository
type Repository interface {
	// Create persists a new operation route in the given organization and ledger.
	Create(ctx context.Context, organizationID, ledgerID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error)
	// FindByID returns one active operation route by scoped ID.
	FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.OperationRoute, error)
	// FindByIDs returns active operation routes matching every requested scoped ID.
	FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.OperationRoute, error)
	// Update applies partial changes to an active operation route by scoped ID.
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error)
	// Delete soft-deletes an active operation route by scoped ID.
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	// FindAll returns active operation routes for a ledger using cursor pagination and date filtering.
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error)
	// HasTransactionRouteLinks reports whether an active operation route is linked to active transaction routes in the same scope.
	HasTransactionRouteLinks(ctx context.Context, organizationID, ledgerID, operationRouteID uuid.UUID) (bool, error)
	// FindTransactionRouteIDs returns active transaction-route IDs linked to an operation route.
	FindTransactionRouteIDs(ctx context.Context, operationRouteID uuid.UUID) ([]uuid.UUID, error)
}

// OperationRoutePostgreSQLRepository is a PostgreSQL implementation of the OperationRouteRepository.
type OperationRoutePostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewOperationRoutePostgreSQLRepository creates a new instance of OperationRoutePostgreSQLRepository.
func NewOperationRoutePostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *OperationRoutePostgreSQLRepository {
	c := &OperationRoutePostgreSQLRepository{
		connection: pc,
		tableName:  "operation_route",
	}
	if len(requireTenant) > 0 {
		c.requireTenant = requireTenant[0]
	}

	return c
}

// getDB resolves the PostgreSQL database connection for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific dbresolver.DB into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (r *OperationRoutePostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
	// Module-specific connection (from middleware WithModule)
	if db := tmcore.GetPGContext(ctx, constant.ModuleTransaction); db != nil {
		return db, nil
	}

	// Generic connection fallback (single-module services)
	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if r.requireTenant {
		return nil, fmt.Errorf("tenant postgres connection missing from context")
	}

	if r.connection == nil {
		return nil, fmt.Errorf("postgres connection not available")
	}

	return r.connection.Resolver(ctx)
}

func (r *OperationRoutePostgreSQLRepository) Create(ctx context.Context, organizationID, ledgerID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	operationRouteID := uuid.Nil
	if operationRoute != nil {
		operationRouteID = operationRoute.ID
	}

	ctx, span := tracer.Start(ctx, "postgres.create_operation_route")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.operation_route_id", operationRouteID.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before creating operation route", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &OperationRoutePostgreSQLModel{}
	record.FromEntity(operationRoute)

	query, args, err := squirrel.Insert(r.tableName).
		Columns("id", "organization_id", "ledger_id", "title", "description", "code", "operation_type", "account_rule_type", "account_rule_valid_if", "accounting_entries", "created_at", "updated_at").
		Values(record.ID, record.OrganizationID, record.LedgerID, record.Title, record.Description, record.Code, record.OperationType, record.AccountRuleType, record.AccountRuleValidIf, record.AccountingEntries, record.CreatedAt, record.UpdatedAt).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build create query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build create query",
			libLog.Err(err),
			libLog.String("operation_route_id", record.ID.String()),
		)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built create operation route query", libLog.String("query", query))

	_, spanExec := tracer.Start(ctx, "postgres.create.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityOperationRoute)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute create query", err)

			logger.Log(ctx, libLog.LevelWarn, "Failed to execute create query",
				libLog.Err(err),
				libLog.String("operation_route_id", record.ID.String()),
			)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute create query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute create query",
			libLog.Err(err),
			libLog.String("operation_route_id", record.ID.String()),
		)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get rows affected",
			libLog.Err(err),
			libLog.String("operation_route_id", record.ID.String()),
		)

		return nil, err
	}

	spanExec.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityOperationRoute)

		libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to create operation route. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, "Operation route not created",
			libLog.Err(err),
			libLog.String("operation_route_id", record.ID.String()),
			libLog.Any("rows_affected", rowsAffected),
		)

		return nil, err
	}

	return record.ToEntity(), nil
}

func (r *OperationRoutePostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation_route")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.operation_route_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before finding operation route", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	query, args, err := squirrel.Select("id", "organization_id", "ledger_id", "title", "description", "code", "operation_type", "account_rule_type", "account_rule_valid_if", "accounting_entries", "created_at", "updated_at", "deleted_at").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID, "ledger_id": ledgerID, "id": id, "deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build find query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build find query",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built find operation route query", libLog.String("query", query))

	operationRoute := &OperationRoutePostgreSQLModel{}

	_, spanQuery := tracer.Start(ctx, "postgres.find.query")
	defer spanQuery.End()

	row := db.QueryRowContext(ctx, query, args...)

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
		&operationRoute.AccountingEntries,
		&operationRoute.CreatedAt,
		&operationRoute.UpdatedAt,
		&operationRoute.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, constant.EntityOperationRoute)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanQuery, "Operation route not found", err)

			logger.Log(ctx, libLog.LevelWarn, "Operation route not found",
				libLog.Err(err),
				libLog.String("operation_route_id", id.String()),
			)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanQuery, "Failed to scan operation route", err)

		logger.Log(ctx, libLog.LevelError, "Failed to scan operation route",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return nil, err
	}

	return operationRoute.ToEntity(), nil
}

func (r *OperationRoutePostgreSQLRepository) FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_operation_routes_by_ids")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Int("app.request.operation_route_ids_count", len(ids)),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before finding operation routes", err)

		return nil, err
	}

	if len(ids) == 0 {
		return []*mmodel.OperationRoute{}, nil
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	query := squirrel.Select("id", "organization_id", "ledger_id", "title", "description", "code", "operation_type", "account_rule_type", "account_rule_valid_if", "accounting_entries", "created_at", "updated_at", "deleted_at").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"id": ids}).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	findByIDsSql, args, err := query.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build find by IDs query", libLog.Err(err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built find operation routes by IDs query", libLog.String("query", findByIDsSql))

	_, spanQuery := tracer.Start(ctx, "postgres.find_by_ids.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, findByIDsSql, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

		return nil, err
	}
	defer rows.Close()

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
			&operationRoute.AccountingEntries,
			&operationRoute.CreatedAt,
			&operationRoute.UpdatedAt,
			&operationRoute.DeletedAt,
		); err != nil {
			libOpentelemetry.HandleSpanError(spanQuery, "Failed to scan operation route", err)
			logger.Log(ctx, libLog.LevelError, "Failed to scan operation route", libLog.Err(err))

			return nil, err
		}

		operationRoutes = append(operationRoutes, operationRoute.ToEntity())
		foundIDs[operationRoute.ID] = true
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to iterate rows", err)
		logger.Log(ctx, libLog.LevelError, "Failed to iterate rows", libLog.Err(err))

		return nil, err
	}

	spanQuery.SetAttributes(attribute.Int("db.rows_returned", len(operationRoutes)))

	var missingIDs []string

	for _, id := range ids {
		if !foundIDs[id] {
			missingIDs = append(missingIDs, id.String())
		}
	}

	if len(missingIDs) > 0 {
		missingIDsStr := strings.Join(missingIDs, ", ")

		err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, constant.EntityOperationRoute, missingIDsStr)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation route(s) not found", err)

		logger.Log(ctx, libLog.LevelWarn, "Operation routes not found",
			libLog.Err(err),
			libLog.Any("missing_operation_route_ids", missingIDs),
		)

		return nil, err
	}

	return operationRoutes, nil
}

func (r *OperationRoutePostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_operation_route")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.operation_route_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before updating operation route", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, err
	}

	record := &OperationRoutePostgreSQLModel{}
	record.FromEntity(operationRoute)

	qb := squirrel.Update(r.tableName)

	if operationRoute.Title != "" {
		qb = qb.Set("title", record.Title)
	}

	if operationRoute.Description != "" {
		qb = qb.Set("description", record.Description)
	}

	if operationRoute.Code != "" {
		qb = qb.Set("code", record.Code)
	}

	if operationRoute.Account != nil {
		if operationRoute.Account.RuleType != "" {
			qb = qb.Set("account_rule_type", record.AccountRuleType)
		}

		if operationRoute.Account.ValidIf != nil {
			qb = qb.Set("account_rule_valid_if", record.AccountRuleValidIf)
		}
	}

	if rawJSON := operationRoute.AccountingEntriesRaw; len(rawJSON) > 0 {
		mergeJSON, removeKeys := splitMergePatch(rawJSON)

		if len(mergeJSON) > 0 {
			if len(removeKeys) > 0 {
				qb = qb.Set("accounting_entries", squirrel.Expr("(COALESCE(accounting_entries, '{}'::jsonb) || ?::jsonb) - ?::text[]", mergeJSON, pq.Array(removeKeys)))
			} else {
				qb = qb.Set("accounting_entries", squirrel.Expr("COALESCE(accounting_entries, '{}'::jsonb) || ?::jsonb", mergeJSON))
			}
		} else if len(removeKeys) > 0 {
			qb = qb.Set("accounting_entries", squirrel.Expr("COALESCE(accounting_entries, '{}'::jsonb) - ?::text[]", pq.Array(removeKeys)))
		}
	} else if record.AccountingEntries != nil {
		qb = qb.Set("accounting_entries", squirrel.Expr("COALESCE(accounting_entries, '{}'::jsonb) || ?::jsonb", record.AccountingEntries))
	}

	record.UpdatedAt = time.Now()
	qb = qb.Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{"organization_id": organizationID, "ledger_id": ledgerID, "id": id, "deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := qb.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build update query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build update query",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built update operation route query", libLog.String("query", query))

	_, spanExec := tracer.Start(ctx, "postgres.update.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			err := services.ValidatePGError(pgErr, constant.EntityOperationRoute)

			libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to execute update query", err)

			logger.Log(ctx, libLog.LevelWarn, "Failed to execute update query",
				libLog.Err(err),
				libLog.String("operation_route_id", id.String()),
			)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute update query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute update query",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get rows affected",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return nil, err
	}

	spanExec.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))

	if rowsAffected == 0 {
		err := services.ErrDatabaseItemNotFound

		libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to update operation route. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, "Operation route not found for update",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
			libLog.Any("rows_affected", rowsAffected),
		)

		return nil, err
	}

	return record.ToEntity(), nil
}

func (r *OperationRoutePostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_operation_route")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.operation_route_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before deleting operation route", err)

		return err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return err
	}

	query, args, err := squirrel.Update(r.tableName).
		Set("deleted_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"id":              id,
			"deleted_at":      nil,
		}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build delete query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build delete query",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built delete operation route query", libLog.String("query", query))

	_, spanExec := tracer.Start(ctx, "postgres.delete.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute delete query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute delete query",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to get rows affected", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get rows affected",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return err
	}

	spanExec.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))

	if rowsAffected == 0 {
		err := services.ErrDatabaseItemNotFound

		libOpentelemetry.HandleSpanBusinessErrorEvent(spanExec, "Failed to delete operation route. Rows affected is 0", err)

		logger.Log(ctx, libLog.LevelWarn, "Operation route not found for delete",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
			libLog.Any("rows_affected", rowsAffected),
		)

		return err
	}

	return nil
}

func (r *OperationRoutePostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.OperationRoute, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_operation_routes")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Int("app.request.query.limit", filter.Limit),
		attribute.String("app.request.query.cursor", filter.Cursor),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before finding operation routes", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get database connection", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	var operationRoutes []*mmodel.OperationRoute

	decodedCursor := libHTTP.Cursor{Direction: libHTTP.CursorDirectionNext}
	orderDirection := strings.ToUpper(filter.SortOrder)

	if !libCommons.IsNilOrEmpty(&filter.Cursor) {
		decodedCursor, err = libHTTP.DecodeCursor(filter.Cursor)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to decode cursor", err)
			logger.Log(ctx, libLog.LevelError, "Failed to decode cursor", libLog.Err(err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	findAll := squirrel.Select(
		"id", "organization_id", "ledger_id", "title", "description", "operation_type",
		"account_rule_type", "account_rule_valid_if", "accounting_entries", "created_at", "updated_at", "deleted_at", "code",
	).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"ledger_id": ledgerID}).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		PlaceholderFormat(squirrel.Dollar)

	findAll, err = applyCursorPagination(findAll, decodedCursor, orderDirection, filter.Limit)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to apply cursor pagination", err)
		logger.Log(ctx, libLog.LevelError, "Failed to apply cursor pagination", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to build query", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built find all operation routes query", libLog.String("query", query))

	_, spanQuery := tracer.Start(ctx, "postgres.find_all.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)
		logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))

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
			&operationRoute.AccountingEntries,
			&operationRoute.CreatedAt,
			&operationRoute.UpdatedAt,
			&operationRoute.DeletedAt,
			&operationRoute.Code,
		); err != nil {
			libOpentelemetry.HandleSpanError(spanQuery, "Failed to scan operation route", err)
			logger.Log(ctx, libLog.LevelError, "Failed to scan operation route", libLog.Err(err))

			return nil, libHTTP.CursorPagination{}, err
		}

		operationRoutes = append(operationRoutes, operationRoute.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to iterate rows", err)
		logger.Log(ctx, libLog.LevelError, "Failed to iterate rows", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	spanQuery.SetAttributes(attribute.Int("db.rows_returned", len(operationRoutes)))

	hasPagination := len(operationRoutes) > filter.Limit
	isFirstPage := libCommons.IsNilOrEmpty(&filter.Cursor)

	operationRoutes = libHTTP.PaginateRecords(isFirstPage, hasPagination, decodedCursor.Direction, operationRoutes, filter.Limit)

	cur := libHTTP.CursorPagination{}
	if len(operationRoutes) > 0 {
		cur, err = libHTTP.CalculateCursor(isFirstPage, hasPagination, decodedCursor.Direction, operationRoutes[0].ID.String(), operationRoutes[len(operationRoutes)-1].ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to calculate cursor", err)
			logger.Log(ctx, libLog.LevelError, "Failed to calculate cursor", libLog.Err(err))

			return nil, libHTTP.CursorPagination{}, err
		}
	}

	return operationRoutes, cur, nil
}

func (r *OperationRoutePostgreSQLRepository) HasTransactionRouteLinks(ctx context.Context, organizationID, ledgerID, operationRouteID uuid.UUID) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.has_transaction_route_links")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.operation_route_id", operationRouteID.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before checking operation route links", err)

		return false, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection",
			libLog.Err(err),
			libLog.String("operation_route_id", operationRouteID.String()),
		)

		return false, err
	}

	query, args, err := squirrel.Select().
		Column(squirrel.Expr(`EXISTS(
		SELECT 1
		FROM operation_transaction_route otr
		JOIN operation_route opr ON opr.id = otr.operation_route_id
		WHERE otr.operation_route_id = ?
		AND opr.organization_id = ?
		AND opr.ledger_id = ?
		AND otr.deleted_at IS NULL
		AND opr.deleted_at IS NULL
	)`, operationRouteID, organizationID, ledgerID)).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build transaction route link query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build transaction route link query",
			libLog.Err(err),
			libLog.String("operation_route_id", operationRouteID.String()),
		)

		return false, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built transaction route link query", libLog.String("query", query))

	_, spanQuery := tracer.Start(ctx, "postgres.has_transaction_route_links.query")
	defer spanQuery.End()

	var exists bool

	row := db.QueryRowContext(ctx, query, args...)

	if err := row.Scan(&exists); err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to scan transaction route link result", err)

		logger.Log(ctx, libLog.LevelError, "Failed to scan transaction route link result",
			libLog.Err(err),
			libLog.String("operation_route_id", operationRouteID.String()),
		)

		return false, err
	}

	spanQuery.SetAttributes(attribute.Bool("app.operation_route_has_transaction_route_links", exists))

	return exists, nil
}

func (r *OperationRoutePostgreSQLRepository) FindTransactionRouteIDs(ctx context.Context, operationRouteID uuid.UUID) ([]uuid.UUID, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction_route_ids")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.operation_route_id", operationRouteID.String()))

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context finished before finding transaction route IDs", err)

		return nil, err
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection",
			libLog.Err(err),
			libLog.String("operation_route_id", operationRouteID.String()),
		)

		return nil, err
	}

	query, args, err := squirrel.Select("transaction_route_id").
		From("operation_transaction_route").
		Where(squirrel.Eq{"operation_route_id": operationRouteID, "deleted_at": nil}).
		OrderBy("created_at").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build transaction route IDs query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build transaction route IDs query",
			libLog.Err(err),
			libLog.String("operation_route_id", operationRouteID.String()),
		)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Built transaction route IDs query", libLog.String("query", query))

	_, spanQuery := tracer.Start(ctx, "postgres.find_transaction_route_ids.query")
	defer spanQuery.End()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to execute query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to execute query",
			libLog.Err(err),
			libLog.String("operation_route_id", operationRouteID.String()),
		)

		return nil, err
	}
	defer rows.Close()

	var transactionRouteIDs []uuid.UUID

	for rows.Next() {
		var transactionRouteID uuid.UUID

		if err := rows.Scan(&transactionRouteID); err != nil {
			libOpentelemetry.HandleSpanError(spanQuery, "Failed to scan transaction route ID", err)

			logger.Log(ctx, libLog.LevelError, "Failed to scan transaction route ID",
				libLog.Err(err),
				libLog.String("operation_route_id", operationRouteID.String()),
			)

			return nil, err
		}

		transactionRouteIDs = append(transactionRouteIDs, transactionRouteID)
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(spanQuery, "Failed to iterate rows", err)

		logger.Log(ctx, libLog.LevelError, "Failed to iterate rows",
			libLog.Err(err),
			libLog.String("operation_route_id", operationRouteID.String()),
		)

		return nil, err
	}

	spanQuery.SetAttributes(attribute.Int("db.rows_returned", len(transactionRouteIDs)))

	return transactionRouteIDs, nil
}

// splitMergePatch separates a raw JSON object into two parts for RFC 7396 merge-patch:
//   - mergeJSON: a JSON object containing only the keys with non-null values (for JSONB || merge)
//   - removeKeys: a list of keys whose value is explicitly null (for JSONB - removal)
//
// Example: {"direct":{"debit":{...}},"hold":null}
//   - mergeJSON = {"direct":{"debit":{...}}}
//   - removeKeys = ["hold"]
func splitMergePatch(raw json.RawMessage) (mergeJSON []byte, removeKeys []string) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		// If we can't parse it, return the raw as merge (fallback to full replacement)
		return raw, nil
	}

	merge := make(map[string]json.RawMessage, len(fields))

	for k, v := range fields {
		trimmed := strings.TrimSpace(string(v))
		if trimmed == "null" {
			removeKeys = append(removeKeys, k)
		} else {
			merge[k] = v
		}
	}

	if len(merge) > 0 {
		var err error
		if mergeJSON, err = json.Marshal(merge); err != nil {
			// If we can't re-serialize, fall back to the raw input (full replacement)
			return raw, nil
		}
	}

	return mergeJSON, removeKeys
}
