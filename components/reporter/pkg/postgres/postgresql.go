// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"strings"
	"sync"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"

	"github.com/LerianStudio/lib-observability/log"
	_ "github.com/jackc/pgx/v5/stdlib" // Registers the "pgx" driver with database/sql via init() – required for sql.Open("pgx", ...)
)

// Connection is a hub which deals with postgres connections.
type Connection struct {
	ConnectionString   string
	DBName             string
	ConnectionDB       *sql.DB
	Connected          bool
	Logger             log.Logger
	MaxOpenConnections int
	MaxIdleConnections int

	mu sync.Mutex
}

// Connect initializes the connection with the PostgreSQL DB.
func (c *Connection) Connect() error {
	c.Logger.Log(context.Background(), log.LevelInfo, "Connecting to PostgreSQL...")

	db, err := sql.Open("pgx", c.ConnectionString)
	if err != nil {
		c.Logger.Log(context.Background(), log.LevelError, "Error opening connection", log.Err(err))
		return err
	}

	if err := db.Ping(); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			c.Logger.Log(context.Background(), log.LevelError, "Error closing connection", log.Err(closeErr))
		}

		c.Logger.Log(context.Background(), log.LevelError, "Error pinging PostgreSQL", log.Err(err))

		return err
	}

	db.SetMaxOpenConns(c.MaxOpenConnections)
	db.SetMaxIdleConns(c.MaxIdleConnections)
	db.SetConnMaxLifetime(constant.PostgresConnMaxLifetime)
	db.SetConnMaxIdleTime(constant.PostgresConnMaxIdleTime)

	c.ConnectionDB = db
	c.Connected = true
	c.Logger.Log(context.Background(), log.LevelInfo, "Connected to PostgreSQL",
		log.String("database", c.DBName), log.Int("max_open", c.MaxOpenConnections),
		log.Int("max_idle", c.MaxIdleConnections), log.Any("max_lifetime", constant.PostgresConnMaxLifetime),
		log.Any("max_idle_time", constant.PostgresConnMaxIdleTime))

	return nil
}

// GetDB returns a pointer to the postgres connection, initializing it if necessary.
func (pc *Connection) GetDB() (*sql.DB, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.ConnectionDB == nil {
		if err := pc.Connect(); err != nil {
			pc.Logger.Log(context.Background(), log.LevelError, "Error connecting", log.Err(err))
			return nil, err
		}
	}

	return pc.ConnectionDB, nil
}

// ValidateFieldsInSchemaPostgres validate if all fields exist on postgres schema table.
// Supports nested JSONB field paths like "fee_charge.totalAmount" where "fee_charge" is the column
// and "totalAmount" is a path inside the JSONB. In this case, only the root column is validated.
func ValidateFieldsInSchemaPostgres(expectedFields []string, schema TableSchema, countIfTableExist *int32) (missing []string) {
	columnSet := make(map[string]struct{}, len(schema.Columns))
	for _, col := range schema.Columns {
		columnSet[strings.ToLower(col.Name)] = struct{}{}
	}

	for _, field := range expectedFields {
		*countIfTableExist++ // variable to count if a table exists on a schema list

		// Handle nested JSONB field paths (e.g., "fee_charge.totalAmount")
		// Extract the root column name to validate against the schema
		fieldToCheck := field
		if dotIdx := strings.Index(field, "."); dotIdx != -1 {
			// This is a nested field path - validate only the root column
			fieldToCheck = field[:dotIdx]
		}

		if _, exists := columnSet[strings.ToLower(fieldToCheck)]; !exists {
			missing = append(missing, field)
		}
	}

	return
}
