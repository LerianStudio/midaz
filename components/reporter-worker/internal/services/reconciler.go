// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/datasource"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/fetcher"
	extractionRepo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/extraction"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/redis"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// defaultReconcileInterval is the default polling interval for the reconciler.
	defaultReconcileInterval = 5 * time.Minute

	// defaultStaleThreshold is the default age after which a pending mapping is considered stale.
	defaultStaleThreshold = 15 * time.Minute

	// reconcilerLockKey is the Redis distributed lock key for the reconciler.
	reconcilerLockKey = "reporter:reconciler:lock"
)

// ExtractionJobStatusChecker abstracts the Fetcher client's job status polling
// so the reconciler can be tested without a real HTTP client.
//
//go:generate mockgen --destination=reconciler.mock.go --package=services --copyright_file=../../../../COPYRIGHT . ExtractionJobStatusChecker
type ExtractionJobStatusChecker interface {
	GetExtractionJobStatus(ctx context.Context, jobID string) (*fetcher.ExtractionJobResponse, error)
}

// TenantLister abstracts tenant discovery for multi-tenant reconciliation.
type TenantLister interface {
	GetActiveTenantsByService(ctx context.Context, service string) ([]*tmclient.TenantSummary, error)
}

// ReconcilerOption configures optional parameters for the Reconciler.
type ReconcilerOption func(*Reconciler)

// WithInterval sets the reconciliation polling interval.
func WithInterval(d time.Duration) ReconcilerOption {
	return func(r *Reconciler) {
		r.interval = d
	}
}

// WithStaleThreshold sets the age threshold for detecting stale pending mappings.
func WithStaleThreshold(d time.Duration) ReconcilerOption {
	return func(r *Reconciler) {
		r.staleThreshold = d
	}
}

// WithMultiTenant enables multi-tenant reconciliation: the reconciler iterates
// all active tenants from the Tenant Manager API and reconciles per-tenant.
func WithMultiTenant(lister TenantLister, mongoManager *tmmongo.Manager, serviceName string) ReconcilerOption {
	return func(r *Reconciler) {
		r.tenantLister = lister
		r.mongoManager = mongoManager
		r.multiTenantEnabled = true
		r.serviceName = serviceName
	}
}

// Reconciler periodically checks for stale ExtractionMappings stuck in "pending"
// status (e.g., when a Fetcher notification was lost) and resolves them by polling
// Fetcher for the current job status. A Redis distributed lock ensures only one
// instance reconciles at a time across multiple replicas.
type Reconciler struct {
	useCase            *UseCase
	extractionRepo     extractionRepo.Repository
	fetcherClient      ExtractionJobStatusChecker
	redisClient        redis.RedisRepository
	logger             log.Logger
	tracer             trace.Tracer
	interval           time.Duration
	staleThreshold     time.Duration
	tenantLister       TenantLister     // nil in single-tenant mode
	mongoManager       *tmmongo.Manager // resolves per-tenant MongoDB (nil in single-tenant mode)
	multiTenantEnabled bool
	serviceName        string // used for Tenant Manager API queries
}

// NewReconciler creates a new Reconciler with the given dependencies and optional
// configuration overrides.
func NewReconciler(
	uc *UseCase,
	extRepo extractionRepo.Repository,
	fetcherClient ExtractionJobStatusChecker,
	redisClient redis.RedisRepository,
	logger log.Logger,
	tracer trace.Tracer,
	opts ...ReconcilerOption,
) *Reconciler {
	r := &Reconciler{
		useCase:        uc,
		extractionRepo: extRepo,
		fetcherClient:  fetcherClient,
		redisClient:    redisClient,
		logger:         logger,
		tracer:         tracer,
		interval:       defaultReconcileInterval,
		staleThreshold: defaultStaleThreshold,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Start runs the reconciliation loop at the configured interval until the context
// is cancelled. This method blocks and should be run in a separate goroutine.
func (r *Reconciler) Start(ctx context.Context) {
	ctx, span := r.tracer.Start(ctx, "service.reconciler.start")
	defer span.End()

	r.logger.Log(ctx, log.LevelInfo, "Reconciler started",
		log.String("interval", r.interval.String()),
		log.String("stale_threshold", r.staleThreshold.String()))

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Log(ctx, log.LevelInfo, "Reconciler stopping due to context cancellation")
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

// reconcile performs a single reconciliation pass: acquires a distributed lock,
// finds stale pending mappings, and resolves each one by polling Fetcher.
// In multi-tenant mode, it iterates all active tenants and reconciles per-tenant.
func (r *Reconciler) reconcile(ctx context.Context) {
	ctx, span := r.tracer.Start(ctx, "service.reconciler.reconcile")
	defer span.End()

	// Acquire distributed lock to prevent concurrent reconciliation across replicas
	acquired, err := r.redisClient.SetNX(ctx, reconcilerLockKey, "1", 2*r.interval)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to acquire reconciler lock", err)
		r.logger.Log(ctx, log.LevelError, "Failed to acquire reconciler lock", log.Err(err))

		return
	}

	if !acquired {
		r.logger.Log(ctx, log.LevelDebug, "Reconciler lock held by another instance, skipping")
		return
	}

	defer func() {
		if delErr := r.redisClient.Del(ctx, reconcilerLockKey); delErr != nil {
			r.logger.Log(ctx, log.LevelError, "Failed to release reconciler lock", log.Err(delErr))
		}
	}()

	if r.multiTenantEnabled && r.tenantLister != nil {
		r.reconcileMultiTenant(ctx, span)
		return
	}

	r.reconcileSingleTenant(ctx, span)
}

// reconcileSingleTenant performs reconciliation for single-tenant mode (default).
func (r *Reconciler) reconcileSingleTenant(ctx context.Context, parentSpan trace.Span) {
	r.reconcileForContext(ctx, parentSpan)
}

// reconcileMultiTenant fetches active tenants and reconciles each one with
// a tenant-scoped context so that per-tenant MongoDB resolution works correctly.
func (r *Reconciler) reconcileMultiTenant(ctx context.Context, parentSpan trace.Span) {
	ctx, span := r.tracer.Start(ctx, "service.reconciler.reconcile_multi_tenant")
	defer span.End()

	tenants, err := r.tenantLister.GetActiveTenantsByService(ctx, r.serviceName)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to list active tenants for reconciliation", err)
		r.logger.Log(ctx, log.LevelError, "Failed to list active tenants for reconciliation", log.Err(err))

		return
	}

	span.SetAttributes(attribute.Int("app.reconciler.tenant_count", len(tenants)))

	r.logger.Log(ctx, log.LevelInfo, "Reconciler: iterating tenants",
		log.Int("tenant_count", len(tenants)))

	for _, tenant := range tenants {
		tenantCtx := tmcore.ContextWithTenantID(ctx, tenant.ID)

		// Resolve per-tenant MongoDB database and inject into context.
		// Without this, repository queries would fail with ErrTenantContextRequired.
		if r.mongoManager != nil {
			tenantDB, dbErr := r.mongoManager.GetDatabaseForTenant(tenantCtx, tenant.ID)
			if dbErr != nil {
				r.logger.Log(tenantCtx, log.LevelError, "Reconciler: failed to resolve tenant MongoDB, skipping",
					log.String("tenant_id", tenant.ID), log.Err(dbErr))

				continue
			}

			tenantCtx = tmcore.ContextWithMB(tenantCtx, tenantDB)
		}

		r.logger.Log(tenantCtx, log.LevelDebug, "Reconciler: reconciling tenant",
			log.String("tenant_id", tenant.ID))

		r.reconcileForContext(tenantCtx, parentSpan)
	}
}

// reconcileForContext finds and resolves stale mappings using the given context.
// The context may contain a tenant ID (multi-tenant) or be plain (single-tenant).
// Scans both "pending" (Fetcher not yet polled) and "processing" (worker crashed after claim).
func (r *Reconciler) reconcileForContext(ctx context.Context, parentSpan trace.Span) {
	// Reconcile stale pending mappings (normal path: Fetcher notification never arrived)
	stalePending, err := r.extractionRepo.FindStalePending(ctx, r.staleThreshold)
	if err != nil {
		libOtel.HandleSpanError(parentSpan, "Failed to find stale pending mappings", err)
		r.logger.Log(ctx, log.LevelError, "Failed to find stale pending mappings", log.Err(err))

		return
	}

	// Reconcile stale processing mappings (crash recovery: worker claimed but never completed)
	staleProcessing, err := r.extractionRepo.FindStaleProcessing(ctx, r.staleThreshold)
	if err != nil {
		libOtel.HandleSpanError(parentSpan, "Failed to find stale processing mappings", err)
		r.logger.Log(ctx, log.LevelError, "Failed to find stale processing mappings", log.Err(err))
		// Continue with pending even if processing query fails
		staleProcessing = nil
	}

	staleMappings := append(stalePending, staleProcessing...)

	if len(staleMappings) == 0 {
		return
	}

	parentSpan.SetAttributes(
		attribute.Int("app.reconciler.stale_pending_count", len(stalePending)),
		attribute.Int("app.reconciler.stale_processing_count", len(staleProcessing)),
		attribute.Int("app.reconciler.stale_total_count", len(staleMappings)),
	)

	r.logger.Log(ctx, log.LevelInfo, "Found stale extraction mappings",
		log.Int("pending", len(stalePending)),
		log.Int("processing", len(staleProcessing)))

	for _, mapping := range staleMappings {
		r.reconcileMapping(ctx, mapping)
	}
}

// reconcileMapping polls Fetcher for the current status of a single stale mapping
// and takes action based on the result.
func (r *Reconciler) reconcileMapping(ctx context.Context, mapping *datasource.ExtractionMapping) {
	ctx, span := r.tracer.Start(ctx, "service.reconciler.reconcile_mapping")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.job_id", mapping.JobID),
		attribute.String("app.request.report_id", mapping.ReportID),
	)

	r.logger.Log(ctx, log.LevelInfo, "Reconciling stale extraction mapping",
		log.String("job_id", mapping.JobID),
		log.String("report_id", mapping.ReportID),
		log.String("created_at", mapping.CreatedAt.Format(time.RFC3339)))

	// Poll Fetcher for current job status
	jobResp, err := r.fetcherClient.GetExtractionJobStatus(ctx, mapping.JobID)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get extraction job status from Fetcher", err)
		r.logger.Log(ctx, log.LevelError, "Failed to poll Fetcher for job status",
			log.String("job_id", mapping.JobID), log.Err(err))

		return
	}

	if jobResp == nil {
		r.logger.Log(ctx, log.LevelWarn, "Fetcher returned nil response for job status, skipping",
			log.String("job_id", mapping.JobID))

		return
	}

	span.SetAttributes(attribute.String("app.reconciler.fetcher_status", jobResp.Status))

	switch jobResp.Status {
	case constant.FetcherStatusCompleted:
		r.handleReconcileCompleted(ctx, mapping, jobResp)
	case constant.FetcherStatusFailed:
		r.handleReconcileFailed(ctx, mapping, jobResp)
	default:
		if time.Since(mapping.CreatedAt) > 60*time.Minute {
			r.logger.Log(ctx, log.LevelWarn, "Reconciler: job processing > 60min, marking as timeout",
				log.String("job_id", mapping.JobID),
				log.String("report_id", mapping.ReportID))

			now := time.Now()

			if err := r.extractionRepo.UpdateStatus(ctx, mapping.JobID, constant.ExtractionStatusFailed, &now); err != nil {
				libOtel.HandleSpanError(span, "Failed to update timed-out mapping", err)
				return
			}

			reportID, parseErr := uuid.Parse(mapping.ReportID)
			if parseErr != nil {
				libOtel.HandleSpanError(span, "Failed to parse report ID for timeout", parseErr)
				return
			}

			timeoutErr := fmt.Errorf("extraction job timed out after 60 minutes (recovered by reconciliation)")
			if errUpdate := r.useCase.updateReportWithErrors(ctx, reportID, timeoutErr); errUpdate != nil {
				libOtel.HandleSpanError(span, "Failed to update report status for timeout", errUpdate)
			}
		} else {
			r.logger.Log(ctx, log.LevelInfo, "Fetcher reports job still pending, skipping",
				log.String("job_id", mapping.JobID))
		}
	}
}
