//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"
	"testing/quick"
	"time"

	"github.com/stretchr/testify/require"
)

// TestProperty_ComputeNotificationSeverity_AlwaysValidSeverity verifies that
// ComputeNotificationSeverity always returns one of the three valid severity
// strings ("overdue", "warning", "info") for any integer input. This is the
// core domain invariant: severity classification must be exhaustive.
func TestProperty_ComputeNotificationSeverity_AlwaysValidSeverity(t *testing.T) {
	t.Parallel()

	validSeverities := map[string]bool{
		overdueSeverity: true,
		warningSeverity: true,
		infoSeverity:    true,
	}

	property := func(daysUntilDue int) bool {
		severity := ComputeNotificationSeverity(daysUntilDue)
		if !validSeverities[severity] {
			t.Logf("Invalid severity %q for daysUntilDue=%d", severity, daysUntilDue)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: ComputeNotificationSeverity returned invalid severity")
}

// TestProperty_ComputeNotificationSeverity_BoundaryConsistency verifies that
// the severity boundaries are consistent: negative days always produce "overdue",
// days in [0, warningThresholdDays] always produce "warning", and days above
// warningThresholdDays always produce "info". This ensures no boundary gaps or
// overlaps exist.
func TestProperty_ComputeNotificationSeverity_BoundaryConsistency(t *testing.T) {
	t.Parallel()

	property := func(daysUntilDue int) bool {
		severity := ComputeNotificationSeverity(daysUntilDue)

		if daysUntilDue < 0 && severity != overdueSeverity {
			t.Logf("Expected %q for negative days %d, got %q",
				overdueSeverity, daysUntilDue, severity)
			return false
		}

		if daysUntilDue >= 0 && daysUntilDue <= warningThresholdDays && severity != warningSeverity {
			t.Logf("Expected %q for days %d in [0,%d], got %q",
				warningSeverity, daysUntilDue, warningThresholdDays, severity)
			return false
		}

		if daysUntilDue > warningThresholdDays && severity != infoSeverity {
			t.Logf("Expected %q for days %d > %d, got %q",
				infoSeverity, daysUntilDue, warningThresholdDays, severity)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: severity boundaries are inconsistent")
}

// TestProperty_ComputeDaysUntilDue_SignMatchesDirection verifies that
// ComputeDaysUntilDue returns a positive value when dueDate is in the future,
// negative when overdue, and zero when both dates are the same day. This is the
// directional consistency invariant.
func TestProperty_ComputeDaysUntilDue_SignMatchesDirection(t *testing.T) {
	t.Parallel()

	property := func(offsetDays int16) bool {
		// Use int16 to keep values reasonable (approx +/- 90 years)
		now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
		dueDate := now.AddDate(0, 0, int(offsetDays))

		result := ComputeDaysUntilDue(dueDate, now)

		dueTrunc := dueDate.Truncate(24 * time.Hour)
		nowTrunc := now.Truncate(24 * time.Hour)

		if dueTrunc.Equal(nowTrunc) {
			if result != 0 {
				t.Logf("Expected 0 for same day, got %d (offset=%d)", result, offsetDays)
				return false
			}

			return true
		}

		if dueTrunc.After(nowTrunc) && result <= 0 {
			t.Logf("Expected positive for future due, got %d (offset=%d)", result, offsetDays)
			return false
		}

		if dueTrunc.Before(nowTrunc) && result >= 0 {
			t.Logf("Expected negative for past due, got %d (offset=%d)", result, offsetDays)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: ComputeDaysUntilDue sign does not match direction")
}

// TestProperty_ComputeDaysUntilDue_Symmetric verifies that if dueDate is N days
// from now, ComputeDaysUntilDue returns exactly N. This is tested by constructing
// a due date that is exactly offsetDays away from a fixed reference time, both
// truncated to day boundaries.
func TestProperty_ComputeDaysUntilDue_Symmetric(t *testing.T) {
	t.Parallel()

	property := func(offsetDays int16) bool {
		// Use a fixed reference at midnight to avoid truncation surprises
		now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
		dueDate := now.AddDate(0, 0, int(offsetDays))

		result := ComputeDaysUntilDue(dueDate, now)

		if result != int(offsetDays) {
			t.Logf("ComputeDaysUntilDue(%v, %v) = %d, want %d",
				dueDate.Format(time.DateOnly), now.Format(time.DateOnly), result, offsetDays)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: ComputeDaysUntilDue is not symmetric with AddDate")
}

// TestProperty_ComputeNotificationSeverity_Monotonic verifies that as
// daysUntilDue increases, severity never becomes MORE urgent. The severity
// ordering is: overdue (most urgent) < warning < info (least urgent). This
// ensures monotonicity of the classification.
func TestProperty_ComputeNotificationSeverity_Monotonic(t *testing.T) {
	t.Parallel()

	severityRank := map[string]int{
		overdueSeverity: 0,
		warningSeverity: 1,
		infoSeverity:    2,
	}

	property := func(a, b int) bool {
		// Ensure a < b
		if a >= b {
			a, b = b, a

			if a == b {
				b = a + 1
			}
		}

		sevA := ComputeNotificationSeverity(a)
		sevB := ComputeNotificationSeverity(b)

		rankA := severityRank[sevA]
		rankB := severityRank[sevB]

		// More days until due (b > a) means severity should be same or less urgent
		if rankB < rankA {
			t.Logf("Monotonicity violated: days=%d -> %q (rank %d), days=%d -> %q (rank %d)",
				a, sevA, rankA, b, sevB, rankB)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: severity is not monotonically decreasing with days")
}

// TestProperty_SeverityOrder_ConsistentWithSeverityValues verifies that
// severityOrder returns distinct numeric values for each valid severity string,
// and that the ordering matches the domain invariant: overdue < warning < info.
func TestProperty_SeverityOrder_ConsistentWithSeverityValues(t *testing.T) {
	t.Parallel()

	property := func(daysUntilDue int) bool {
		severity := ComputeNotificationSeverity(daysUntilDue)
		order := severityOrder(severity)

		// Verify the order is within expected range [0, 2]
		if order < 0 || order > 2 {
			t.Logf("severityOrder(%q) = %d, expected [0, 2]", severity, order)
			return false
		}

		// Verify specific mapping
		expectedOrder := map[string]int{
			overdueSeverity: 0,
			warningSeverity: 1,
			infoSeverity:    2,
		}

		if order != expectedOrder[severity] {
			t.Logf("severityOrder(%q) = %d, expected %d",
				severity, order, expectedOrder[severity])
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: severityOrder inconsistent with severity values")
}
