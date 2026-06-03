// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package deadline

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// FuzzNewDeadline_Name fuzzes the NewDeadline constructor focusing on name input.
// Verifies: no panics, returns error for invalid input, returns valid Deadline for valid input.
func FuzzNewDeadline_Name(f *testing.F) {
	// Seed corpus: valid, empty, boundary, unicode, security payloads
	f.Add("Monthly Regulatory Report")
	f.Add("")
	f.Add(strings.Repeat("a", 2000))
	f.Add("\u540d\u524d\u30c6\u30b9\u30c8\u2603\u2764\ufe0f")
	f.Add("<script>alert('xss')</script>")
	f.Add("' OR 1=1 --")
	f.Add("\x00\x01\x02\x03null\x7f")
	f.Add("   \t\n\r   ")

	validID := uuid.New()
	validDueDate := time.Now().Add(24 * time.Hour)

	f.Fuzz(func(t *testing.T, name string) {
		// Bound input to prevent OOM
		if len(name) > 4096 {
			name = name[:4096]
		}

		d, err := NewDeadline(validID, name, "regulatory", validDueDate, "monthly", "#FF5733")

		// Invariant: must not panic (implicit by reaching here)
		// Invariant: empty name must return error
		if name == "" {
			if err == nil {
				t.Fatalf("expected error for empty name, got nil")
			}

			if d != nil {
				t.Fatalf("expected nil deadline for empty name, got non-nil")
			}

			return
		}

		// For non-empty name: should succeed
		if err != nil {
			t.Fatalf("unexpected error for name=%q: %v", name, err)
		}

		if d == nil {
			t.Fatalf("expected non-nil deadline for name=%q, got nil", name)
		}

		if d.Name != name {
			t.Fatalf("expected name=%q, got=%q", name, d.Name)
		}

		if d.Active != true {
			t.Fatalf("expected Active=true, got false")
		}
	})
}

// FuzzNewDeadline_TypeAndFrequency fuzzes the type and frequency fields.
// Verifies: only valid types/frequencies are accepted, all others rejected with error.
func FuzzNewDeadline_TypeAndFrequency(f *testing.F) {
	// Seed corpus covering valid and invalid combinations
	f.Add("regulatory", "monthly")
	f.Add("custom", "once")
	f.Add("custom", "daily")
	f.Add("custom", "weekly")
	f.Add("custom", "semiannual")
	f.Add("custom", "annual")
	f.Add("", "")
	f.Add("invalid", "invalid")
	f.Add("REGULATORY", "MONTHLY")
	f.Add("regulatory ", " monthly")
	f.Add("<script>", "'; DROP TABLE;--")
	f.Add(strings.Repeat("x", 1000), strings.Repeat("y", 1000))
	f.Add("\x00", "\x00")

	validID := uuid.New()
	validDueDate := time.Now().Add(24 * time.Hour)

	f.Fuzz(func(t *testing.T, deadlineType, frequency string) {
		// Bound inputs
		if len(deadlineType) > 4096 {
			deadlineType = deadlineType[:4096]
		}

		if len(frequency) > 4096 {
			frequency = frequency[:4096]
		}

		d, err := NewDeadline(validID, "Test Deadline", deadlineType, validDueDate, frequency, "#FF5733")

		// Invariant: no panics (implicit)

		isValidType := ValidTypes[deadlineType]
		isValidFreq := ValidFrequencies[frequency]

		if !isValidType || !isValidFreq {
			// Must return error for invalid type or frequency
			if err == nil {
				t.Fatalf("expected error for type=%q freq=%q, got nil", deadlineType, frequency)
			}

			if d != nil {
				t.Fatalf("expected nil deadline for invalid input, got non-nil")
			}

			return
		}

		// Both valid: should succeed
		if err != nil {
			t.Fatalf("unexpected error for type=%q freq=%q: %v", deadlineType, frequency, err)
		}

		if d.Type != deadlineType {
			t.Fatalf("expected type=%q, got=%q", deadlineType, d.Type)
		}

		if d.Frequency != frequency {
			t.Fatalf("expected frequency=%q, got=%q", frequency, d.Frequency)
		}
	})
}

// FuzzNewDeadline_Color fuzzes the color field.
// Verifies: empty color rejected, any non-empty color accepted.
func FuzzNewDeadline_Color(f *testing.F) {
	f.Add("#FF5733")
	f.Add("")
	f.Add("#000000")
	f.Add("red")
	f.Add("not-a-color")
	f.Add(strings.Repeat("#", 1000))
	f.Add("\x00\x01\x02")
	f.Add("rgb(255,0,0)")

	validID := uuid.New()
	validDueDate := time.Now().Add(24 * time.Hour)

	f.Fuzz(func(t *testing.T, color string) {
		if len(color) > 4096 {
			color = color[:4096]
		}

		d, err := NewDeadline(validID, "Test Deadline", "regulatory", validDueDate, "monthly", color)

		isValidHex := ValidColorRegex.MatchString(color)

		if !isValidHex {
			if err == nil {
				t.Fatalf("expected error for invalid color=%q, got nil", color)
			}

			if d != nil {
				t.Fatalf("expected nil deadline for invalid color=%q, got non-nil", color)
			}

			return
		}

		if err != nil {
			t.Fatalf("unexpected error for valid color=%q: %v", color, err)
		}

		if d.Color != color {
			t.Fatalf("expected color=%q, got=%q", color, d.Color)
		}
	})
}

// FuzzComputeStatus_Times fuzzes ComputeStatus with various time combinations.
// Verifies: always returns one of "pending", "overdue", "delivered" - no panics.
func FuzzComputeStatus_Times(f *testing.F) {
	now := time.Now()

	// Seed corpus: various time scenarios
	// (dueDateUnix int64, hasDelivered bool, deliveredOffsetSecs int64)
	f.Add(now.Add(24*time.Hour).Unix(), false, int64(0))     // future, not delivered -> pending
	f.Add(now.Add(-24*time.Hour).Unix(), false, int64(0))    // past, not delivered -> overdue
	f.Add(now.Add(-24*time.Hour).Unix(), true, int64(-3600)) // past, delivered -> delivered
	f.Add(now.Unix(), false, int64(0))                       // exactly now
	f.Add(int64(0), false, int64(0))                         // zero time
	f.Add(int64(-62135596800), false, int64(0))              // very old time (year 0)
	f.Add(int64(253402300799), false, int64(0))              // far future (year 9999)
	f.Add(now.Unix(), true, int64(0))                        // now, delivered

	validStatuses := map[string]bool{
		StatusPending:   true,
		StatusOverdue:   true,
		StatusDelivered: true,
	}

	f.Fuzz(func(t *testing.T, dueDateUnix int64, hasDelivered bool, deliveredOffsetSecs int64) {
		// Bound time values to avoid overflow
		if dueDateUnix < -62135596800 {
			dueDateUnix = -62135596800
		}

		if dueDateUnix > 253402300799 {
			dueDateUnix = 253402300799
		}

		if deliveredOffsetSecs < -315360000 {
			deliveredOffsetSecs = -315360000
		}

		if deliveredOffsetSecs > 315360000 {
			deliveredOffsetSecs = 315360000
		}

		dueDate := time.Unix(dueDateUnix, 0)

		var deliveredAt *time.Time

		if hasDelivered {
			t := time.Unix(dueDateUnix+deliveredOffsetSecs, 0)
			deliveredAt = &t
		}

		status := ComputeStatus(dueDate, deliveredAt)

		// Invariant: status must be one of the three valid values
		if !validStatuses[status] {
			t.Fatalf("invalid status=%q for dueDate=%v deliveredAt=%v", status, dueDate, deliveredAt)
		}

		// Invariant: if deliveredAt is non-nil, status must be "delivered"
		if deliveredAt != nil && status != StatusDelivered {
			t.Fatalf("expected delivered when deliveredAt=%v, got=%q", deliveredAt, status)
		}

		// Invariant: if deliveredAt is nil, status must be pending or overdue
		if deliveredAt == nil && status == StatusDelivered {
			t.Fatalf("got delivered without deliveredAt for dueDate=%v", dueDate)
		}
	})
}
