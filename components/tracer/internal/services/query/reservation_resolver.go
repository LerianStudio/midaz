// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// ReservationSpec is one counter-backed limit's resolved reservation parameters.
// It carries everything the reservation row and the reserve CTE need so confirm
// and release never re-query limits (R38): the row stores LimitID/ScopeKey/
// PeriodKey/Amount, and the reserve guard uses MaxAmount. Amounts are the smallest
// currency unit (cents), matching the BIGINT counter columns.
type ReservationSpec struct {
	LimitID   uuid.UUID
	ScopeKey  string
	PeriodKey string
	Amount    int64
	MaxAmount int64
}

// ResolveReservations resolves the applicable limits for a transaction ONCE and
// computes the per-limit reservation parameters for counter-backed limits
// (DAILY/WEEKLY/MONTHLY/CUSTOM). It mirrors the resolution and scope-key logic of
// processLimitAtomic without touching any counter, so the reserve service can hold
// capacity in reserved_usage instead of committing it.
//
// Decision precedence matches the synchronous Validate path:
//   - A PER_TRANSACTION limit whose maxAmount is exceeded denies immediately.
//   - A counter-backed limit whose amount alone exceeds maxAmount denies immediately
//     (the reserve CTE INSERT branch has no WHERE guard, so this pre-check is
//     mandatory — identical to the increment path).
//
// When denied is true, the returned specs are nil: the caller reserves nothing and
// returns the limit-exceeded decision. Limits outside their time window / custom
// period are skipped (no reservation, no denial), exactly as the increment path
// skips them.
func (s *LimitCheckerService) ResolveReservations(ctx context.Context, input *model.CheckLimitsInput) (specs []ReservationSpec, denied bool, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.reservation.resolve")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if input == nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Nil input", constant.ErrCheckLimitsNilInput)
		return nil, false, constant.ErrCheckLimitsNilInput
	}

	if err := input.Validate(); err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid input", err)
		return nil, false, err
	}

	limits, err := s.getApplicableLimits(ctx, input)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get applicable limits", err)
		return nil, false, err
	}

	if len(limits) == 0 {
		return nil, false, nil
	}

	serverNow := s.clock.Now()
	txScope := buildTransactionScope(input)
	specs = make([]ReservationSpec, 0, len(limits))

	for i := range limits {
		limit := &limits[i]

		// Skip limits outside their active window / custom period (no reservation,
		// no denial) — same precedence as the increment path.
		if _, shouldSkip := skipIfOutsideTimeWindow(limit, input, serverNow); shouldSkip {
			continue
		}

		if _, shouldSkip := skipIfOutsideCustomPeriod(limit, input, serverNow); shouldSkip {
			continue
		}

		// PER_TRANSACTION limits have no counter to reserve against: enforce the
		// cap directly. Exceeded -> immediate denial.
		if limit.LimitType == model.LimitTypePerTransaction {
			if input.Amount.GreaterThan(limit.MaxAmount) {
				return nil, true, nil
			}

			continue
		}

		periodKey, err := model.CalculatePeriodKey(limit.LimitType, serverNow)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to calculate period key", err)
			return nil, false, err
		}

		// Pre-check: amount alone > maxAmount denies (INSERT branch has no guard).
		if input.Amount.GreaterThan(limit.MaxAmount) {
			return nil, true, nil
		}

		specs = append(specs, ReservationSpec{
			LimitID:   limit.ID,
			ScopeKey:  calculateScopeKeyFromScopes(limit.Scopes, txScope),
			PeriodKey: periodKey,
			Amount:    input.Amount.IntPart(),
			MaxAmount: limit.MaxAmount.IntPart(),
		})
	}

	logger.With(
		libLog.String("operation", "service.reservation.resolve"),
		libLog.Int("applicable_limits", len(limits)),
		libLog.Int("reservation_specs", len(specs)),
	).Log(ctx, libLog.LevelInfo, "Resolved reservation specs")

	return specs, false, nil
}
