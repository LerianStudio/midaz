// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// domainComponent is the bounded `component` label value for every D6 domain
// operation metric emitted by the reporter worker. Both reporter binaries
// share it so the metric family aggregates across the manager and worker.
const domainComponent = "reporter"

// Domain operation names (D6). This is the fixed, compile-time set required by
// T11: the `operation` label must never take a caller-derived value. Each
// public use-case entrypoint maps to exactly one of these constants.
const (
	opGenerateReport      = "generate_report"
	opProcessNotification = "process_notification"
)

// recordDomainOp emits the D6 domain operation metrics for one entrypoint
// completion against the fixed `reporter` component. Call it via defer with a
// named error so the final outcome (including business-vs-technical
// classification) is captured at the single exit boundary.
func (uc *UseCase) recordDomainOp(ctx context.Context, operation string, start time.Time, err error) {
	utils.RecordDomainOperation(ctx, uc.MetricsFactory, uc.Logger, domainComponent, operation, start, err)
}
