package postgres

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	i "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// InstrumentPostgreSQLRepository is a Postgresql-specific implementation of the InstrumentRepository.
type InstrumentPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewInstrumentPostgreSQLRepository returns a new instance of InstrumentPostgreSQLRepository using the given Postgres connection.
func NewInstrumentPostgreSQLRepository(pc *mpostgres.PostgresConnection) *InstrumentPostgreSQLRepository {
	c := &InstrumentPostgreSQLRepository{
		connection: pc,
		tableName:  "instrument",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new instrument entity into Postgresql and returns it.
func (r *InstrumentPostgreSQLRepository) Create(ctx context.Context, instrument *i.Instrument) (*i.Instrument, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &i.InstrumentPostgreSQLModel{}
	record.FromEntity(instrument)

	result, err := db.ExecContext(ctx, `INSERT INTO instrument VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *`,
		record.ID,
		record.Name,
		record.Type,
		record.Code,
		record.Status,
		record.StatusDescription,
		record.LedgerID,
		record.OrganizationID,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(i.Instrument{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(i.Instrument{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// FindByNameOrCode retrieves Instrument entities by nam or code from the database.
func (r *InstrumentPostgreSQLRepository) FindByNameOrCode(ctx context.Context, organizationID, ledgerID uuid.UUID, name, code string) (bool, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return false, err
	}

	rows, err := db.QueryContext(ctx, "SELECT * FROM instrument WHERE organization_id = $1 AND ledger_id = $2 AND name LIKE $3 OR code = $4 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, name, code)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	if rows.Next() {
		return true, common.EntityConflictError{
			EntityType: reflect.TypeOf(i.Instrument{}).Name(),
			Code:       "0003",
			Title:      "Invalid Data provided.",
			Message:    "Invalid Data provided.",
		}
	}

	return false, nil
}

// FindAll retrieves Instrument entities from the database.
func (r *InstrumentPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, limit, page int) ([]*i.Instrument, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var instruments []*i.Instrument

	findAll := sqrl.Select("*").
		From(r.tableName).
		Where(sqrl.Expr("organization_id = ?", organizationID)).
		Where(sqrl.Expr("ledger_id = ?", ledgerID)).
		Where(sqrl.Eq{"deleted_at": nil}).
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		Offset(uint64((page - 1) * limit)).
		PlaceholderFormat(sqrl.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var instrument i.InstrumentPostgreSQLModel
		if err := rows.Scan(&instrument.ID, &instrument.Name, &instrument.Type, &instrument.Code, &instrument.Status, &instrument.StatusDescription,
			&instrument.LedgerID, &instrument.OrganizationID, &instrument.CreatedAt, &instrument.UpdatedAt, &instrument.DeletedAt); err != nil {
			return nil, err
		}

		instruments = append(instruments, instrument.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return instruments, nil
}

// ListByIDs retrieves Instruments entities from the database using the provided IDs.
func (r *InstrumentPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*i.Instrument, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	var instruments []*i.Instrument

	rows, err := db.QueryContext(ctx, "SELECT * FROM instrument WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var instrument i.InstrumentPostgreSQLModel
		if err := rows.Scan(&instrument.ID, &instrument.Name, &instrument.Type, &instrument.Code, &instrument.Status, &instrument.StatusDescription,
			&instrument.LedgerID, &instrument.OrganizationID, &instrument.CreatedAt, &instrument.UpdatedAt, &instrument.DeletedAt); err != nil {
			return nil, err
		}

		instruments = append(instruments, instrument.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return instruments, nil
}

// Find retrieves an Instrument entity from the database using the provided ID.
func (r *InstrumentPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*i.Instrument, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	instrument := &i.InstrumentPostgreSQLModel{}

	row := db.QueryRowContext(ctx, "SELECT * FROM instrument WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL",
		organizationID, ledgerID, id)
	if err := row.Scan(&instrument.ID, &instrument.Name, &instrument.Type, &instrument.Code, &instrument.Status, &instrument.StatusDescription,
		&instrument.LedgerID, &instrument.OrganizationID, &instrument.CreatedAt, &instrument.UpdatedAt, &instrument.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(i.Instrument{}).Name(),
				Title:      "Entity not found.",
				Code:       "0007",
				Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
			}
		}

		return nil, err
	}

	return instrument.ToEntity(), nil
}

// Update an Instrument entity into Postgresql and returns the Instrument updated.
func (r *InstrumentPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, instrument *i.Instrument) (*i.Instrument, error) {
	db, err := r.connection.GetDB()
	if err != nil {
		return nil, err
	}

	record := &i.InstrumentPostgreSQLModel{}
	record.FromEntity(instrument)

	var updates []string

	var args []any

	if instrument.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if !instrument.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE instrument SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, app.ValidatePGError(pgErr, reflect.TypeOf(i.Instrument{}).Name())
		}

		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(i.Instrument{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// Delete removes an Instrument entity from the database using the provided IDs.
func (r *InstrumentPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	db, err := r.connection.GetDB()
	if err != nil {
		return err
	}

	result, err := db.ExecContext(ctx, `UPDATE instrument SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return common.EntityNotFoundError{
			EntityType: reflect.TypeOf(i.Instrument{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return nil
}
