package cluster

import (
	"context"
	"database/sql"
	"errors"
	"github.com/LerianStudio/midaz/pkg/mpointers"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// Repository provides an interface for operations related to cluster entities.
//
//go:generate mockgen --destination=cluster.mock.go --package=cluster . Repository
type Repository interface {
	Create(ctx context.Context, cluster *mmodel.Cluster) (*mmodel.Cluster, error)
	FindByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Cluster, error)
	FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Cluster, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Cluster, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, cluster *mmodel.Cluster) (*mmodel.Cluster, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
}

// ClusterPostgreSQLRepository is a Postgresql-specific implementation of the Repository.
type ClusterPostgreSQLRepository struct {
	connection *mpostgres.PostgresConnection
	tableName  string
}

// NewClusterPostgreSQLRepository returns a new instance of ClusterPostgreSQLRepository using the given Postgres connection.
func NewClusterPostgreSQLRepository(pc *mpostgres.PostgresConnection) *ClusterPostgreSQLRepository {
	c := &ClusterPostgreSQLRepository{
		connection: pc,
		tableName:  "cluster",
	}

	_, err := c.connection.GetDB()
	if err != nil {
		panic("Failed to connect database")
	}

	return c
}

// Create a new cluster entity into Postgresql and returns it.
func (p *ClusterPostgreSQLRepository) Create(ctx context.Context, cluster *mmodel.Cluster) (*mmodel.Cluster, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.create_cluster")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &ClusterPostgreSQLModel{}
	record.FromEntity(cluster)

	ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "cluster_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert cluster record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, `INSERT INTO cluster VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *`,
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
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute insert query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Cluster{}).Name())
		}

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Cluster{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to create cluster. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// FindByName find cluster from the database using Organization and Ledger id and Name.
func (p *ClusterPostgreSQLRepository) FindByName(ctx context.Context, organizationID, ledgerID uuid.UUID, name string) (bool, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_cluster_by_name")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return false, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_cluster_by_name.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM cluster WHERE organization_id = $1 AND ledger_id = $2 AND name LIKE $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, name)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return false, err
	}
	defer rows.Close()

	spanQuery.End()

	if rows.Next() {
		err := pkg.ValidateBusinessError(constant.ErrDuplicateClusterName, reflect.TypeOf(mmodel.Cluster{}).Name(), name, ledgerID)

		mopentelemetry.HandleSpanError(&span, "Failed to find cluster by name", err)

		return true, err
	}

	return false, nil
}

// FindAll retrieves Cluster entities from the database.
func (p *ClusterPostgreSQLRepository) FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Cluster, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_all_clusters")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var clusters []*mmodel.Cluster

	findAll := squirrel.Select("*").
		From(p.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.GtOrEq{"created_at": pkg.NormalizeDate(filter.StartDate, mpointers.Int(-1))}).
		Where(squirrel.LtOrEq{"created_at": pkg.NormalizeDate(filter.EndDate, mpointers.Int(1))}).
		OrderBy("id " + strings.ToUpper(filter.SortOrder)).
		Limit(pkg.SafeIntToUint64(filter.Limit)).
		Offset(pkg.SafeIntToUint64((filter.Page - 1) * filter.Limit)).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := findAll.ToSql()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to build query", err)

		return nil, err
	}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_all.query")

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Cluster{}).Name())
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var cluster ClusterPostgreSQLModel
		if err := rows.Scan(&cluster.ID, &cluster.Name, &cluster.LedgerID, &cluster.OrganizationID,
			&cluster.Status, &cluster.StatusDescription, &cluster.CreatedAt, &cluster.UpdatedAt, &cluster.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		clusters = append(clusters, cluster.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return clusters, nil
}

// FindByIDs retrieves Clusters entities from the database using the provided IDs.
func (p *ClusterPostgreSQLRepository) FindByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Cluster, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_clusters_by_ids")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	var clusters []*mmodel.Cluster

	ctx, spanQuery := tracer.Start(ctx, "postgres.find_clusters_by_ids.query")

	rows, err := db.QueryContext(ctx, "SELECT * FROM cluster WHERE organization_id = $1 AND ledger_id = $2 AND id = ANY($3) AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, pq.Array(ids))
	if err != nil {
		mopentelemetry.HandleSpanError(&spanQuery, "Failed to execute query", err)

		return nil, err
	}
	defer rows.Close()

	spanQuery.End()

	for rows.Next() {
		var cluster ClusterPostgreSQLModel
		if err := rows.Scan(&cluster.ID, &cluster.Name, &cluster.LedgerID, &cluster.OrganizationID,
			&cluster.Status, &cluster.StatusDescription, &cluster.CreatedAt, &cluster.UpdatedAt, &cluster.DeletedAt); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

			return nil, err
		}

		clusters = append(clusters, cluster.ToEntity())
	}

	if err := rows.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan rows", err)

		return nil, err
	}

	return clusters, nil
}

// Find retrieves a Cluster entity from the database using the provided ID.
func (p *ClusterPostgreSQLRepository) Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Cluster, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_cluster")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	cluster := &ClusterPostgreSQLModel{}

	ctx, spanQuery := tracer.Start(ctx, "postgres.find.query")

	row := db.QueryRowContext(ctx, "SELECT * FROM cluster WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL ORDER BY created_at DESC",
		organizationID, ledgerID, id)

	spanQuery.End()

	if err := row.Scan(&cluster.ID, &cluster.Name, &cluster.LedgerID, &cluster.OrganizationID,
		&cluster.Status, &cluster.StatusDescription, &cluster.CreatedAt, &cluster.UpdatedAt, &cluster.DeletedAt); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to scan row", err)

		if errors.Is(err, sql.ErrNoRows) {
			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Cluster{}).Name())
		}

		return nil, err
	}

	return cluster.ToEntity(), nil
}

// Update a Cluster entity into Postgresql and returns the Cluster updated.
func (p *ClusterPostgreSQLRepository) Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, prd *mmodel.Cluster) (*mmodel.Cluster, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_cluster")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, err
	}

	record := &ClusterPostgreSQLModel{}
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

	query := `UPDATE cluster SET ` + strings.Join(updates, ", ") +
		` WHERE organization_id = $` + strconv.Itoa(len(args)-2) +
		` AND ledger_id = $` + strconv.Itoa(len(args)-1) +
		` AND id = $` + strconv.Itoa(len(args)) +
		` AND deleted_at IS NULL`

	ctx, spanExec := tracer.Start(ctx, "postgres.update.exec")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanExec, "cluster_repository_input", record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to convert cluster record from entity to JSON string", err)

		return nil, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute update query", err)

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Cluster{}).Name())
		}

		return nil, err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return nil, err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Cluster{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to update cluster. Rows affected is 0", err)

		return nil, err
	}

	return record.ToEntity(), nil
}

// Delete removes a Cluster entity from the database using the provided IDs.
func (p *ClusterPostgreSQLRepository) Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.delete_cluster")
	defer span.End()

	db, err := p.connection.GetDB()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.delete.exec")

	result, err := db.ExecContext(ctx, `UPDATE cluster SET deleted_at = now() WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanExec, "Failed to execute delete query", err)

		return err
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return err
	}

	if rowsAffected == 0 {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Cluster{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to delete cluster. Rows affected is 0", err)

		return err
	}

	return nil
}
