package constant

import (
	"fmt"
	"strings"
	"testing"
)

func TestAssertValidStatusCode_ValidCodes(t *testing.T) {
	validCodes := []string{CREATED, PENDING, APPROVED, CANCELED, NOTED}

	for _, code := range validCodes {
		t.Run(code, func(t *testing.T) {
			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Unexpected panic for valid code %s: %v", code, r)
				}
			}()
			AssertValidStatusCode(code)
		})
	}
}

func TestAssertValidStatusCode_InvalidCode_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for invalid status code")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "unknown transaction status code") {
			t.Errorf("Expected panic about unknown status code, got: %v", r)
		}
	}()

	AssertValidStatusCode("INVALID_STATUS")
}

func TestAssertValidStatusTransition_ValidTransitions(t *testing.T) {
	validTransitions := []struct {
		from string
		to   string
	}{
		{CREATED, PENDING},
		{CREATED, APPROVED},
		{CREATED, NOTED},
		{PENDING, APPROVED},
		{PENDING, CANCELED},
	}

	for _, tt := range validTransitions {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Unexpected panic for valid transition %s->%s: %v", tt.from, tt.to, r)
				}
			}()
			AssertValidStatusTransition(tt.from, tt.to)
		})
	}
}

func TestAssertValidStatusTransition_InvalidTransition_Panics(t *testing.T) {
	invalidTransitions := []struct {
		from string
		to   string
	}{
		{APPROVED, PENDING},  // Terminal state
		{CANCELED, APPROVED}, // Terminal state
		{PENDING, CREATED},   // Backward transition
	}

	for _, tt := range invalidTransitions {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("Expected panic for invalid transition %s->%s", tt.from, tt.to)
				}
				panicMsg := fmt.Sprintf("%v", r)
				if !strings.Contains(panicMsg, "invalid status transition") {
					t.Errorf("Expected panic about invalid transition, got: %v", r)
				}
			}()
			AssertValidStatusTransition(tt.from, tt.to)
		})
	}
}

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected bool
	}{
		{CREATED, false},
		{PENDING, false},
		{APPROVED, true},
		{CANCELED, true},
		{NOTED, true},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := IsTerminalStatus(tt.status)
			if result != tt.expected {
				t.Errorf("IsTerminalStatus(%s) = %v, want %v", tt.status, result, tt.expected)
			}
		})
	}
}
