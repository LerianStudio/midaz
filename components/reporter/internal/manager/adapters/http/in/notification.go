// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"

	"github.com/LerianStudio/midaz/v4/components/reporter/internal/manager/services"
	netHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	_ "github.com/LerianStudio/midaz/v4/pkg/reporter" // swag: resolves pkg.HTTPError in annotations
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/deadline"
	http "github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"
	"github.com/gofiber/fiber/v2"
)

const (
	defaultNotificationLimit = 10
	minNotificationLimit     = 1
	maxNotificationLimit     = 100
	overdueSeverity          = "overdue"
	warningSeverity          = "warning"
	infoSeverity             = "info"
	warningThresholdDays     = 7
)

// NotificationHandler handles HTTP requests for deadline notification operations.
type NotificationHandler struct {
	service *services.UseCase
}

// NewNotificationHandler creates a new NotificationHandler with the given service dependency.
func NewNotificationHandler(service *services.UseCase) (*NotificationHandler, error) {
	if service == nil {
		return nil, errors.New("service must not be nil for NotificationHandler")
	}

	if service.DeadlineRepo == nil {
		return nil, errors.New("service.DeadlineRepo must not be nil for NotificationHandler")
	}

	return &NotificationHandler{service: service}, nil
}

// NotificationItem represents a single notification in the response.
//
//	@Description	NotificationItem describes a deadline that has entered its notification window.
type NotificationItem struct {
	// ID is the unique identifier of the deadline.
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000001"`
	// Name is the human-readable label of the deadline.
	Name string `json:"name" example:"CADOC 4010"`
	// Description is an optional longer description of the deadline.
	Description string `json:"description,omitempty" example:"Monthly regulatory report"`
	// Type categorises the deadline (e.g. "regulatory", "custom").
	Type string `json:"type" example:"regulatory"`
	// DueDate is the ISO-8601 timestamp when the deadline falls due.
	DueDate string `json:"dueDate" example:"2026-07-31T00:00:00Z"`
	// Frequency describes how often the deadline recurs (e.g. "monthly", "annual").
	Frequency string `json:"frequency" example:"monthly"`
	// MonthsOfYear lists the months (1-12) the deadline applies to, when frequency is not every month.
	MonthsOfYear []int `json:"monthsOfYear,omitempty" example:"1,6,12"`
	// Color is the hex colour code used in the UI for this deadline.
	Color string `json:"color" example:"#ef4444"`
	// Severity is the urgency level: "overdue", "warning", or "info".
	Severity string `json:"severity" example:"warning"`
	// DaysUntilDue is the number of days until (positive) or past (negative) the due date.
	DaysUntilDue int `json:"daysUntilDue" example:"3"`
	// NotifyDaysBefore is how many days ahead of the due date this notification window opens.
	NotifyDaysBefore int `json:"notifyDaysBefore" example:"5"`
} //	@name	NotificationItem

// NotificationResponse represents the JSON response from GET /v1/deadlines/notifications.
//
//	@Description	NotificationResponse wraps the list of active deadline notifications with a total count.
type NotificationResponse struct {
	// Items is the list of deadline notifications within their alert window, sorted by urgency.
	Items []NotificationItem `json:"items"`
	// Total is the number of notifications returned.
	Total int `json:"total" example:"3"`
} //	@name	NotificationResponse

// GetNotifications returns active deadlines that are within their notification window,
// sorted by urgency (overdue first, then warning, then info).
//
//	@Summary		Get deadline notifications
//	@Description	Returns active deadlines within their notification window, sorted by urgency
//	@Tags			Deadlines
//	@Produce		json
//	@Security		BearerAuth
//	@Param			limit	query		int	false	"Maximum number of notifications (1-100)"	default(10)
//	@Success		200		{object}	NotificationResponse
//	@Failure		400		{object}	pkg.HTTPError
//	@Failure		401		{object}	pkg.HTTPError
//	@Failure		403		{object}	pkg.HTTPError
//	@Failure		500		{object}	pkg.HTTPError
//	@Router			/v1/deadlines/notifications [get]
func (nh *NotificationHandler) GetNotifications(c *fiber.Ctx) error {
	ctx := c.UserContext()

	ctx, span := nh.service.Tracer.Start(ctx, "handler.notification.get")
	defer span.End()

	nh.service.Logger.Log(ctx, log.LevelDebug, "Request to get deadline notifications")

	limit, err := parseNotificationLimit(c)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid limit parameter", err)

		return http.BadRequest(c, err.Error())
	}

	deadlines, err := nh.service.DeadlineRepo.FindActiveNotifiable(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to fetch notifications", err)
		nh.service.Logger.Log(ctx, log.LevelError, "Failed to fetch notifications", log.Err(err))

		return netHTTP.WithError(c, err)
	}

	now := time.Now().UTC()
	items := filterAndBuildNotifications(deadlines, now)

	sortNotificationsByUrgency(items)

	if len(items) > limit {
		items = items[:limit]
	}

	reqID := ctxutil.HeaderIDFromContext(ctx)
	nh.service.Logger.Log(ctx, log.LevelDebug, "Successfully retrieved notifications",
		log.String("request_id", reqID),
		log.Any("total", len(items)),
	)

	return c.Status(fiber.StatusOK).JSON(NotificationResponse{
		Items: items,
		Total: len(items),
	})
}

// filterAndBuildNotifications applies the notification window filter and builds response items.
func filterAndBuildNotifications(deadlines []*deadline.Deadline, now time.Time) []NotificationItem {
	items := make([]NotificationItem, 0, len(deadlines))

	for _, d := range deadlines {
		daysUntilDue := ComputeDaysUntilDue(d.DueDate, now)

		if daysUntilDue >= 0 && daysUntilDue > d.NotifyDaysBefore {
			continue
		}

		items = append(items, NotificationItem{
			ID:               d.ID.String(),
			Name:             d.Name,
			Description:      d.Description,
			Type:             d.Type,
			DueDate:          d.DueDate.Format(time.RFC3339),
			Frequency:        d.Frequency,
			MonthsOfYear:     d.MonthsOfYear,
			Color:            d.Color,
			Severity:         ComputeNotificationSeverity(daysUntilDue),
			DaysUntilDue:     daysUntilDue,
			NotifyDaysBefore: d.NotifyDaysBefore,
		})
	}

	return items
}

// sortNotificationsByUrgency sorts notifications: overdue first (most negative), then warning, then info.
func sortNotificationsByUrgency(items []NotificationItem) {
	sort.SliceStable(items, func(i, j int) bool {
		si := severityOrder(items[i].Severity)
		sj := severityOrder(items[j].Severity)

		if si != sj {
			return si < sj
		}

		return items[i].DaysUntilDue < items[j].DaysUntilDue
	})
}

// severityOrder returns a numeric order for sorting: overdue=0, warning=1, info=2.
func severityOrder(severity string) int {
	switch severity {
	case overdueSeverity:
		return 0
	case warningSeverity:
		return 1
	default:
		return 2
	}
}

// ComputeNotificationSeverity determines the severity level based on days until due.
func ComputeNotificationSeverity(daysUntilDue int) string {
	if daysUntilDue < 0 {
		return overdueSeverity
	}

	if daysUntilDue <= warningThresholdDays {
		return warningSeverity
	}

	return infoSeverity
}

// ComputeDaysUntilDue calculates the number of days between today and the due date.
// Negative values indicate the deadline is overdue.
func ComputeDaysUntilDue(dueDate, now time.Time) int {
	due := dueDate.Truncate(hoursPerDay * time.Hour)
	today := now.Truncate(hoursPerDay * time.Hour)

	return int(due.Sub(today).Hours() / hoursPerDay)
}

// parseNotificationLimit extracts and validates the limit query parameter.
func parseNotificationLimit(c *fiber.Ctx) (int, error) {
	raw := c.Query("limit")
	if raw == "" {
		return defaultNotificationLimit, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer, got: %s", raw)
	}

	if limit < minNotificationLimit || limit > maxNotificationLimit {
		return 0, fmt.Errorf("limit must be between %d and %d, got: %d",
			minNotificationLimit, maxNotificationLimit, limit)
	}

	return limit, nil
}
