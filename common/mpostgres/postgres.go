package mpostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"

	"go.uber.org/zap"

	"github.com/bxcodec/dbresolver/v2"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"

	// File system migration source. We need to import it to be able to use it as source in migrate.NewWithSourceInstance
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// PostgresConnection is a hub which deal with postgres connections.
type PostgresConnection struct {
	ConnectionStringPrimary string
	ConnectionStringReplica string
	PrimaryDBName           string
	ReplicaDBName           string
	ConnectionDB            *dbresolver.DB
	Connected               bool
}

// Connect keeps a singleton connection with postgres.
func (pc *PostgresConnection) Connect() error {
	fmt.Println("Connecting to primary and replica databases...")

	dbPrimary, err := sql.Open("pgx", pc.ConnectionStringPrimary)
	if err != nil {
		log.Fatal("failed to open connect to primary database", zap.Error(err))
		return nil
	}

	dbReadOnlyReplica, err := sql.Open("pgx", pc.ConnectionStringReplica)
	if err != nil {
		log.Fatal("failed to open connect to replica database", zap.Error(err))
		return nil
	}

	connectionDB := dbresolver.New(
		dbresolver.WithPrimaryDBs(dbPrimary),
		dbresolver.WithReplicaDBs(dbReadOnlyReplica),
		dbresolver.WithLoadBalancer(dbresolver.RoundRobinLB))

	migrationsPath, err := filepath.Abs(filepath.Join("components", "ledger", "migrations"))
	if err != nil {
		log.Fatal("failed get filepath",
			zap.Error(err))

		return err
	}

	primaryURL, err := url.Parse(filepath.ToSlash(migrationsPath))
	if err != nil {
		log.Fatal("failed parse url",
			zap.Error(err))

		return err
	}

	primaryURL.Scheme = "file"

	primaryDriver, err := postgres.WithInstance(dbPrimary, &postgres.Config{
		MultiStatementEnabled: true,
		DatabaseName:          pc.PrimaryDBName,
		SchemaName:            "public",
	})
	if err != nil {
		log.Fatalf("failed to open connect to database %v", zap.Error(err))
		return nil
	}

	m, err := migrate.NewWithDatabaseInstance(primaryURL.String(), pc.PrimaryDBName, primaryDriver)
	if err != nil {
		log.Fatal("failed to get migrations",
			zap.Error(err))

		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	if err := connectionDB.Ping(); err != nil {
		log.Printf("PostgresConnection.Ping %v",
			zap.Error(err))

		return err
	}

	pc.Connected = true
	pc.ConnectionDB = &connectionDB

	fmt.Println("Connected to postgres ✅ ")

	return nil
}

// GetDB returns a pointer to the postgres connection, initializing it if necessary.
func (pc *PostgresConnection) GetDB(ctx context.Context) (dbresolver.DB, error) {
	if pc.ConnectionDB == nil {
		if err := pc.Connect(); err != nil {
			log.Printf("ERRCONECT %s", err)
			return nil, err
		}
	}

	return *pc.ConnectionDB, nil
}
