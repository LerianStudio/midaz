package bootstrap

import (
	"context"
	"database/sql"
	"errors"

	"go.mongodb.org/mongo-driver/mongo"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Service holds all components for the reconciliation worker
type Service struct {
	Worker     *ReconciliationWorker
	HTTPServer *HTTPServer
	Logger     libLog.Logger
	Config     *Config

	// Database connections for cleanup
	onboardingDB  *sql.DB
	transactionDB *sql.DB
	mongoClient   *mongo.Client
}

// Run starts all workers using the launcher pattern
func (s *Service) Run() {
	opts := []libCommons.LauncherOption{
		libCommons.WithLogger(s.Logger),
		libCommons.RunApp("Reconciliation Worker", s.Worker),
		libCommons.RunApp("HTTP Status Server", s.HTTPServer),
	}
	libCommons.NewLauncher(opts...).Run()
}

// Shutdown gracefully closes all database connections
func (s *Service) Shutdown(ctx context.Context) error {
	var errs []error

	s.Logger.Info("Shutting down reconciliation service...")

	if s.onboardingDB != nil {
		if err := s.onboardingDB.Close(); err != nil {
			s.Logger.Errorf("Failed to close onboarding DB: %v", err)
			errs = append(errs, err)
		}
	}

	if s.transactionDB != nil {
		if err := s.transactionDB.Close(); err != nil {
			s.Logger.Errorf("Failed to close transaction DB: %v", err)
			errs = append(errs, err)
		}
	}

	if s.mongoClient != nil {
		if err := s.mongoClient.Disconnect(ctx); err != nil {
			s.Logger.Errorf("Failed to disconnect MongoDB: %v", err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	s.Logger.Info("Reconciliation service shutdown complete")

	return nil
}
