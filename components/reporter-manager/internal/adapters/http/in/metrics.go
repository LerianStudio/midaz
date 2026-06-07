// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"

	"github.com/LerianStudio/midaz/v4/components/reporter-manager/internal/services"
	netHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	_ "github.com/LerianStudio/midaz/v4/pkg/reporter" // swag: resolves pkg.HTTPError in annotations
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	http "github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"
	"github.com/gofiber/fiber/v2"
)

const (
	defaultErrorPeriodDays = 7
	minErrorPeriodDays     = 1
	maxErrorPeriodDays     = 365
	hoursPerDay            = 24
	errorStatus            = "Error"
	// metricsParallelGroups is the EXACT count of goroutines launched in GetMetrics.
	// MUST equal:
	//   1. The number of go func() blocks in GetMetrics
	//   2. The size of the errs[] slice: make([]error, metricsParallelGroups)
	//   3. The wg.Add argument: wg.Add(metricsParallelGroups)
	// Changing this without updating all three sites causes a runtime panic (index out of range or deadlock).
	metricsParallelGroups = 3
)

// MetricsHandler handles HTTP requests for metrics operations.
type MetricsHandler struct {
	service *services.UseCase
}

// NewMetricsHandler creates a new MetricsHandler with the given service dependency.
func NewMetricsHandler(service *services.UseCase) (*MetricsHandler, error) {
	if service == nil {
		return nil, errors.New("service must not be nil for MetricsHandler")
	}

	if service.TemplateRepo == nil {
		return nil, errors.New("service.TemplateRepo must not be nil for MetricsHandler")
	}

	if service.ReportRepo == nil {
		return nil, errors.New("service.ReportRepo must not be nil for MetricsHandler")
	}

	if service.ExternalDataSources == nil {
		return nil, errors.New("service.ExternalDataSources must not be nil for MetricsHandler")
	}

	return &MetricsHandler{service: service}, nil
}

// metricsResponse represents the JSON response from GET /v1/metrics.
type metricsResponse struct {
	Templates   int64        `json:"templates"`
	Reports     int64        `json:"reports"`
	DataSources int64        `json:"dataSources"`
	Errors      errorMetrics `json:"errors"`
}

// errorMetrics represents the error counters in the metrics response.
type errorMetrics struct {
	Total               int64 `json:"total"`
	PreviousPeriodTotal int64 `json:"previousPeriodTotal"`
}

// GetMetrics returns aggregated metrics including template count, report count,
// data source count, and error counts for the current and previous periods.
//
//	@Summary		Get system metrics
//	@Description	Returns aggregated counts of templates, reports, data sources, and errors
//	@Tags			Metrics
//	@Produce		json
//	@Security		BearerAuth
//	@Param			errorPeriodDays	query		int	false	"Number of days for error period"	default(7)	minimum(1)	maximum(365)
//	@Success		200				{object}	metricsResponse
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		401				{object}	pkg.HTTPError
//	@Failure		403				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/metrics [get]
func (mh *MetricsHandler) GetMetrics(c *fiber.Ctx) error {
	ctx := c.UserContext()

	ctx, span := mh.service.Tracer.Start(ctx, "handler.metrics.get")
	defer span.End()

	mh.service.Logger.Log(ctx, log.LevelInfo, "Request to get metrics")

	periodDays, err := parseErrorPeriodDays(c)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid errorPeriodDays parameter", err)

		return http.BadRequest(c, err.Error())
	}

	now := time.Now().UTC()
	currentPeriodStart := now.Add(-time.Duration(periodDays) * hoursPerDay * time.Hour)
	previousPeriodStart := currentPeriodStart.Add(-time.Duration(periodDays) * hoursPerDay * time.Hour)

	var (
		templateCount        int64
		reportCount          int64
		currentPeriodErrors  int64
		previousPeriodErrors int64
		errs                 = make([]error, metricsParallelGroups)
		wg                   sync.WaitGroup
	)

	wg.Add(metricsParallelGroups)

	go func() {
		defer wg.Done()

		templateCount, errs[0] = mh.service.TemplateRepo.CountAll(ctx)
	}()

	go func() {
		defer wg.Done()

		reportCount, errs[1] = mh.service.ReportRepo.CountAll(ctx)
	}()

	go func() {
		defer wg.Done()

		currentPeriodErrors, errs[2] = mh.service.ReportRepo.CountByStatus(ctx, errorStatus, currentPeriodStart, now)
		if errs[2] != nil {
			return
		}

		previousPeriodErrors, errs[2] = mh.service.ReportRepo.CountByStatus(ctx, errorStatus, previousPeriodStart, currentPeriodStart)
	}()

	wg.Wait()

	for _, e := range errs {
		if e != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to fetch metrics", e)
			mh.service.Logger.Log(ctx, log.LevelError, "Failed to fetch metrics", log.Err(e))

			return netHTTP.WithError(c, e)
		}
	}

	// In fetcher mode the DataSourceProvider delegates to the Fetcher service;
	// in direct mode fall back to the local SafeDataSources map count.
	var dataSourceCount int64
	if mh.service.DataSourceProvider != nil {
		dataSourceCount = int64(len(mh.service.GetDataSourceInformation(ctx)))
	} else {
		dataSourceCount = int64(mh.service.ExternalDataSources.Len())
	}

	response := metricsResponse{
		Templates:   templateCount,
		Reports:     reportCount,
		DataSources: dataSourceCount,
		Errors: errorMetrics{
			Total:               currentPeriodErrors,
			PreviousPeriodTotal: previousPeriodErrors,
		},
	}

	reqID := ctxutil.HeaderIDFromContext(ctx)
	mh.service.Logger.Log(ctx, log.LevelInfo, "Successfully retrieved metrics",
		log.String("request_id", reqID),
		log.Any("templates", templateCount),
		log.Any("reports", reportCount),
		log.Any("data_sources", dataSourceCount),
	)

	return c.Status(fiber.StatusOK).JSON(response)
}

// parseErrorPeriodDays extracts and validates the errorPeriodDays query parameter.
func parseErrorPeriodDays(c *fiber.Ctx) (int, error) {
	raw := c.Query("errorPeriodDays")
	if raw == "" {
		return defaultErrorPeriodDays, nil
	}

	days, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("errorPeriodDays must be an integer, got: %s", raw)
	}

	if days < minErrorPeriodDays || days > maxErrorPeriodDays {
		return 0, fmt.Errorf("errorPeriodDays must be between %d and %d, got: %d",
			minErrorPeriodDays, maxErrorPeriodDays, days)
	}

	return days, nil
}
