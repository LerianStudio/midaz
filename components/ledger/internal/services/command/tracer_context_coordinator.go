// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type ContextTracerConfig struct {
	Facts           tracerreservation.Config
	MaxReservations int
	// AdmissionTimeout caps the entire facts/journal/Reserve phase.
	AdmissionTimeout time.Duration
}

// ContextTracerCoordinator composes admission with its durable recovery. A
// standalone client cannot serve as the settings activation verifier.
type ContextTracerCoordinator struct {
	recovery *TracerRecoveryProcessor
	facts    TracerFactsLoader
	config   ContextTracerConfig
}

func NewContextTracerCoordinator(recovery *TracerRecoveryProcessor, facts TracerFactsLoader, cfg ContextTracerConfig) (*ContextTracerCoordinator, error) {
	if recovery == nil || facts == nil || cfg.MaxReservations <= 0 || cfg.AdmissionTimeout <= 0 {
		return nil, constant.ErrTracerContractUnavailable
	}

	if err := cfg.Facts.Validate(); err != nil {
		return nil, err
	}

	return &ContextTracerCoordinator{recovery: recovery, facts: facts, config: cfg}, nil
}

// ValidateActivation is local and safe under the settings merge lock. Deployed
// artifact compatibility is verified by rollout, with strict response checks on
// every call; this method never creates a probe reservation or dials the peer.
func (c *ContextTracerCoordinator) ValidateActivation(ctx context.Context) error { return ctx.Err() }

func (c *ContextTracerCoordinator) RunOnce(ctx context.Context) (TracerRecoverySummary, error) {
	return c.recovery.RunOnce(ctx)
}
