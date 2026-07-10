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
	tmevent "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/event"
	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	pkgsd "github.com/LerianStudio/midaz/v3/pkg/servicediscovery"
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

	// StreamingEnabled mirrors Config.StreamingEnabled so Run() can decide
	// whether to register the streaming drain runnable. StreamingClose is the
	// emitter close hook (always non-nil; a no-op when streaming is disabled).
	StreamingEnabled bool
	StreamingClose   func() error

	// ServiceDiscovery is the SD Manager (always non-nil; a working no-op when
	// discovery is disabled). ServiceDiscoveryEnabled mirrors SD_ENABLED so Run()
	// can decide whether to wire a register/deregister runnable. ServiceDescriptor
	// is the descriptor this service advertises; it is built at wiring time only
	// when discovery is enabled and left zero-value otherwise.
	ServiceDiscovery        *libsd.Manager
	ServiceDiscoveryEnabled bool
	ServiceDescriptor       libsd.Service
	// ServiceDiscoveryMetrics records SD register/deregister metrics through the
	// discovery runnable. It is a NopMetricsRecorder when discovery is disabled,
	// so no SD metrics are emitted with SD off.
	ServiceDiscoveryMetrics pkgsd.MetricsRecorder

	libLog.Logger
}

// Run starts the application.
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	launcherOpts := []libCommons.LauncherOption{
		libCommons.WithLogger(app.Logger),
	}

	for _, a := range app.launcherApps() {
		launcherOpts = append(launcherOpts, libCommons.RunApp(a.name, a.app))
	}

	libCommons.NewLauncher(launcherOpts...).Run()
}

// launcherApp pairs a Launcher app with its display name so the assembly order
// and the enable/disable guards are inspectable by tests without starting the
// blocking Launcher.
type launcherApp struct {
	name string
	app  libCommons.App
}

// launcherApps assembles the ordered Launcher apps for this service, applying the
// same enable/disable guards Run() relies on. Apps run CONCURRENTLY under the
// Launcher, so this order does not sequence execution — it only fixes assembly.
func (app *Service) launcherApps() []launcherApp {
	apps := make([]launcherApp, 0)

	// Service discovery: registered only when discovery is enabled to preserve
	// boot parity — no extra Launcher entry / goroutine otherwise. Ordering is
	// best-effort only; the 30s TTL on the descriptor is the real guarantee that
	// a stale instance drops out of the registry if deregister is missed.
	if app.ServiceDiscoveryEnabled {
		apps = append(apps, launcherApp{
			"Service Discovery",
			pkgsd.NewRunnable(app.ServiceDiscovery, app.ServiceDescriptor, app.Logger, app.ServiceDiscoveryMetrics),
		})
	}

	apps = append(apps, launcherApp{"HTTP Service", app.Server})

	if app.EventListener != nil {
		apps = append(apps, launcherApp{
			"Tenant Event Listener",
			&eventListenerRunnable{listener: app.EventListener},
		})
	}

	// Streaming producer: register only when streaming is actually enabled AND
	// we have a non-nil close hook. The NoopEmitter path is skipped to keep the
	// Launcher app list lean.
	if app.StreamingEnabled && app.StreamingClose != nil {
		apps = append(apps, launcherApp{
			"Streaming Producer",
			&streamingProducerRunnable{close: app.StreamingClose, logger: app.Logger},
		})
	}

	return apps
}

// streamingProducerRunnable adapts the lib-streaming Producer's Close hook to
// the libCommons.App interface. It blocks until SIGINT/SIGTERM and then runs
// the producer's drain/flush close path so buffered records reach the broker
// before the process exits.
type streamingProducerRunnable struct {
	close  func() error
	logger libLog.Logger
}

// Run blocks until SIGINT/SIGTERM and then invokes the producer close hook. The
// close hook is responsible for draining records under its configured close
// timeout; a non-nil return is logged but not propagated because at shutdown
// the Launcher cannot meaningfully react.
func (r *streamingProducerRunnable) Run(_ *libCommons.Launcher) error {
	if r == nil || r.close == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	if err := r.close(); err != nil && r.logger != nil {
		r.logger.Log(
			context.Background(), libLog.LevelWarn,
			"streaming producer Close returned error",
			libLog.Err(err),
		)
	}

	return nil
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
