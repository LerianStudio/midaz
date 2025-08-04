package segment

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// Repository provides an interface for operations related to segment entities.
// It defines methods for creating, finding, updating, and deleting segments in the database.
type Repository interface {
	Create(ctx context.Context, segment *mmodel.Segment) (*mmodel.Segment, error)
	FindByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Segment, error)
	FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Segment, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Segment, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, segment *mmodel.Segment) (*mmodel.Segment, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error)
}

// SegmentPostgreSQLRepository is a Postgresql-specific implementation of the Repository.
type SegmentPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewSegmentPostgreSQLRepository returns a new instance of SegmentPostgreSQLRepository using the given Postgres connection.
func NewSegmentPostgreSQLRepository(pc *libPostgres.PostgresConnection) *SegmentPostgreSQLRepository {
	c := &SegmentPostgreSQLRepository{
		connection: pc,
		tableName:  "segment",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new segment entity into Postgresql and returns it.
func (p *SegmentPostgreSQLRepository) Create(ctx context.Context, segment *mmodel.Segment) (*mmodel.Segment, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_segment")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &SegmentPostgreSQLModel{}
	record.FromEntity(segment)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "segment_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert segment record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO segment VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *`,
		record.ID,
		record.Name,
		record.LedgerID,
		record.OrganizationID,
		record.Status,
		record.StatusDescription,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Segment{}).Name())
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Segment{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to create segment. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByName find segment from the database using Organization and Ledger id and Name.
func (p *SegmentPostgreSQLRepository) FindByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_segment_by_name")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_segment_by_name.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM segment WHERE organization_id = $1 AND ledger_id = $2 AND name LIKE $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, name)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrDuplicateSegmentName, reflect.TypeOf(mmodel.Segment{}).Name(), name, ledgerID)

		libOpentelemetry.HandleSpanError(&span, "Failed to find segment by name", err)

		return true, err
	}

	return false, nil
}

// FindAll retrieves Segment entities from the database.
func (p *SegmentPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Segment, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_segments")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var segments []*mmodel.Segment

	findAll := squirrel.Select("*").
		From(p.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": libCommons.NormalizeDateTime(filter.StartDate, libPointers.Int(0), false)}).
		Where(squirrel.LtOrEq{"created_at": libCommons.NormalizeDateTime(filter.EndDate, libPointers.Int(0), true)}).
		OrderBy("id " + strings.ToUpper(filter.SortOrder)).
		Limit(libCommons.SafeIntToUint64(filter.Limit)).
		Offset(libCommons.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Segment{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var segment SegmentPostgreSQLModel
		if err := rows.Scan(&segment.ID, &segment.Name, &segment.LedgerID, &segment.OrganizationID,
			&segment.Status, &segment.StatusDescription, &segment.CreatedAt, &segment.UpdatedAt, &segment.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		segments = append(segments, segment.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return segments, nil
}

// FindByIDs retrieves Segments entities from the database using the provided IDs.
func (p *SegmentPostgreSQLRepository) FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Segment, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_segments_by_ids")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var segments []*mmodel.Segment

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_segments_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM segment WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var segment SegmentPostgreSQLModel
		if err := rows.Scan(&segment.ID, &segment.Name, &segment.LedgerID, &segment.OrganizationID,
			&segment.Status, &segment.StatusDescription, &segment.CreatedAt, &segment.UpdatedAt, &segment.DeletedAt); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		segments = append(segments, segment.ToEntity())
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return segments, nil
}

// Find retrieves a Segment entity from the database using the provided ID.
func (p *SegmentPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Segment, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_segment")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	segment := &SegmentPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM segment WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, id)

	spanQuery.End()

	if err := row.Scan(&segment.ID, &segment.Name, &segment.LedgerID, &segment.OrganizationID,
		&segment.Status, &segment.StatusDescription, &segment.CreatedAt, &segment.UpdatedAt, &segment.DeletedAt); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Segment{}).Name())
		}

		return nil, err
	}

	return segment.ToEntity(), nil
}

// Update a Segment entity into Postgresql and returns the Segment updated.
func (p *SegmentPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, prd *mmodel.Segment) (*mmodel.Segment, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_segment")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &SegmentPostgreSQLModel{}
	record.FromEntity(prd)

	var updates []string

	var args []any

	if prd.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if !prd.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE segment SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = libOpentelemetry.SetSpanAttributesFromStruct(&spanExec, "segment_repository_input", record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to convert segment record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Segment{}).Name())
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Segment{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to update segment. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Segment entity from the database using the provided IDs.
func (p *SegmentPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_segment")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE segment SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
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
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Segment{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to delete segment. Rows affected is 0", err)

		return err
	}

	return nil
}

// Count retrieves the number of Segment entities in the database.
func (p *SegmentPostgreSQLRepository) Count(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.count_segments")
	defer span.End()

	var count = int64(0)

	db, err := p.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return count, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.count.query")
	defer spanQuery.End()

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM segment WHERE organization_id = $1 AND ledger_id = $2 AND deleted_at IS NULL", organizationID, ledgerID).Scan(&count)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to execute query", err)

		return count, err
	}

	return count, nil
}
