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
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// PortfolioPostgreSQLRepository is a Postgresql-specific implementation of the PortfolioRepository.
type PortfolioPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
}

// NewPortfolioPostgreSQLRepository returns a new instance of PortfolioPostgreSQLRepository using the given Postgres connection.
func NewPortfolioPostgreSQLRepository(pc *mpostgres.PostgresConnection) *PortfolioPostgreSQLRepository {
	c := &PortfolioPostgreSQLRepository{
		connection: pc,
	}

	_, err := c.connection.GetDB(context.Background())
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new portfolio entity into Postgresql and returns it.
func (r *PortfolioPostgreSQLRepository) Create(ctx context.Context, portfolio *p.Portfolio) (*p.Portfolio, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	record := &p.PortfolioPostgreSQLModel{}
	record.FromEntity(portfolio)

	result, err := db.ExecContext(ctx, `INSERT INTO portfolio (id, name, entity_id, ledger_id, organization_id, 
        status, status_description, created_at, updated_at, deleted_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *`,
		record.ID, record.Name, record.EntityID, record.LedgerID, record.OrganizationID,
		record.Status, record.StatusDescription, record.CreatedAt, record.UpdatedAt, record.DeletedAt)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// FindByIDEntity find portfolio from the database using the Entity id.
func (r *PortfolioPostgreSQLRepository) FindByIDEntity(ctx context.Context, organizationID, ledgerID, entityID uuid.UUID) (*p.Portfolio, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	portfolio := &p.PortfolioPostgreSQLModel{}

	row := db.QueryRowContext(ctx, "SELECT * FROM portfolio WHERE organization_id = $1 AND ledger_id = $2 AND entity_id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, entityID)
	if err := row.Scan(&portfolio.ID, &portfolio.Name, &portfolio.EntityID, &portfolio.LedgerID, &portfolio.OrganizationID,
		&portfolio.Status, &portfolio.StatusDescription, &portfolio.CreatedAt, &portfolio.UpdatedAt, &portfolio.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
				Title:      "Entity not found.",
				Code:       "0007",
				Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
			}
		}

		return nil, err
	}

	return portfolio.ToEntity(), nil
}

// FindAll retrieves Portfolio entities from the database.
func (r *PortfolioPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]*p.Portfolio, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	var portfolios []*p.Portfolio

	rows, err := db.QueryContext(ctx, "SELECT * FROM portfolio WHERE organization_id = $1 AND ledger_id = $2 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID)
	if err != nil {
		return nil, common.EntityNotFoundError{
			EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}
	defer rows.Close()

	for rows.Next() {
		var portfolio p.PortfolioPostgreSQLModel
		if err := rows.Scan(&portfolio.ID, &portfolio.Name, &portfolio.EntityID, &portfolio.LedgerID, &portfolio.OrganizationID,
			&portfolio.Status, &portfolio.StatusDescription, &portfolio.CreatedAt, &portfolio.UpdatedAt, &portfolio.DeletedAt); err != nil {
			return nil, err
		}

		portfolios = append(portfolios, portfolio.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return portfolios, nil
}

// Find retrieves a Portfolio entity from the database using the provided ID.
func (r *PortfolioPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*p.Portfolio, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	portfolio := &p.PortfolioPostgreSQLModel{}

	row := db.QueryRowContext(ctx, "SELECT * FROM portfolio WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, id)
	if err := row.Scan(&portfolio.ID, &portfolio.Name, &portfolio.EntityID, &portfolio.LedgerID, &portfolio.OrganizationID,
		&portfolio.Status, &portfolio.StatusDescription, &portfolio.CreatedAt, &portfolio.UpdatedAt, &portfolio.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.EntityNotFoundError{
				EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
				Title:      "Entity not found.",
				Code:       "0007",
				Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
			}
		}

		return nil, err
	}

	return portfolio.ToEntity(), nil
}

// ListByIDs retrieves Portfolios entities from the database using the provided IDs.
func (r *PortfolioPostgreSQLRepository) ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*p.Portfolio, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	var portfolios []*p.Portfolio

	rows, err := db.QueryContext(ctx, "SELECT * FROM portfolio WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var portfolio p.PortfolioPostgreSQLModel
		if err := rows.Scan(&portfolio.ID, &portfolio.Name, &portfolio.EntityID, &portfolio.LedgerID, &portfolio.OrganizationID,
			&portfolio.Status, &portfolio.StatusDescription, &portfolio.CreatedAt, &portfolio.UpdatedAt, &portfolio.DeletedAt); err != nil {
			return nil, err
		}

		portfolios = append(portfolios, portfolio.ToEntity())
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return portfolios, nil
}

// Update a Portfolio entity into Postgresql and returns the Portfolio updated.
func (r *PortfolioPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, portfolio *p.Portfolio) (*p.Portfolio, error) {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	record := &p.PortfolioPostgreSQLModel{}
	record.FromEntity(portfolio)

	var updates []string

	var args []any

	if portfolio.Name != "" {
		updates = append(updates, "name = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Name)
	}

	if !portfolio.Status.IsEmpty() {
		updates = append(updates, "status = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.Status)

		updates = append(updates, "status_description = $"+strconv.Itoa(len(args)+1))
		args = append(args, record.StatusDescription)
	}

	record.UpdatedAt = time.Now()

	updates = append(updates, "updated_at = $"+strconv.Itoa(len(args)+1))

	args = append(args, record.UpdatedAt, organizationID, ledgerID, id)

	query := `UPDATE portfolio SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
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
			EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return record.ToEntity(), nil
}

// Delete removes a Portfolio entity from the database using the provided IDs.
func (r *PortfolioPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	db, err := r.connection.GetDB(ctx)
	if err != nil {
		return err
	}

	result, err := db.ExecContext(ctx, `UPDATE portfolio SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
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
			EntityType: reflect.TypeOf(p.Portfolio{}).Name(),
			Title:      "Entity not found.",
			Code:       "0007",
			Message:    "No entity was found matching the provided ID. Ensure the correct ID is being used for the entity you are attempting to manage.",
		}
	}

	return nil
}
