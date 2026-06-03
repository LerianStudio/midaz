// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package deadline

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/net/http"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ValidColorRegex matches valid hex color codes (e.g., #FF5733).
var ValidColorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// Repository provides an interface for operations related to deadline entities in MongoDB.
//
//go:generate mockgen --destination=deadline.mock.go --package=deadline --copyright_file=../../../COPYRIGHT . Repository
type Repository interface {
	FindByID(ctx context.Context, id uuid.UUID) (*Deadline, error)
	FindList(ctx context.Context, filters http.QueryHeader) ([]*Deadline, error)
	Count(ctx context.Context, filters http.QueryHeader) (int64, error)
	Create(ctx context.Context, record *DeadlineMongoDBModel) (*Deadline, error)
	Update(ctx context.Context, id uuid.UUID, updateFields *bson.M) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByTemplateID(ctx context.Context, templateID uuid.UUID) (int64, error)
	FindActiveNotifiable(ctx context.Context) ([]*Deadline, error)
}

// Deadline represents the entity model for a deadline.
// Public fields are required for JSON serialization (json tags) and Swagger documentation.
// This is a documented deviation from Ring's private-field pattern; use NewDeadline() for programmatic creation.
type Deadline struct {
	ID               uuid.UUID  `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name             string     `json:"name" example:"Monthly Regulatory Report"`
	Description      string     `json:"description,omitempty" example:"Monthly regulatory compliance report"`
	Type             string     `json:"type" example:"regulatory"`
	TemplateID       *uuid.UUID `json:"templateId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	TemplateName     string     `json:"templateName,omitempty" example:"Template Financeiro"`
	DueDate          time.Time  `json:"dueDate" example:"2026-03-31T23:59:59Z"`
	Frequency        string     `json:"frequency" example:"monthly"`
	MonthsOfYear     []int      `json:"monthsOfYear,omitempty" example:"1,6"`
	Active           bool       `json:"active" example:"true"`
	NotifyDaysBefore int        `json:"notifyDaysBefore,omitempty" example:"5"`
	Color            string     `json:"color" example:"#FF5733"`
	Status           string     `json:"status" example:"pending"`
	DeliveredAt      *time.Time `json:"deliveredAt,omitempty" example:"2026-03-15T10:00:00Z"`
	CreatedAt        time.Time  `json:"createdAt" example:"2026-01-01T00:00:00Z"`
	UpdatedAt        time.Time  `json:"updatedAt" example:"2026-01-01T00:00:00Z"`
}

// ValidFrequencies defines the set of allowed frequency values for a deadline.
var ValidFrequencies = map[string]bool{
	"once":       true,
	"daily":      true,
	"weekly":     true,
	"monthly":    true,
	"semiannual": true,
	"annual":     true,
}

// FrequenciesWithMonthsOfYear defines frequencies where monthsOfYear is meaningful.
var FrequenciesWithMonthsOfYear = map[string]bool{
	"semiannual": true,
	"annual":     true,
}

// expectedMonthCount maps each frequency to the required number of distinct months in monthsOfYear.
var expectedMonthCount = map[string]int{
	"semiannual": 2,
	"annual":     1,
}

// ValidateScheduleFields checks that monthsOfYear is compatible with the given frequency.
// When requireMissing is true, it also enforces that required fields are present (used on create).
func ValidateScheduleFields(frequency string, monthsOfYear []int, requireMissing bool) error {
	if len(monthsOfYear) > 0 {
		if !FrequenciesWithMonthsOfYear[frequency] {
			return constant.ErrMonthsOfYearNotApplicable
		}

		for _, m := range monthsOfYear {
			if m < 1 || m > 12 {
				return constant.ErrMonthsOfYearOutOfRange
			}
		}

		// Enforce month count matches the declared frequency
		expected := expectedMonthCount[frequency]
		if expected > 0 && len(monthsOfYear) != expected {
			return constant.ErrMonthsOfYearCountMismatch
		}
	}

	if requireMissing {
		if FrequenciesWithMonthsOfYear[frequency] && len(monthsOfYear) == 0 {
			return constant.ErrMonthsOfYearRequired
		}
	}

	return nil
}

// ValidTypes defines the set of allowed type values for a deadline.
var ValidTypes = map[string]bool{
	"regulatory": true,
	"custom":     true,
}

// StatusPending indicates the deadline has not yet been delivered and is not overdue.
const StatusPending = "pending"

// StatusOverdue indicates the deadline has passed its due date without delivery.
const StatusOverdue = "overdue"

// StatusDelivered indicates the deadline has been delivered.
const StatusDelivered = "delivered"

// NewDeadline creates a new Deadline entity with invariant validation.
// This constructor ensures the Deadline can never exist in an invalid state.
//
// Parameters:
//   - id: The deadline UUID (must not be uuid.Nil)
//   - name: The deadline name (must not be empty)
//   - deadlineType: The deadline type (must be "regulatory" or "custom")
//   - dueDate: The due date (must not be zero)
//   - frequency: The frequency (must be a valid frequency value)
//   - color: The color code (must not be empty)
//
// Returns:
//   - *Deadline: A validated Deadline entity
//   - error: Wrapped ErrMissingRequiredFields if any invariant is violated
func NewDeadline(id uuid.UUID, name, deadlineType string, dueDate time.Time, frequency, color string) (*Deadline, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("deadline id must not be nil: %w", constant.ErrMissingRequiredFields)
	}

	if name == "" {
		return nil, fmt.Errorf("deadline name must not be empty: %w", constant.ErrMissingRequiredFields)
	}

	if !ValidTypes[deadlineType] {
		return nil, constant.ErrInvalidDeadlineType
	}

	if dueDate.IsZero() {
		return nil, fmt.Errorf("deadline dueDate must not be zero: %w", constant.ErrMissingRequiredFields)
	}

	today := time.Now().Truncate(24 * time.Hour)
	if dueDate.Truncate(24 * time.Hour).Before(today) {
		return nil, constant.ErrDueDateInPast
	}

	if !ValidFrequencies[frequency] {
		return nil, constant.ErrInvalidDeadlineFrequency
	}

	if !ValidColorRegex.MatchString(color) {
		return nil, constant.ErrInvalidDeadlineColor
	}

	now := time.Now()

	return &Deadline{
		ID:        id,
		Name:      name,
		Type:      deadlineType,
		DueDate:   dueDate,
		Frequency: frequency,
		Color:     color,
		Active:    true,
		Status:    ComputeStatus(dueDate, nil),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ReconstructDeadline creates a Deadline from persisted data without validation.
// Used only for database hydration where data integrity is already ensured.
func ReconstructDeadline(
	id uuid.UUID,
	name, description, deadlineType string,
	templateID *uuid.UUID,
	templateName string,
	dueDate time.Time,
	frequency string,
	monthsOfYear []int,
	active bool,
	notifyDaysBefore int,
	color string,
	deliveredAt *time.Time,
	createdAt, updatedAt time.Time,
) *Deadline {
	return &Deadline{
		ID:               id,
		Name:             name,
		Description:      description,
		Type:             deadlineType,
		TemplateID:       templateID,
		TemplateName:     templateName,
		DueDate:          dueDate,
		Frequency:        frequency,
		MonthsOfYear:     monthsOfYear,
		Active:           active,
		NotifyDaysBefore: notifyDaysBefore,
		Color:            color,
		Status:           ComputeStatus(dueDate, deliveredAt),
		DeliveredAt:      deliveredAt,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}
}

// NextDueDate computes the next occurrence date after the given dueDate based on recurrence fields.
// Returns the same dueDate for "once" frequency (no recurrence).
// The day is always derived from the dueDate itself.
// For monthly/semiannual/annual, clamps the day to the last day of the target month (e.g., day 31 in Feb → 28/29).
func NextDueDate(dueDate time.Time, frequency string, monthsOfYear []int) time.Time {
	switch frequency {
	case "daily":
		return dueDate.AddDate(0, 0, 1)
	case "weekly":
		return dueDate.AddDate(0, 0, 7)
	case "monthly":
		day := dueDate.Day()

		// Advance by incrementing month manually to avoid Go's AddDate overflow behavior
		// (e.g., Jan 31 + 1 month = Mar 3 in Go, but we want Feb 28)
		nextYear := dueDate.Year()
		nextMonth := dueDate.Month() + 1

		if nextMonth > 12 {
			nextMonth = 1
			nextYear++
		}

		return clampDayToMonth(nextYear, nextMonth, day, dueDate)
	case "semiannual", "annual":
		day := dueDate.Day()

		if len(monthsOfYear) == 0 {
			if frequency == "semiannual" {
				return dueDate.AddDate(0, 6, 0)
			}

			return dueDate.AddDate(1, 0, 0)
		}

		sorted := make([]int, len(monthsOfYear))
		copy(sorted, monthsOfYear)
		sort.Ints(sorted)

		currentMonth := int(dueDate.Month())
		currentYear := dueDate.Year()

		// Find the next month in the list after the current dueDate month
		for _, m := range sorted {
			if m > currentMonth {
				return clampDayToMonth(currentYear, time.Month(m), day, dueDate)
			}
		}

		// Wrap to first configured month of the next year
		return clampDayToMonth(currentYear+1, time.Month(sorted[0]), day, dueDate)
	}

	// "once" or unknown: no advancement
	return dueDate
}

// clampDayToMonth creates a date with the given day, clamped to the last day of the target month.
// It preserves the clock time (hour, minute, second, nanosecond) from the reference time.
func clampDayToMonth(year int, month time.Month, day int, ref time.Time) time.Time {
	loc := ref.Location()
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()

	if day > lastDay {
		day = lastDay
	}

	return time.Date(year, month, day, ref.Hour(), ref.Minute(), ref.Second(), ref.Nanosecond(), loc)
}

// AdvanceRecurrence advances the deadline to the next occurrence if it was delivered and the
// current dueDate has passed. For recurring deadlines (not "once"), this resets deliveredAt
// and moves dueDate forward until it reaches a future date.
// This implements the lazy recurrence advancement described in the TRD (computed on GET).
func (d *Deadline) AdvanceRecurrence(now time.Time) {
	if d.DeliveredAt == nil || d.Frequency == "once" {
		return
	}

	if !now.After(d.DueDate) {
		return
	}

	originalDueDate := d.DueDate

	for now.After(d.DueDate) {
		next := NextDueDate(d.DueDate, d.Frequency, d.MonthsOfYear)
		if !next.After(d.DueDate) {
			// Safety: prevent infinite loop if NextDueDate doesn't advance
			break
		}

		d.DueDate = next
	}

	if d.DueDate != originalDueDate {
		d.DeliveredAt = nil
		d.Status = ComputeStatus(d.DueDate, nil, now)
	}
}

// ComputeStatus determines the deadline status based on dueDate and deliveredAt.
// - If deliveredAt is non-nil, status is "delivered"
// - If dueDate is before now and not delivered, status is "overdue"
// - Otherwise, status is "pending"
func ComputeStatus(dueDate time.Time, deliveredAt *time.Time, now ...time.Time) string {
	if deliveredAt != nil {
		return StatusDelivered
	}

	ref := time.Now()
	if len(now) > 0 {
		ref = now[0]
	}

	if ref.After(dueDate) {
		return StatusOverdue
	}

	return StatusPending
}

// DeadlineMongoDBModel represents the MongoDB model for a deadline.
type DeadlineMongoDBModel struct {
	ID               uuid.UUID  `bson:"_id"`
	Name             string     `bson:"name"`
	Description      string     `bson:"description,omitempty"`
	Type             string     `bson:"type"`
	TemplateID       *uuid.UUID `bson:"template_id,omitempty"`
	TemplateName     string     `bson:"template_name,omitempty"`
	DueDate          time.Time  `bson:"due_date"`
	Frequency        string     `bson:"frequency"`
	MonthsOfYear     []int      `bson:"months_of_year,omitempty"`
	Active           bool       `bson:"active"`
	NotifyDaysBefore int        `bson:"notify_days_before"`
	Color            string     `bson:"color"`
	DeliveredAt      *time.Time `bson:"delivered_at,omitempty"`
	CreatedAt        time.Time  `bson:"created_at"`
	UpdatedAt        time.Time  `bson:"updated_at"`
	DeletedAt        *time.Time `bson:"deleted_at,omitempty"`
}

// ToEntity converts DeadlineMongoDBModel to Deadline using ReconstructDeadline.
// For recurring deadlines that were delivered and whose dueDate has passed,
// it automatically advances to the next occurrence and resets deliveredAt.
func (dm *DeadlineMongoDBModel) ToEntity() *Deadline {
	d := ReconstructDeadline(
		dm.ID,
		dm.Name,
		dm.Description,
		dm.Type,
		dm.TemplateID,
		dm.TemplateName,
		dm.DueDate,
		dm.Frequency,
		dm.MonthsOfYear,
		dm.Active,
		dm.NotifyDaysBefore,
		dm.Color,
		dm.DeliveredAt,
		dm.CreatedAt,
		dm.UpdatedAt,
	)

	d.AdvanceRecurrence(time.Now())

	return d
}

// FromDeadlineEntity creates a new DeadlineMongoDBModel from a Deadline domain entity.
// This is the preferred way to build a complete model for persistence.
func FromDeadlineEntity(d *Deadline) *DeadlineMongoDBModel {
	return &DeadlineMongoDBModel{
		ID:               d.ID,
		Name:             d.Name,
		Description:      d.Description,
		Type:             d.Type,
		TemplateID:       d.TemplateID,
		TemplateName:     d.TemplateName,
		DueDate:          d.DueDate,
		Frequency:        d.Frequency,
		MonthsOfYear:     d.MonthsOfYear,
		Active:           d.Active,
		NotifyDaysBefore: d.NotifyDaysBefore,
		Color:            d.Color,
		DeliveredAt:      d.DeliveredAt,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
	}
}

// CreateDeadlineInput is the input payload for creating a deadline.
// Public fields are required for JSON binding (json tags) and validation (validate tags).
//
// swagger:model CreateDeadlineInput
//
//	@Description	CreateDeadlineInput is the input payload to create a deadline.
type CreateDeadlineInput struct {
	Name             string     `json:"name" validate:"required" example:"Monthly Regulatory Report"`
	Description      string     `json:"description,omitempty" example:"Monthly regulatory compliance report"`
	Type             string     `json:"type" validate:"required" example:"regulatory"`
	TemplateID       *uuid.UUID `json:"templateId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	DueDate          time.Time  `json:"dueDate" validate:"required" example:"2026-03-31T23:59:59Z"`
	Frequency        string     `json:"frequency" validate:"required" example:"monthly"`
	MonthsOfYear     []int      `json:"monthsOfYear,omitempty" example:"1,6"`
	Active           *bool      `json:"active,omitempty" example:"true"`
	NotifyDaysBefore int        `json:"notifyDaysBefore,omitempty" example:"5"`
	Color            string     `json:"color" validate:"required" example:"#FF5733"`
} //	@name	CreateDeadlineInput

// UpdateDeadlineInput is the input payload for updating a deadline (partial update).
// All fields are optional for PATCH-style partial updates.
//
// swagger:model UpdateDeadlineInput
//
//	@Description	UpdateDeadlineInput is the input payload to update a deadline.
type UpdateDeadlineInput struct {
	Name             *string    `json:"name,omitempty" example:"Updated Report Name"`
	Description      *string    `json:"description,omitempty" example:"Updated description"`
	Type             *string    `json:"type,omitempty" example:"custom"`
	TemplateID       *uuid.UUID `json:"templateId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	DueDate          *time.Time `json:"dueDate,omitempty" example:"2026-06-30T23:59:59Z"`
	Frequency        *string    `json:"frequency,omitempty" example:"annual"`
	MonthsOfYear     []int      `json:"monthsOfYear,omitempty" example:"1,6"`
	Active           *bool      `json:"active,omitempty" example:"false"`
	NotifyDaysBefore *int       `json:"notifyDaysBefore,omitempty" example:"10"`
	Color            *string    `json:"color,omitempty" example:"#00FF00"`
} //	@name	UpdateDeadlineInput

// DeliverDeadlineInput is the input payload for the deliver endpoint.
//
// swagger:model DeliverDeadlineInput
//
//	@Description	DeliverDeadlineInput is the input payload to mark a deadline as delivered or not.
type DeliverDeadlineInput struct {
	Delivered *bool `json:"delivered" validate:"required" example:"true"`
} //	@name	DeliverDeadlineInput
