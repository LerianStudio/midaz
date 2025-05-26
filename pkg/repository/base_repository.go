package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// Entity represents a base entity interface
type Entity interface {
	GetID() string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDeletedAt() *time.Time
}

// BaseEntity provides common fields for all entities
type BaseEntity struct {
	ID        string     `json:"id" db:"id"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// GetID returns the entity ID
func (e BaseEntity) GetID() string { return e.ID }

// GetCreatedAt returns the creation time
func (e BaseEntity) GetCreatedAt() time.Time { return e.CreatedAt }

// GetUpdatedAt returns the last update time
func (e BaseEntity) GetUpdatedAt() time.Time { return e.UpdatedAt }

// GetDeletedAt returns the deletion time
func (e BaseEntity) GetDeletedAt() *time.Time { return e.DeletedAt }

// QueryBuilder helps build SQL queries dynamically
type QueryBuilder struct {
	baseQuery   string
	conditions  []string
	args        []interface{}
	orderBy     string
	limit       int
	offset      int
}

// NewQueryBuilder creates a new query builder
func NewQueryBuilder(baseQuery string) *QueryBuilder {
	return &QueryBuilder{
		baseQuery:  baseQuery,
		conditions: []string{},
		args:       []interface{}{},
	}
}

// Where adds a WHERE condition
func (qb *QueryBuilder) Where(condition string, args ...interface{}) *QueryBuilder {
	qb.conditions = append(qb.conditions, condition)
	qb.args = append(qb.args, args...)
	return qb
}

// OrderBy sets the ORDER BY clause
func (qb *QueryBuilder) OrderBy(orderBy string) *QueryBuilder {
	qb.orderBy = orderBy
	return qb
}

// Limit sets the LIMIT clause
func (qb *QueryBuilder) Limit(limit int) *QueryBuilder {
	qb.limit = limit
	return qb
}

// Offset sets the OFFSET clause
func (qb *QueryBuilder) Offset(offset int) *QueryBuilder {
	qb.offset = offset
	return qb
}

// Build constructs the final SQL query
func (qb *QueryBuilder) Build() (string, []interface{}) {
	query := qb.baseQuery
	
	if len(qb.conditions) > 0 {
		query += " WHERE " + strings.Join(qb.conditions, " AND ")
	}
	
	if qb.orderBy != "" {
		query += " ORDER BY " + qb.orderBy
	}
	
	if qb.limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", qb.limit)
		if qb.offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", qb.offset)
		}
	}
	
	return query, qb.args
}

// BaseRepository provides common repository functionality
type BaseRepository[T Entity] struct {
	db         *sql.DB
	tx         *sql.Tx
	tableName  string
	entityName string
}

// NewBaseRepository creates a new base repository
func NewBaseRepository[T Entity](db *sql.DB, tableName, entityName string) *BaseRepository[T] {
	return &BaseRepository[T]{
		db:         db,
		tableName:  tableName,
		entityName: entityName,
	}
}

// GetTx returns the current transaction
func (r *BaseRepository[T]) GetTx() *sql.Tx {
	return r.tx
}

// SetTx sets the transaction
func (r *BaseRepository[T]) SetTx(tx *sql.Tx) {
	r.tx = tx
}

// getExecutor returns either the transaction or database
func (r *BaseRepository[T]) getExecutor() interface{} {
	if r.tx != nil {
		return r.tx
	}
	return r.db
}

// FindByID retrieves an entity by ID
func (r *BaseRepository[T]) FindByID(ctx context.Context, id uuid.UUID, dest T) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, fmt.Sprintf("%s.find_by_id", r.entityName))
	defer span.End()

	query := fmt.Sprintf("SELECT * FROM %s WHERE id = $1 AND deleted_at IS NULL", r.tableName)
	
	var err error
	if r.tx != nil {
		err = r.tx.QueryRowContext(ctx, query, id).Scan(dest)
	} else {
		err = r.db.QueryRowContext(ctx, query, id).Scan(dest)
	}

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warnf("%s not found: %s", r.entityName, id)
			return fmt.Errorf("%s not found", r.entityName)
		}
		libOpentelemetry.HandleSpanError(&span, fmt.Sprintf("Failed to find %s", r.entityName), err)
		return err
	}

	return nil
}

// FindAll retrieves all entities with pagination
func (r *BaseRepository[T]) FindAll(ctx context.Context, organizationID uuid.UUID, pagination http.Pagination, scanFunc func(*sql.Rows) (T, error)) ([]T, libHTTP.CursorPagination, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, fmt.Sprintf("%s.find_all", r.entityName))
	defer span.End()

	qb := NewQueryBuilder(fmt.Sprintf("SELECT * FROM %s", r.tableName))
	qb.Where("organization_id = $1", organizationID)
	qb.Where("deleted_at IS NULL")
	
	// Apply filters
	paramCount := 2
	for key, value := range pagination.Filters {
		qb.Where(fmt.Sprintf("%s = $%d", key, paramCount), value)
		paramCount++
	}
	
	// Apply sorting
	if pagination.SortBy != "" {
		order := "ASC"
		if strings.ToUpper(pagination.Order) == "DESC" {
			order = "DESC"
		}
		qb.OrderBy(fmt.Sprintf("%s %s", pagination.SortBy, order))
	} else {
		qb.OrderBy("created_at DESC")
	}
	
	// Apply limit
	qb.Limit(pagination.Limit + 1) // Fetch one extra to check if there's a next page
	
	// Apply cursor
	if pagination.Cursor != "" {
		// Decode cursor and add to query
		decodedCursor, err := libHTTP.DecodeCursor(pagination.Cursor)
		if err == nil && decodedCursor.LastID != "" {
			qb.Where(fmt.Sprintf("id > $%d", paramCount), decodedCursor.LastID)
			paramCount++
		}
	}
	
	query, args := qb.Build()
	
	var rows *sql.Rows
	var err error
	
	if r.tx != nil {
		rows, err = r.tx.QueryContext(ctx, query, args...)
	} else {
		rows, err = r.db.QueryContext(ctx, query, args...)
	}
	
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, fmt.Sprintf("Failed to query %s", r.entityName), err)
		logger.Errorf("Failed to query %s: %v", r.entityName, err)
		return nil, libHTTP.CursorPagination{}, err
	}
	defer rows.Close()
	
	var results []T
	for rows.Next() {
		entity, err := scanFunc(rows)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, fmt.Sprintf("Failed to scan %s", r.entityName), err)
			return nil, libHTTP.CursorPagination{}, err
		}
		results = append(results, entity)
	}
	
	// Check for iteration errors
	if err = rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)
		return nil, libHTTP.CursorPagination{}, err
	}
	
	// Build cursor pagination
	hasMore := len(results) > pagination.Limit
	if hasMore {
		results = results[:pagination.Limit] // Remove the extra item
	}
	
	var nextCursor string
	if hasMore && len(results) > 0 {
		lastEntity := results[len(results)-1]
		nextCursor = libHTTP.EncodeCursor(libHTTP.Cursor{
			LastID:    lastEntity.GetID(),
			LastValue: lastEntity.GetCreatedAt().Format(time.RFC3339),
		})
	}
	
	cursorPagination := libHTTP.CursorPagination{
		Next:   nextCursor,
		HasMore: hasMore,
		Count:  len(results),
	}
	
	logger.Infof("Found %d %s entities", len(results), r.entityName)
	return results, cursorPagination, nil
}

// Create inserts a new entity
func (r *BaseRepository[T]) Create(ctx context.Context, entity T) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, fmt.Sprintf("%s.create", r.entityName))
	defer span.End()

	// This is a simplified example - actual implementation would need
	// dynamic SQL generation based on entity fields
	logger.Infof("Creating new %s", r.entityName)
	
	return nil
}

// Update modifies an existing entity
func (r *BaseRepository[T]) Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, fmt.Sprintf("%s.update", r.entityName))
	defer span.End()

	if len(updates) == 0 {
		return nil
	}

	// Build UPDATE query
	var setClauses []string
	var args []interface{}
	argCount := 1

	for column, value := range updates {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, argCount))
		args = append(args, value)
		argCount++
	}

	// Add updated_at
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argCount))
	args = append(args, time.Now())
	argCount++

	// Add ID for WHERE clause
	args = append(args, id)

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = $%d AND deleted_at IS NULL",
		r.tableName,
		strings.Join(setClauses, ", "),
		argCount,
	)

	var result sql.Result
	var err error

	if r.tx != nil {
		result, err = r.tx.ExecContext(ctx, query, args...)
	} else {
		result, err = r.db.ExecContext(ctx, query, args...)
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, fmt.Sprintf("Failed to update %s", r.entityName), err)
		logger.Errorf("Failed to update %s: %v", r.entityName, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%s not found", r.entityName)
	}

	logger.Infof("Updated %s %s", r.entityName, id)
	return nil
}

// Delete performs a soft delete
func (r *BaseRepository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, fmt.Sprintf("%s.delete", r.entityName))
	defer span.End()

	query := fmt.Sprintf(
		"UPDATE %s SET deleted_at = $1, updated_at = $2 WHERE id = $3 AND deleted_at IS NULL",
		r.tableName,
	)

	now := time.Now()
	var result sql.Result
	var err error

	if r.tx != nil {
		result, err = r.tx.ExecContext(ctx, query, now, now, id)
	} else {
		result, err = r.db.ExecContext(ctx, query, now, now, id)
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, fmt.Sprintf("Failed to delete %s", r.entityName), err)
		logger.Errorf("Failed to delete %s: %v", r.entityName, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%s not found", r.entityName)
	}

	logger.Infof("Soft deleted %s %s", r.entityName, id)
	return nil
}

// Exists checks if an entity exists
func (r *BaseRepository[T]) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	query := fmt.Sprintf(
		"SELECT EXISTS(SELECT 1 FROM %s WHERE id = $1 AND deleted_at IS NULL)",
		r.tableName,
	)

	var exists bool
	var err error

	if r.tx != nil {
		err = r.tx.QueryRowContext(ctx, query, id).Scan(&exists)
	} else {
		err = r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	}

	if err != nil {
		return false, err
	}

	return exists, nil
}

// Count returns the total number of entities
func (r *BaseRepository[T]) Count(ctx context.Context, filters map[string]interface{}) (int64, error) {
	qb := NewQueryBuilder(fmt.Sprintf("SELECT COUNT(*) FROM %s", r.tableName))
	qb.Where("deleted_at IS NULL")

	paramCount := 1
	for key, value := range filters {
		qb.Where(fmt.Sprintf("%s = $%d", key, paramCount), value)
		paramCount++
	}

	query, args := qb.Build()

	var count int64
	var err error

	if r.tx != nil {
		err = r.tx.QueryRowContext(ctx, query, args...).Scan(&count)
	} else {
		err = r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	}

	if err != nil {
		return 0, err
	}

	return count, nil
}