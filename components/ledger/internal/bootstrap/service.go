package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// Service is the unified ledger service that composes onboarding and transaction.
type Service struct {
	OnboardingService  mbootstrap.Service
	TransactionService transaction.TransactionService
	Logger             libLog.Logger
	Telemetry          *libOpentelemetry.Telemetry
}

// Run starts both onboarding and transaction services in the same process.
// It collects all runnables from both services and launches them with a unified launcher.
func (s *Service) Run() {
	s.Logger.Info("Running unified ledger service")

	// Collect runnables from onboarding service
	onboardingRunnables := s.OnboardingService.GetRunnables()

	// Collect runnables from transaction service
	transactionRunnables := s.TransactionService.GetRunnables()

	// Build launcher options
	launcherOpts := []libCommons.LauncherOption{
		libCommons.WithLogger(s.Logger),
	}

	// Add onboarding runnables
	for _, r := range onboardingRunnables {
		launcherOpts = append(launcherOpts, libCommons.RunApp(r.Name, r.Runnable))
	}

	// Add transaction runnables
	for _, r := range transactionRunnables {
		launcherOpts = append(launcherOpts, libCommons.RunApp(r.Name, r.Runnable))
	}

	libCommons.NewLauncher(launcherOpts...).Run()
}
