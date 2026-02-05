// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding"
	"github.com/LerianStudio/midaz/v3/components/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// Service is the unified ledger service that composes onboarding and transaction.
type Service struct {
	OnboardingService  onboarding.OnboardingService
	TransactionService transaction.TransactionService
	UnifiedServer      *UnifiedServer
	Logger             libLog.Logger
	Telemetry          *libOpentelemetry.Telemetry
}

// Run starts the unified ledger service with all APIs on a single port.
// The UnifiedServer consolidates all HTTP routes from onboarding and transaction.
// Non-HTTP runnables (RabbitMQ, Redis consumers, workers) are started separately.
func (s *Service) Run() {
	s.Logger.Info("Running unified ledger service with single-port mode")

	// Build launcher options with unified HTTP server
	launcherOpts := []libCommons.LauncherOption{
		libCommons.WithLogger(s.Logger),
		libCommons.RunApp("Unified HTTP Server", s.UnifiedServer),
	}

	// Add non-HTTP runnables from transaction service
	// (RabbitMQ consumer, Redis consumer, Balance Sync Worker)
	transactionRunnables := s.TransactionService.GetRunnables()
	for _, r := range transactionRunnables {
		// Skip the individual Fiber server as we use the UnifiedServer instead
		if r.Name != "Transaction Fiber Server" {
			launcherOpts = append(launcherOpts, libCommons.RunApp(r.Name, r.Runnable))
		}
	}

	libCommons.NewLauncher(launcherOpts...).Run()
}

// GetRunnables returns all runnable components for the unified ledger.
// This can be used for custom launcher configuration if needed.
func (s *Service) GetRunnables() []mbootstrap.RunnableConfig {
	runnables := []mbootstrap.RunnableConfig{
		{Name: "Unified HTTP Server", Runnable: s.UnifiedServer},
	}

	// Add non-HTTP runnables from transaction
	transactionRunnables := s.TransactionService.GetRunnables()
	for _, r := range transactionRunnables {
		if r.Name != "Transaction Fiber Server" {
			runnables = append(runnables, r)
		}
	}

	return runnables
}
