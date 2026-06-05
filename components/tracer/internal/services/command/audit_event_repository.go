// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

//go:generate mockgen -source=audit_event_repository.go -destination=mocks/audit_event_repository_mock.go -package=mocks

// AuditEventRepository defines the write interface for audit event persistence.
type AuditEventRepository interface {
	Insert(ctx context.Context, event *model.AuditEvent) error

	// InsertWithTx inserts an audit event using the provided database connection.
	// This allows callers to pass either a regular DB connection or a transaction (*sql.Tx),
	// enabling atomic operations with other database changes.
	InsertWithTx(ctx context.Context, db pgdb.DB, event *model.AuditEvent) error
}
