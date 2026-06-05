// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache

//go:generate mockgen -source=rule_sync_repository.go -destination=mocks/rule_sync_repository_mock.go -package=mocks

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// RuleSyncRepository provides database queries for the cache sync system.
// Interface defined in the consuming package (per PROJECT_RULES.md).
type RuleSyncRepository interface {
	// GetAllActiveRules retrieves all rules with status=ACTIVE for initial warm-up.
	GetAllActiveRules(ctx context.Context) ([]*model.Rule, error)

	// GetRulesUpdatedSince retrieves all rules updated at or after the given timestamp.
	// Returns ALL statuses to detect deactivations/deletions.
	GetRulesUpdatedSince(ctx context.Context, since time.Time) ([]*model.Rule, error)
}
