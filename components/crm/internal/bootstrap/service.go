// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	tmevent "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/event"
	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/audit"
	mongoEncryption "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/encryption"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
	"github.com/LerianStudio/midaz/v3/pkg/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
)

// serviceDiscoveryDeregisterTimeout bounds the shutdown deregister call so a
// slow or unreachable registry cannot hold the process open at exit. TTL expiry
// is the backstop when deregister does not complete in time.
const serviceDiscoveryDeregisterTimeout = 5 * time.Second

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
	// is the descriptor this service advertises; it is populated in a later task
	// and left zero-value here.
	ServiceDiscovery        *libsd.Manager
	ServiceDiscoveryEnabled bool
	ServiceDescriptor       libsd.Service

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
		apps = append(apps, launcherApp{"Service Discovery", &serviceDiscoveryRunnable{
			manager: app.ServiceDiscovery,
			svc:     app.ServiceDescriptor,
			logger:  app.Logger,
		}})
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

// parseServerPort extracts the numeric port from a listen address. It accepts
// both the leading-colon form (":4003") and the host:port form
// ("0.0.0.0:8080"); net.SplitHostPort handles both. A malformed address is a
// config bug and surfaces as an error for fail-fast handling at wiring time.
func parseServerPort(serverAddress string) (int, error) {
	_, portStr, err := net.SplitHostPort(serverAddress)
	if err != nil {
		return 0, fmt.Errorf("parsing server address %q: %w", serverAddress, err)
	}

	return strconv.Atoi(portStr)
}

// buildCRMServiceDescriptor builds the registry descriptor for this CRM
// instance. Address and Scheme are intentionally left unset: Manager.Register
// fills them from SD_ADVERTISE_ADDRESS. The TTL health check needs no reachable
// HTTP endpoint — the registry heartbeats from inside the process.
func buildCRMServiceDescriptor(port int) libsd.Service {
	return libsd.Service{
		ID:          "midaz-crm-" + strconv.Itoa(port),
		Name:        "midaz-crm",
		Port:        port,
		HealthCheck: &libsd.HealthCheck{TTL: "30s"},
	}
}

// serviceDiscoveryRunnable adapts service-discovery register/deregister to the
// libCommons.App interface. It registers asynchronously at start, blocks until
// SIGINT/SIGTERM, then deregisters on shutdown. A deregister failure is logged
// at Warn but not propagated: TTL expiry is the backstop and the Launcher cannot
// meaningfully react at shutdown.
type serviceDiscoveryRunnable struct {
	manager *libsd.Manager
	svc     libsd.Service
	logger  libLog.Logger

	// notifyContext builds the signal-scoped context that gates the runnable's
	// lifetime. It defaults to signal.NotifyContext in production; tests inject a
	// factory returning a context they cancel to simulate SIGTERM deterministically.
	notifyContext func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
}

// Run registers the service asynchronously against the signal-scoped context,
// blocks until SIGINT/SIGTERM, then deregisters under a fresh short-lived
// context (the signal context is already cancelled by then).
func (r *serviceDiscoveryRunnable) Run(_ *libCommons.Launcher) error {
	if r == nil || r.manager == nil {
		return nil
	}

	notify := r.notifyContext
	if notify == nil {
		notify = signal.NotifyContext
	}

	sigCtx, stop := notify(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// RegisterAsync is non-blocking and retries in the background until sigCtx
	// is cancelled. It needs the app-lifetime (signal) context, not a
	// request-scoped one.
	r.manager.RegisterAsync(sigCtx, r.svc)

	<-sigCtx.Done()

	ctx, cancel := context.WithTimeout(context.Background(), serviceDiscoveryDeregisterTimeout)
	defer cancel()

	if err := r.manager.Deregister(ctx, r.svc.ID); err != nil && r.logger != nil {
		r.logger.Log(
			context.Background(), libLog.LevelWarn,
			"service discovery deregister returned error",
			libLog.String("service_id", r.svc.ID),
			libLog.Err(err),
		)
	}

	return nil
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
