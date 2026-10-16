// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	configCopyFamilyBalance = "balance"
	configCopyFamilyAccount = "account"

	// configCopyOutcomeFailed labels a copy that ended in a Redis or blob error, as
	// opposed to one that kept losing its compare-and-set (conflict).
	configCopyOutcomeFailed = "failed"
)

// recordConfigBlobCopies counts the configuration copies one PATCH confirmed or
// attempted in the balance blobs (utils.ConfigBlobCopy), by family and outcome, so
// a copy that lost its compare-and-set can be told from a Redis failure without a
// trace. failed counts the copies that ended in an error. Best-effort: a metric
// failure never affects the PATCH.
func (uc *UseCase) recordConfigBlobCopies(ctx context.Context, logger libLog.Logger, family string, outcomes []redis.BlobCopyOutcome, failed int) {
	if uc.MetricsFactory == nil || (len(outcomes) == 0 && failed == 0) {
		return
	}

	counts := make(map[string]int64, len(outcomes)+1)
	for _, outcome := range outcomes {
		counts[outcome.String()]++
	}

	if failed > 0 {
		counts[configCopyOutcomeFailed] += int64(failed)
	}

	counter, err := uc.MetricsFactory.Counter(utils.ConfigBlobCopy)
	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to create configuration blob copy counter", libLog.Err(err))

		return
	}

	for outcome, count := range counts {
		if addErr := counter.WithLabels(map[string]string{"family": family, "outcome": outcome}).Add(ctx, count); addErr != nil {
			logger.Log(ctx, libLog.LevelDebug, "Failed to emit configuration blob copy counter", libLog.Err(addErr))
		}
	}
}
