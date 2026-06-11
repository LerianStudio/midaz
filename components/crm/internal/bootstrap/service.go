// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	tmevent "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/event"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	EventListener  *tmevent.TenantEventListener
	EncryptionMode crypto.EncryptionMode
	VaultClient    *vault.Client
	// Encryption repositories - only populated in envelope encryption mode.
	// These are nil in legacy mode (KMS_VENDOR=none or empty).
	KeysetRepo   mongoEncryption.KeysetRepository
	RegistryRepo mongoEncryption.RegistryRepository
	// AuditRepo is the read-side protection audit repository. It is only
	// populated in envelope encryption mode and is nil in legacy mode
	// (KMS_VENDOR=none or empty); the audit endpoint stays unregistered then.
	AuditRepo audit.Repository

	// Encryption services - only populated in envelope encryption mode.
	// These are nil in legacy mode (KMS_VENDOR=none or empty).
	EncryptionService       encryption.EncryptionService
	ProvisioningService     encryption.ProvisioningService
	ProtectionStateResolver *encryption.ProtectionStateResolver
	KeysetManager           *encryption.KeysetManager

	libLog.Logger
}

// Run starts the application.
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	launcherOpts := []libCommons.LauncherOption{
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("HTTP Service", app.Server),
	}

	if app.EventListener != nil {
		launcherOpts = append(launcherOpts, libCommons.RunApp("Tenant Event Listener",
			&eventListenerRunnable{listener: app.EventListener}))
	}

	libCommons.NewLauncher(launcherOpts...).Run()
}

// eventListenerRunnable adapts a TenantEventListener to the libCommons.App interface.
// It starts the Redis Pub/Sub listener and blocks until SIGINT/SIGTERM is received,
// matching the shutdown pattern of other runnables in this package.
type eventListenerRunnable struct {
	listener *tmevent.TenantEventListener
}

// Run starts the event listener and blocks until SIGINT/SIGTERM.
func (r *eventListenerRunnable) Run(_ *libCommons.Launcher) error {
	if r.listener == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := r.listener.Start(ctx); err != nil {
		stop()

		return err
	}

	<-ctx.Done()
	stop()

	return r.listener.Stop()
}
