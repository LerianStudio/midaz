package mpostgres

import (
	"database/sql"
	"errors"
	"go.uber.org/zap"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	// File system migration source. We need to import it to be able to use it as source in migrate.NewWithSourceInstance

	"github.com/LerianStudio/midaz/pkg/mlog"

	"github.com/bxcodec/dbresolver/v2"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresConnection is a hub which deal with postgres connections.
type PostgresConnection struct {
	ConnectionStringPrimary string
	ConnectionStringReplica string
	PrimaryDBName           string
	ReplicaDBName           string
	ConnectionDB            *dbresolver.DB
	Connected               bool
	Component               string
	Logger                  mlog.Logger
	MaxOpenConnections      int
	MaxIdleConnections      int
}

// Connect keeps a singleton connection with postgres.
func (pc *PostgresConnection) Connect() error {
	pc.Logger.Info("Connecting to primary and replica databases...")

	dbPrimary, err := sql.Open("pgx", pc.ConnectionStringPrimary)
	if err != nil {
		pc.Logger.Fatal("failed to open connect to primary database", zap.Error(err))
		return nil
	}

	dbPrimary.SetMaxOpenConns(pc.MaxOpenConnections)
	dbPrimary.SetMaxIdleConns(pc.MaxIdleConnections)
	dbPrimary.SetConnMaxLifetime(time.Minute * 30)

	dbReadOnlyReplica, err := sql.Open("pgx", pc.ConnectionStringReplica)
	if err != nil {
		pc.Logger.Fatal("failed to open connect to replica database", zap.Error(err))
		return nil
	}

	dbReadOnlyReplica.SetMaxOpenConns(pc.MaxOpenConnections)
	dbReadOnlyReplica.SetMaxIdleConns(pc.MaxIdleConnections)
	dbReadOnlyReplica.SetConnMaxLifetime(time.Minute * 30)

	connectionDB := dbresolver.New(
		dbresolver.WithPrimaryDBs(dbPrimary),
		dbresolver.WithReplicaDBs(dbReadOnlyReplica),
		dbresolver.WithLoadBalancer(dbresolver.RoundRobinLB))

	migrationsPath, err := filepath.Abs(filepath.Join("components", pc.Component, "migrations"))
	if err != nil {
		pc.Logger.Fatal("failed get filepath",
			zap.Error(err))

		return err
	}

	primaryURL, err := url.Parse(filepath.ToSlash(migrationsPath))
	if err != nil {
		pc.Logger.Fatal("failed parse url",
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
		pc.Logger.Fatalf("failed to open connect to database %v", zap.Error(err))
		return nil
	}

	m, err := migrate.NewWithDatabaseInstance(primaryURL.String(), pc.PrimaryDBName, primaryDriver)
	if err != nil {
		pc.Logger.Fatal("failed to get migrations",
			zap.Error(err))

		return err
	}

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			pc.Logger.Info("No new migrations found. Skipping...")
		} else if strings.Contains(err.Error(), "file does not exist") {
			pc.Logger.Warn("No migration files found. Skipping migration step...")
		} else {
			pc.Logger.Error("Migration failed", zap.Error(err))
			return err
		}
	}

	if err := connectionDB.Ping(); err != nil {
		pc.Logger.Infof("PostgresConnection.Ping %v",
			zap.Error(err))

		return err
	}

	pc.Connected = true
	pc.ConnectionDB = &connectionDB

	pc.Logger.Info("Connected to postgres ✅ \n")

	return nil
}

// GetDB returns a pointer to the postgres connection, initializing it if necessary.
func (pc *PostgresConnection) GetDB() (dbresolver.DB, error) {
	if pc.ConnectionDB == nil {
		if err := pc.Connect(); err != nil {
			pc.Logger.Infof("ERRCONECT %s", err)
			return nil, err
		}
	}

	return *pc.ConnectionDB, nil
}
