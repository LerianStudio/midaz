// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestIsDeterministicReplayWriteFailure(t *testing.T) {
	t.Parallel()

	pgError := func(code string) error {
		return fmt.Errorf("create transaction: %w", &pgconn.PgError{Code: code})
	}

	tests := []struct {
		name          string
		err           error
		deterministic bool
	}{
		{name: "invalid operation direction", err: fmt.Errorf("operation x: %w", command.ErrInvalidOperationDirection), deterministic: true},
		{name: "string data right truncation", err: pgError("22001"), deterministic: true},
		{name: "numeric value out of range", err: pgError("22003"), deterministic: true},
		{name: "invalid byte sequence", err: pgError("22021"), deterministic: true},
		{name: "invalid text representation", err: pgError("22P02"), deterministic: true},
		{name: "not null violation", err: pgError("23502"), deterministic: true},
		{name: "foreign key violation", err: pgError("23503"), deterministic: true},
		{name: "check violation", err: pgError("23514"), deterministic: true},
		{name: "exclusion violation", err: pgError("23P01"), deterministic: true},

		{name: "unique violation is the idempotent path", err: pgError("23505")},
		{name: "connection failure", err: pgError("08006")},
		{name: "too many connections", err: pgError("53300")},
		{name: "query canceled", err: pgError("57014")},
		{name: "admin shutdown", err: pgError("57P01")},
		{name: "serialization failure", err: pgError("40001")},
		{name: "deadlock", err: pgError("40P01")},
		{name: "undefined table during a migration", err: pgError("42P01")},
		{name: "deadline exceeded", err: fmt.Errorf("write: %w", context.DeadlineExceeded)},
		{name: "canceled", err: context.Canceled},
		{name: "data exception after the deadline expired", err: errors.Join(context.DeadlineExceeded, pgError("22021"))},
		{name: "untyped error", err: errors.New("transaction database unavailable")},
		{name: "no error", err: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.deterministic, isDeterministicReplayWriteFailure(tt.err))
		})
	}
}
