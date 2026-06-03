//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package property

import (
	"testing"
	"testing/quick"
	"time"

	"github.com/LerianStudio/reporter/pkg/mongodb/deadline"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// validTypes is the set of valid deadline types for generating test inputs.
var validTypes = []string{"regulatory", "custom"}

// validFrequencies is the set of valid deadline frequencies for generating test inputs.
var validFrequencies = []string{"once", "daily", "weekly", "monthly", "semiannual", "annual"}

// pickType selects a valid type from a seed byte.
func pickType(seed byte) string {
	return validTypes[int(seed)%len(validTypes)]
}

// pickFrequency selects a valid frequency from a seed byte.
func pickFrequency(seed byte) string {
	return validFrequencies[int(seed)%len(validFrequencies)]
}

// --- Property 1: Status Computation Trichotomy ---
// ComputeStatus ALWAYS returns exactly one of {"pending", "overdue", "delivered"}.

func TestProperty_ComputeStatus_Trichotomy(t *testing.T) {
	t.Parallel()

	validStatuses := map[string]bool{
		deadline.StatusPending:   true,
		deadline.StatusOverdue:   true,
		deadline.StatusDelivered: true,
	}

	property := func(dueDateOffset int64, hasDelivered bool) bool {
		// Bound offset to reasonable range (-10 years to +10 years in seconds)
		if dueDateOffset > 315360000 {
			dueDateOffset = dueDateOffset % 315360000
		}
		if dueDateOffset < -315360000 {
			dueDateOffset = -((-dueDateOffset) % 315360000)
		}

		dueDate := time.Now().Add(time.Duration(dueDateOffset) * time.Second)

		var deliveredAt *time.Time
		if hasDelivered {
			now := time.Now()
			deliveredAt = &now
		}

		status := deadline.ComputeStatus(dueDate, deliveredAt)

		// Must be exactly one of the valid statuses
		return validStatuses[status]
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Property 2: Delivered Status Dominance ---
// If deliveredAt is non-nil, status is ALWAYS "delivered" regardless of dueDate.

func TestProperty_ComputeStatus_DeliveredDominance(t *testing.T) {
	t.Parallel()

	property := func(dueDateOffset int64) bool {
		if dueDateOffset > 315360000 {
			dueDateOffset = dueDateOffset % 315360000
		}
		if dueDateOffset < -315360000 {
			dueDateOffset = -((-dueDateOffset) % 315360000)
		}

		dueDate := time.Now().Add(time.Duration(dueDateOffset) * time.Second)
		now := time.Now()
		deliveredAt := &now

		status := deadline.ComputeStatus(dueDate, deliveredAt)
		return status == deadline.StatusDelivered
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Property 3: Overdue Requires Past DueDate ---
// Status is "overdue" ONLY when dueDate < now AND deliveredAt is nil.

func TestProperty_ComputeStatus_OverdueRequiresPastDueDate(t *testing.T) {
	t.Parallel()

	property := func(secondsInPast uint32) bool {
		// Ensure dueDate is in the past (at least 2 seconds ago to avoid race)
		offset := time.Duration(secondsInPast%315360000+2) * time.Second
		dueDate := time.Now().Add(-offset)

		status := deadline.ComputeStatus(dueDate, nil)
		return status == deadline.StatusOverdue
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Property 4: Pending Requires Future DueDate ---
// Status is "pending" ONLY when dueDate >= now AND deliveredAt is nil.

func TestProperty_ComputeStatus_PendingRequiresFutureDueDate(t *testing.T) {
	t.Parallel()

	property := func(secondsInFuture uint32) bool {
		// Ensure dueDate is far enough in the future (at least 10 seconds)
		offset := time.Duration(secondsInFuture%315360000+10) * time.Second
		dueDate := time.Now().Add(offset)

		status := deadline.ComputeStatus(dueDate, nil)
		return status == deadline.StatusPending
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Property 5: NewDeadline Validation Completeness ---
// NewDeadline with valid inputs ALWAYS succeeds; with any invalid input ALWAYS fails (never panics).

func TestProperty_NewDeadline_ValidInputsSucceed(t *testing.T) {
	t.Parallel()

	property := func(typeSeed, freqSeed byte, nameSuffix string) bool {
		if len(nameSuffix) > 1000 {
			return true
		}

		id := uuid.New()
		name := "Deadline-" + nameSuffix
		if name == "" {
			name = "Deadline-test"
		}

		deadlineType := pickType(typeSeed)
		frequency := pickFrequency(freqSeed)
		dueDate := time.Now().Add(24 * time.Hour)
		color := "#FF5733"

		result, err := deadline.NewDeadline(id, name, deadlineType, dueDate, frequency, color)
		if err != nil {
			return false
		}

		return result != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

func TestProperty_NewDeadline_NilIDAlwaysFails(t *testing.T) {
	t.Parallel()

	property := func(typeSeed, freqSeed byte) bool {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewDeadline panicked with nil ID: %v", r)
			}
		}()

		deadlineType := pickType(typeSeed)
		frequency := pickFrequency(freqSeed)

		_, err := deadline.NewDeadline(uuid.Nil, "TestName", deadlineType, time.Now().Add(24*time.Hour), frequency, "#FF5733")
		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

func TestProperty_NewDeadline_EmptyNameAlwaysFails(t *testing.T) {
	t.Parallel()

	property := func(typeSeed, freqSeed byte) bool {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewDeadline panicked with empty name: %v", r)
			}
		}()

		deadlineType := pickType(typeSeed)
		frequency := pickFrequency(freqSeed)

		_, err := deadline.NewDeadline(uuid.New(), "", deadlineType, time.Now().Add(24*time.Hour), frequency, "#FF5733")
		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

func TestProperty_NewDeadline_InvalidTypeAlwaysFails(t *testing.T) {
	t.Parallel()

	property := func(invalidType string) bool {
		// Skip if accidentally a valid type
		if deadline.ValidTypes[invalidType] {
			return true
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewDeadline panicked with invalid type %q: %v", invalidType, r)
			}
		}()

		_, err := deadline.NewDeadline(uuid.New(), "TestName", invalidType, time.Now().Add(24*time.Hour), "monthly", "#FF5733")
		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

func TestProperty_NewDeadline_InvalidFrequencyAlwaysFails(t *testing.T) {
	t.Parallel()

	property := func(invalidFreq string) bool {
		if deadline.ValidFrequencies[invalidFreq] {
			return true
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewDeadline panicked with invalid frequency %q: %v", invalidFreq, r)
			}
		}()

		_, err := deadline.NewDeadline(uuid.New(), "TestName", "regulatory", time.Now().Add(24*time.Hour), invalidFreq, "#FF5733")
		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

func TestProperty_NewDeadline_ZeroDueDateAlwaysFails(t *testing.T) {
	t.Parallel()

	property := func(typeSeed, freqSeed byte) bool {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewDeadline panicked with zero dueDate: %v", r)
			}
		}()

		deadlineType := pickType(typeSeed)
		frequency := pickFrequency(freqSeed)

		_, err := deadline.NewDeadline(uuid.New(), "TestName", deadlineType, time.Time{}, frequency, "#FF5733")
		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

func TestProperty_NewDeadline_EmptyColorAlwaysFails(t *testing.T) {
	t.Parallel()

	property := func(typeSeed, freqSeed byte) bool {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewDeadline panicked with empty color: %v", r)
			}
		}()

		deadlineType := pickType(typeSeed)
		frequency := pickFrequency(freqSeed)

		_, err := deadline.NewDeadline(uuid.New(), "TestName", deadlineType, time.Now().Add(24*time.Hour), frequency, "")
		return err != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Property 6: Entity Round-Trip ---
// Deadline -> DeadlineMongoDBModel -> Deadline preserves all fields.

func TestProperty_Deadline_EntityRoundTrip(t *testing.T) {
	t.Parallel()

	property := func(
		nameSuffix string,
		typeSeed, freqSeed byte,
		description string,
		hasTemplateID bool,
		active bool,
		notifyDays uint8,
	) bool {
		if len(nameSuffix) > 500 || len(description) > 500 {
			return true
		}

		deadlineType := pickType(typeSeed)
		frequency := pickFrequency(freqSeed)
		id := uuid.New()
		name := "Deadline-" + nameSuffix
		if name == "Deadline-" {
			name = "Deadline-test"
		}

		dueDate := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		var templateID *uuid.UUID
		if hasTemplateID {
			tid := uuid.New()
			templateID = &tid
		}

		monthsOfYear := []int{1, 6}

		original := deadline.ReconstructDeadline(
			id, name, description, deadlineType,
			templateID, "TemplateName",
			dueDate, frequency,
			monthsOfYear,
			active, int(notifyDays),
			"#FF5733",
			nil, // deliveredAt
			now, now,
		)

		// Convert to MongoDB model and back
		model := deadline.FromDeadlineEntity(original)
		restored := model.ToEntity()

		// Verify all persisted fields are preserved
		if original.ID != restored.ID {
			return false
		}
		if original.Name != restored.Name {
			return false
		}
		if original.Description != restored.Description {
			return false
		}
		if original.Type != restored.Type {
			return false
		}
		if original.Frequency != restored.Frequency {
			return false
		}
		if !original.DueDate.Equal(restored.DueDate) {
			return false
		}
		if original.Active != restored.Active {
			return false
		}
		if original.NotifyDaysBefore != restored.NotifyDaysBefore {
			return false
		}
		if original.Color != restored.Color {
			return false
		}
		if !original.CreatedAt.Equal(restored.CreatedAt) {
			return false
		}
		if !original.UpdatedAt.Equal(restored.UpdatedAt) {
			return false
		}
		if original.TemplateName != restored.TemplateName {
			return false
		}

		// Check optional fields
		if (original.TemplateID == nil) != (restored.TemplateID == nil) {
			return false
		}
		if original.TemplateID != nil && *original.TemplateID != *restored.TemplateID {
			return false
		}
		// Status is recomputed by ToEntity via ReconstructDeadline, so it should match
		// the expected computation for the same inputs.
		expectedStatus := deadline.ComputeStatus(original.DueDate, original.DeliveredAt)
		if restored.Status != expectedStatus {
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Property 7: Type Enum Invariant ---
// After successful NewDeadline construction, Type is ALWAYS "regulatory" or "custom".

func TestProperty_NewDeadline_TypeEnumInvariant(t *testing.T) {
	t.Parallel()

	property := func(typeSeed, freqSeed byte) bool {
		deadlineType := pickType(typeSeed)
		frequency := pickFrequency(freqSeed)

		d, err := deadline.NewDeadline(
			uuid.New(), "TestName", deadlineType,
			time.Now().Add(24*time.Hour), frequency, "#FF5733",
		)
		if err != nil {
			return false
		}

		return deadline.ValidTypes[d.Type]
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Property 8: Frequency Enum Invariant ---
// After successful construction, Frequency is ALWAYS one of the valid values.

func TestProperty_NewDeadline_FrequencyEnumInvariant(t *testing.T) {
	t.Parallel()

	property := func(typeSeed, freqSeed byte) bool {
		deadlineType := pickType(typeSeed)
		frequency := pickFrequency(freqSeed)

		d, err := deadline.NewDeadline(
			uuid.New(), "TestName", deadlineType,
			time.Now().Add(24*time.Hour), frequency, "#FF5733",
		)
		if err != nil {
			return false
		}

		return deadline.ValidFrequencies[d.Frequency]
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Property 9: Soft Delete Idempotency ---
// Setting DeletedAt multiple times doesn't change the effective deleted state.

func TestProperty_DeadlineModel_SoftDeleteIdempotency(t *testing.T) {
	t.Parallel()

	property := func(seed uint32) bool {
		now := time.Now()
		model := &deadline.DeadlineMongoDBModel{
			ID:        uuid.New(),
			Name:      "Test",
			Type:      "regulatory",
			DueDate:   time.Now().Add(24 * time.Hour),
			Frequency: "monthly",
			Color:     "#FF5733",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}

		// First soft delete
		deletedAt1 := time.Now()
		model.DeletedAt = &deletedAt1

		isDeletedAfterFirst := model.DeletedAt != nil

		// Second soft delete (idempotent - setting again)
		deletedAt2 := time.Now().Add(time.Duration(seed%3600) * time.Second)
		model.DeletedAt = &deletedAt2

		isDeletedAfterSecond := model.DeletedAt != nil

		// Effective state should remain "deleted" regardless of how many times set
		return isDeletedAfterFirst == isDeletedAfterSecond && isDeletedAfterFirst
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Additional Property: ComputeStatus is deterministic ---
// Same inputs always produce same output.

func TestProperty_ComputeStatus_Deterministic(t *testing.T) {
	t.Parallel()

	property := func(dueDateOffset int64, hasDelivered bool) bool {
		if dueDateOffset > 315360000 {
			dueDateOffset = dueDateOffset % 315360000
		}
		if dueDateOffset < -315360000 {
			dueDateOffset = -((-dueDateOffset) % 315360000)
		}

		dueDate := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC).Add(time.Duration(dueDateOffset) * time.Second)

		var deliveredAt *time.Time
		if hasDelivered {
			t := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
			deliveredAt = &t
		}

		status1 := deadline.ComputeStatus(dueDate, deliveredAt)
		status2 := deadline.ComputeStatus(dueDate, deliveredAt)

		return status1 == status2
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}

// --- Additional Property: NewDeadline sets Status via ComputeStatus ---
// The status field of a newly created deadline must match ComputeStatus.

func TestProperty_NewDeadline_StatusMatchesComputeStatus(t *testing.T) {
	t.Parallel()

	property := func(typeSeed, freqSeed byte) bool {
		deadlineType := pickType(typeSeed)
		frequency := pickFrequency(freqSeed)
		dueDate := time.Now().Add(24 * time.Hour) // future date

		d, err := deadline.NewDeadline(
			uuid.New(), "TestName", deadlineType,
			dueDate, frequency, "#FF5733",
		)
		if err != nil {
			return false
		}

		// NewDeadline passes nil for deliveredAt
		expectedStatus := deadline.ComputeStatus(dueDate, nil)
		return d.Status == expectedStatus
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err)
}
