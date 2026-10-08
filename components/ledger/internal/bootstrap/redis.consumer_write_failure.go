// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	sqlStateClassDataException                = "22"
	sqlStateClassIntegrityConstraintViolation = "23"
)

// isDeterministicReplayWriteFailure reports whether a legacy replay write failed
// because of the record's own content, so the same record can never be written
// and belongs in the quarantine flow. Anything not positively identified is
// treated as transient: quarantining a sound record during an outage costs an
// operator intervention, while missing a poison record only keeps it retrying.
//
// A unique violation is excluded because it is the writer's idempotent path.
// Schema errors (class 42) are excluded because they can appear mid-migration.
func isDeterministicReplayWriteFailure(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if errors.Is(err, command.ErrInvalidOperationDirection) {
		return true
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || len(pgErr.Code) != 5 {
		return false
	}

	switch pgErr.Code[:2] {
	case sqlStateClassDataException:
		return true
	case sqlStateClassIntegrityConstraintViolation:
		return pgErr.Code != constant.UniqueViolationCode
	default:
		return false
	}
}
