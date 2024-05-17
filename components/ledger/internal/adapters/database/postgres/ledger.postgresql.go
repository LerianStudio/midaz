package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// LedgerPostgreSQLRepository is a Postgresql-specific implementation of the LedgerRepository.
type LedgerPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
}

// NewLedgerPostgreSQLRepository returns a new instance of LedgerPostgresRepository using the given Postgres connection.
func NewLedgerPostgreSQLRepository(pc *mpostgres.PostgresConnection) *LedgerPostgreSQLRepository {
	c := &LedgerPostgreSQLRepository{
		connection: pc,
	}

	_, err := c.connection.GetDB(context.Background())
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new Ledger entity into Postgresql and returns it.
func (r *LedgerPostgreSQLRepository) Create(ctx context.Context, ledger *l.Ledger) (*l.Ledger, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	record := &l.LedgerPostgreSQLModel{}
	record.FromEntity(ledger)

	result, err := db.ExecContext(ctx, `INSERT INTO ledger (id, name, organization_id, status, status_description, created_at, updated_at, deleted_at) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *`,
		record.ID, record.Name, record.OrganizationID, record.Status, record.StatusDescription, record.CreatedAt, record.UpdatedAt, record.DeletedAt)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(l.Ledger{}).Name(),
		}
	}

	return record.ToEntity(), nil
}

// Find retrieves a Ledger entity from the database using the provided ID.
func (r *LedgerPostgreSQLRepository) Find(ctx context.Context, organizationID, id uuid.UUID) (*l.Ledger, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	ledger := &l.LedgerPostgreSQLModel{}

	var status string

	row := db.QueryRowContext(ctx, "SELECT * FROM ledger WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL", organizationID, id)
	if err := row.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
		&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(l.Ledger{}).Name(),
				Title:      "Entity not found.",
				Code:       "0007",
				Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
			}
		}

		return nil, err
	}

	err = json.Unmarshal([]byte(status), &ledger.Status)
	if err != nil {
		return nil, err
	}

	return ledger.ToEntity(), nil
}

// FindAll retrieves Ledgers entities from the database.
func (r *LedgerPostgreSQLRepository) FindAll(ctx context.Context, organizationID uuid.UUID) ([]*l.Ledger, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	var ledgers []*l.Ledger

	rows, err := db.QueryContext(ctx, "SELECT * FROM ledger WHERE organization_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC", organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ledger l.LedgerPostgreSQLModel

		var status string

		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(status), &ledger.Status)
		if err != nil {
			return nil, err
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ledgers, nil
}

// ListByIDs retrieves Ledgers entities from the database using the provided IDs.
func (r *LedgerPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID uuid.UUID, ids []uuid.UUID) ([]*l.Ledger, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	var ledgers []*l.Ledger

	rows, err := db.QueryContext(ctx, "SELECT * FROM ledger WHERE organization_id = $1 AND id = ANY($2) AND deleted_at IS NULL ORDER BY created_at DESC", organizationID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ledger l.LedgerPostgreSQLModel

		var status string

		if err := rows.Scan(&ledger.ID, &ledger.Name, &ledger.OrganizationID, &ledger.Status, &ledger.StatusDescription,
			&ledger.CreatedAt, &ledger.UpdatedAt, &ledger.DeletedAt); err != nil {
			return nil, err
		}

		err = json.Unmarshal([]byte(status), &ledger.Status)
		if err != nil {
			return nil, err
		}

		ledgers = append(ledgers, ledger.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ledgers, nil
}

// Update a Ledger entity into Postgresql and returns the Ledger updated.
func (r *LedgerPostgreSQLRepository) Update(ctx context.Context, organizationID, id uuid.UUID, ledger *l.Ledger) (*l.Ledger, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	record := &l.LedgerPostgreSQLModel{}
	record.FromEntity(ledger)

	var updates []string

	var args []any

	if ledger.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if ledger.OrganizationID != "" {
		updates = append(updates, "organization_id = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.OrganizationID)
	}

	if !ledger.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, id)

	query := `UPDATE ledger SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(l.Ledger{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// Delete removes a Ledger entity from the database using the provided ID.
func (r *LedgerPostgreSQLRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return err
	}

	result, err := db.ExecContext(ctx, `UPDATE ledger SET deleted_at = now() WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL`, organizationID, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return common.EntityNotFoundError{
			EntityType: reflect.TypeOf(l.Ledger{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return nil
}
