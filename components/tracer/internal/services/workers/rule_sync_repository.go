// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

//go:generate mockgen -source=rule_sync_repository.go -destination=mocks/rule_sync_repository_mock.go -package=mocks

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// RuleSyncRepository provides the delta query needed by the sync worker.
// Returns ALL statuses (not just ACTIVE) so the worker can detect deactivations.
// Satisfied by internal/adapters/postgres/rule_sync_repository.go.
type RuleSyncRepository interface {
	// GetRulesUpdatedSince retrieves all rules updated at or after the given timestamp.
	// Returns ALL statuses to detect deactivations/deletions.
	GetRulesUpdatedSince(ctx context.Context, since time.Time) ([]*model.Rule, error)
}
