// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package testutil_integration

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/migration"
)

// ApplyFunctionMigrations applies all function migrations to the test database.
// This should be called before applying schema migrations to ensure functions are available for triggers.
func ApplyFunctionMigrations(ctx context.Context, db *sql.DB, functionsPath string) error {
	migrator := migration.NewFunctionMigrator(db, functionsPath, nil)

	if err := migrator.Up(ctx); err != nil {
		return fmt.Errorf("failed to apply function migrations: %w", err)
	}

	return nil
}
