// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ContextReservationConfig bounds the complete snapshot and expanded plan.
// None of these resource limits has an implicit production default.
type ContextReservationConfig struct {
	Facts             tracercontract.Limits
	MaxLimits         int
	MaxScopesPerLimit int
	MaxReservations   int
}

// ContextReservationSpec keeps existing counter coordinates and adds explicit
// period-based retention. Counter retention is not a reservation TTL and must
// never authorize release of a decision whose accounting outcome is unknown.
type ContextReservationSpec struct {
	ReservationSpec
	CounterExpiresAt time.Time
}

// ContextReservationPlan is a preflight, never an authorization to commit.
// Denied=false still requires atomic current+reserved capacity checks, policy
// precedence and mandatory decision/audit persistence. AccountIDs are unique
// and sorted; Reservations are sorted by limit, scope, period for lock ordering.
type ContextReservationPlan struct {
	AccountIDs       []uuid.UUID
	Reservations     []ContextReservationSpec
	Denied           bool
	ExceededLimitIDs []uuid.UUID
}

// ContextReservationResolver plans gross debits against a complete, trusted
// active snapshot. It does not access a database, acquire locks or reserve funds.
type ContextReservationResolver struct {
	clock  clock.Clock
	config ContextReservationConfig
}

func NewContextReservationResolver(clk clock.Clock, config ContextReservationConfig) (*ContextReservationResolver, error) {
	if clk == nil || config.MaxLimits <= 0 || config.MaxScopesPerLimit <= 0 || config.MaxReservations <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	if err := config.Facts.Validate(); err != nil {
		return nil, err
	}

	return &ContextReservationResolver{clock: clk, config: config}, nil
}

// Execute requires namespace from verified integration configuration and every
// candidate limit, including unresolved/unsupported candidates. The caller must
// select them on the tenant primary in the admission transaction, without
// pagination or code-only filtering. Nil/empty snapshots mean proven absence;
// this pure component cannot prove that a repository returned a complete set.
func (q *ContextReservationResolver) Execute(ctx context.Context, facts tracercontract.Context, namespace string, limits []model.ContextAccountLimit) (_ *ContextReservationPlan, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.resolve_context_reservations")
	defer span.End()
	defer func() {
		if retErr == nil {
			return
		}

		if errors.Is(retErr, constant.ErrInvalidRequestBody) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Invalid reservation context", retErr)
		} else {
			libOtel.HandleSpanError(span, "Cannot resolve account limits", retErr)
		}
	}()

	debits, err := model.AccountDebits(ctx, facts, namespace, q.config.Facts)
	if err != nil {
		return nil, err
	}

	if err := q.validateSnapshot(ctx, facts, namespace, limits); err != nil {
		return nil, err
	}

	now := q.clock.Now().UTC()
	if now.IsZero() || now.Year() < 1 || now.Year() > 9999 {
		return nil, constant.ErrInternalServer
	}

	plan := &ContextReservationPlan{
		AccountIDs:   make([]uuid.UUID, 0, len(debits)),
		Reservations: make([]ContextReservationSpec, 0), ExceededLimitIDs: make([]uuid.UUID, 0),
	}

	byAccount := make(map[uuid.UUID]model.AccountDebit, len(debits))
	for _, debit := range debits {
		plan.AccountIDs = append(plan.AccountIDs, debit.AccountID)
		byAccount[debit.AccountID] = debit
	}

	for _, limit := range limits {
		if err := q.appendLimit(ctx, plan, limit, byAccount, now); err != nil {
			return nil, err
		}
	}

	slices.SortFunc(plan.ExceededLimitIDs, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

	if plan.Denied {
		plan.Reservations = []ContextReservationSpec{}
	} else {
		slices.SortFunc(plan.Reservations, func(a, b ContextReservationSpec) int {
			if c := bytes.Compare(a.LimitID[:], b.LimitID[:]); c != 0 {
				return c
			}

			if c := cmp.Compare(a.ScopeKey, b.ScopeKey); c != 0 {
				return c
			}

			return cmp.Compare(a.PeriodKey, b.PeriodKey)
		})
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logging.WithTrace(ctx, logger).With(libLog.Int("limits.count", len(limits)), libLog.Int("reservations.count", len(plan.Reservations))).Log(ctx, libLog.LevelDebug, "Account limit plan resolved")

	return plan, nil
}

func (q *ContextReservationResolver) validateSnapshot(ctx context.Context, facts tracercontract.Context, namespace string, limits []model.ContextAccountLimit) error {
	if len(limits) > q.config.MaxLimits {
		return constant.ErrContextLimitsUnavailable
	}

	assets := make(map[tracercontract.AssetIdentity]string)
	for _, entry := range facts.Entries {
		assets[entry.Asset.Identity()] = entry.Asset.Code
	}

	accounts := make(map[uuid.UUID]tracercontract.AssetIdentity, len(facts.Accounts))
	for _, account := range facts.Accounts {
		accounts[account.ID] = account.Asset.Identity()
	}

	seen := make(map[uuid.UUID]struct{}, len(limits))
	for _, limit := range limits {
		if err := limit.Validate(ctx, namespace, q.config.Facts, q.config.MaxScopesPerLimit); err != nil {
			return err
		}

		if _, exists := seen[limit.Definition.ID]; exists {
			return constant.ErrContextLimitsUnavailable
		}

		seen[limit.Definition.ID] = struct{}{}
		if code, exists := assets[limit.Asset.Identity()]; exists && code != limit.Asset.Code {
			return constant.ErrContextLimitsUnavailable
		}

		assets[limit.Asset.Identity()] = limit.Asset.Code

		// A configured account has one official asset. An incompatible association
		// is a migration/configuration error, not a reason to silently skip a cap.
		for _, scope := range limit.Definition.Scopes {
			if err := ctx.Err(); err != nil {
				return err
			}

			if asset, known := accounts[*scope.AccountID]; known && asset != limit.Asset.Identity() {
				return constant.ErrContextLimitsUnavailable
			}
		}
	}

	return nil
}

func (q *ContextReservationResolver) appendLimit(ctx context.Context, plan *ContextReservationPlan, limit model.ContextAccountLimit, debits map[uuid.UUID]model.AccountDebit, now time.Time) error {
	d := limit.Definition
	if !d.IsWithinTimeWindow(now) || !d.IsWithinCustomPeriod(now) {
		return nil
	}

	period, err := model.CalculatePeriodKey(d.LimitType, now)
	if err != nil {
		return err
	}

	expires := calculateCounterExpiresAt(d.LimitType, model.CalculateResetAt(d.LimitType, now), d.CustomEndDate)
	exceeded := false

	for _, scope := range d.Scopes {
		if err := ctx.Err(); err != nil {
			return err
		}

		debit, exists := debits[*scope.AccountID]
		if !exists || debit.Asset.Identity() != limit.Asset.Identity() {
			continue
		}

		exceeded = exceeded || debit.Amount.GreaterThan(d.MaxAmount)
		if d.LimitType == model.LimitTypePerTransaction {
			continue
		}

		if expires == nil || !expires.After(now) || expires.Year() > 9999 {
			return constant.ErrContextLimitsUnavailable
		}

		if len(plan.Reservations) >= q.config.MaxReservations {
			return constant.ErrContextLimitsUnavailable
		}

		plan.Reservations = append(plan.Reservations, ContextReservationSpec{ReservationSpec: ReservationSpec{
			LimitID: d.ID, ScopeKey: model.CalculateScopeKey(&scope), PeriodKey: period, Amount: debit.Amount, MaxAmount: d.MaxAmount,
		}, CounterExpiresAt: *expires})
	}

	if exceeded {
		plan.Denied = true
		plan.ExceededLimitIDs = append(plan.ExceededLimitIDs, d.ID)
	}

	return nil
}
