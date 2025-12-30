package postgres

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestDetermineStatus_Healthy(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(0, StatusThresholds{WarningThreshold: 10})
	assert.Equal(t, domain.StatusHealthy, result)
}

func TestDetermineStatus_Warning(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(5, StatusThresholds{WarningThreshold: 10})
	assert.Equal(t, domain.StatusWarning, result)
}

func TestDetermineStatus_Critical(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(15, StatusThresholds{WarningThreshold: 10})
	assert.Equal(t, domain.StatusCritical, result)
}

func TestDetermineStatus_CriticalOnAny(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(1, StatusThresholds{CriticalOnAny: true})
	assert.Equal(t, domain.StatusCritical, result)
}

func TestDetermineStatus_CriticalOnAnyZero(t *testing.T) {
	t.Parallel()

	// Even with CriticalOnAny, zero issues is HEALTHY
	result := DetermineStatus(0, StatusThresholds{CriticalOnAny: true})
	assert.Equal(t, domain.StatusHealthy, result)
}

func TestDetermineStatusWithPartial_Healthy(t *testing.T) {
	t.Parallel()

	result := DetermineStatusWithPartial(0, 0)
	assert.Equal(t, domain.StatusHealthy, result)
}

func TestDetermineStatusWithPartial_Warning(t *testing.T) {
	t.Parallel()

	result := DetermineStatusWithPartial(0, 5)
	assert.Equal(t, domain.StatusWarning, result)
}

func TestDetermineStatusWithPartial_Critical(t *testing.T) {
	t.Parallel()

	result := DetermineStatusWithPartial(3, 5)
	assert.Equal(t, domain.StatusCritical, result)
}

func TestDetermineStatus_ExclusiveThreshold(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(10, StatusThresholds{
		WarningThreshold:          10,
		WarningThresholdExclusive: true,
	})
	assert.Equal(t, domain.StatusCritical, result)
}

func TestDetermineStatus_NonExclusiveThresholdEquality(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(10, StatusThresholds{
		WarningThreshold:          10,
		WarningThresholdExclusive: false,
	})
	assert.Equal(t, domain.StatusWarning, result)
}
