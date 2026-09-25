// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type TracerRecoveryWorkerConfig struct {
	MultiTenant       bool
	Service           string
	Interval          time.Duration
	CycleTimeout      time.Duration
	TenantTimeout     time.Duration
	MaxTenants        int
	MaxCatalogTenants int
}

// TracerRecoveryWorker owns one sequential loop, independent of ledger settings
// and Redis availability. It retains no per-tenant pool across work cycles.
type TracerRecoveryWorker struct {
	processor tracerRecoveryProcessor
	catalog   tracerRecoveryCatalog
	resolver  tracerRecoveryPoolResolver
	config    TracerRecoveryWorkerConfig
	logger    libLog.Logger
	after     string
}

func NewTracerRecoveryWorker(processor tracerRecoveryProcessor, catalog tracerRecoveryCatalog, resolver tracerRecoveryPoolResolver, cfg TracerRecoveryWorkerConfig, logger libLog.Logger) (*TracerRecoveryWorker, error) {
	if processor == nil || logger == nil || cfg.Interval <= 0 || cfg.CycleTimeout <= 0 || cfg.TenantTimeout <= 0 || cfg.MaxTenants <= 0 || cfg.MaxCatalogTenants < cfg.MaxTenants {
		return nil, constant.ErrTracerContractUnavailable
	}

	if cfg.MultiTenant && (catalog == nil || resolver == nil || strings.TrimSpace(cfg.Service) == "") {
		return nil, constant.ErrTracerContractUnavailable
	}

	return &TracerRecoveryWorker{processor: processor, catalog: catalog, resolver: resolver, config: cfg, logger: logger}, nil
}

func (w *TracerRecoveryWorker) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return w.run(libObservability.ContextWithLogger(ctx, w.logger))
}

func (w *TracerRecoveryWorker) run(ctx context.Context) error {
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		_, err := w.runCycle(ctx)
		if err != nil && ctx.Err() == nil {
			// Retained durable work is retried on later cycles; warn once at
			// this worker boundary rather than logging every nested I/O error.
			w.logger.Log(ctx, libLog.LevelWarn, "Tracer recovery cycle incomplete; durable work retained", libLog.Err(err))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *TracerRecoveryWorker) runCycle(ctx context.Context) (summary command.TracerRecoverySummary, retErr error) {
	// A recovered panic must leave the cycle failed, not terminate the worker
	// or report successful delivery. Normal returns overwrite this sentinel.
	retErr = constant.ErrTracerContractUnavailable
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	// Nest discovery and per-tenant recovery under one bounded cycle span.
	ctx, span := tracer.Start(ctx, "worker.recover_tracer_reservations")
	defer span.End()
	defer func() {
		span.SetAttributes(attribute.Int("app.response.tracer.recovery.claimed", summary.Claimed), attribute.Int("app.response.tracer.recovery.delivered", summary.Delivered), attribute.Int("app.response.tracer.recovery.unresolved", summary.Unresolved), attribute.Int("app.response.tracer.recovery.failed", summary.Failed))

		if retErr != nil {
			libOtel.HandleSpanError(span, "Tracer recovery cycle incomplete", retErr)
		}
	}()
	defer libRuntime.RecoverWithPolicyAndContext(ctx, w.logger, "ledger", "tracer-recovery-cycle", libRuntime.KeepRunning)

	ctx, cancel := context.WithTimeout(ctx, w.config.CycleTimeout)
	defer cancel()

	if !w.config.MultiTenant {
		return w.processor.RunOnce(ctx)
	}

	ids, err := w.activeTenants(ctx)
	if err != nil {
		return summary, err
	}

	if len(ids) == 0 {
		return summary, nil
	}

	start := sort.SearchStrings(ids, w.after)
	if start < len(ids) && ids[start] == w.after {
		start++
	}

	var failures []error

	for offset := 0; offset < min(len(ids), w.config.MaxTenants); offset++ {
		if err := ctx.Err(); err != nil {
			return summary, errors.Join(append(failures, err)...)
		}

		tenantID := ids[(start+offset)%len(ids)]
		w.after = tenantID
		result, err := w.recoverTenant(ctx, tenantID)
		summary.Claimed += result.Claimed
		summary.Delivered += result.Delivered
		summary.Unresolved += result.Unresolved
		summary.Failed += result.Failed

		if err != nil {
			failures = append(failures, err)
		}
	}

	return summary, errors.Join(failures...)
}

func (w *TracerRecoveryWorker) activeTenants(ctx context.Context) ([]string, error) {
	entries, err := w.catalog.GetActiveTenantsByService(ctx, w.config.Service)
	if err != nil {
		return nil, fmt.Errorf("discover tracer recovery tenants: %w", err)
	}

	// The catalog API returns the full inventory. Retain only a bounded window
	// following the cursor, wrapping at the end; never drop later tenants just
	// because the inventory grew beyond the local window size.
	ids := make([]string, 0, min(len(entries), w.config.MaxCatalogTenants))
	skipped := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if entry == nil || !eligibleTracerRecoveryTenant(entry.ID, entry.Status) {
			skipped++
			continue
		}

		ids = recoveryTenantWindow(ids, entry.ID, w.after, w.config.MaxCatalogTenants)
	}

	if skipped > 0 {
		w.logger.Log(ctx, libLog.LevelWarn, "Tracer recovery skipped invalid or inactive catalog entries", libLog.Int("count", skipped))
	}

	sort.Strings(ids)

	return ids, nil
}

func eligibleTracerRecoveryTenant(id, status string) bool {
	return tmcore.IsValidTenantID(id) && strings.EqualFold(status, "active")
}

// Keep the smallest distinct identifiers in cursor order using bounded storage.
func recoveryTenantWindow(ids []string, id, after string, limit int) []string {
	index := sort.Search(len(ids), func(i int) bool {
		if (ids[i] > after) != (id > after) {
			return ids[i] <= after
		}

		return ids[i] >= id
	})
	if index == limit || (index < len(ids) && ids[index] == id) {
		return ids
	}

	if len(ids) < limit {
		ids = append(ids, "")
	}

	copy(ids[index+1:], ids[index:len(ids)-1])
	ids[index] = id

	return ids
}

func (w *TracerRecoveryWorker) recoverTenant(ctx context.Context, tenantID string) (command.TracerRecoverySummary, error) {
	ctx, cancel := context.WithTimeout(ctx, w.config.TenantTimeout)
	defer cancel()

	ctx = tmcore.ContextWithTenantID(ctx, tenantID)

	database, err := w.resolver.GetDB(ctx, tenantID)
	if err != nil {
		return command.TracerRecoverySummary{}, fmt.Errorf("resolve tracer recovery pool: %w", err)
	}

	if database == nil {
		return command.TracerRecoverySummary{}, constant.ErrTracerContractUnavailable
	}

	return w.processor.RunOnce(tmcore.ContextWithPG(ctx, database, constant.ModuleTransaction))
}
