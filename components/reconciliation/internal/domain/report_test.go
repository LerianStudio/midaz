package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReconciliationReport_DetermineOverallStatus_Healthy(t *testing.T) {
	t.Parallel()

	report := &ReconciliationReport{
		BalanceCheck:     &BalanceCheckResult{Status: StatusHealthy},
		DoubleEntryCheck: &DoubleEntryCheckResult{Status: StatusHealthy},
		ReferentialCheck: &ReferentialCheckResult{Status: StatusHealthy},
		SyncCheck:        &SyncCheckResult{Status: StatusHealthy},
		OrphanCheck:      &OrphanCheckResult{Status: StatusHealthy},
		MetadataCheck:    &MetadataCheckResult{Status: StatusHealthy},
		DLQCheck:         &DLQCheckResult{Status: StatusHealthy},
	}

	report.DetermineOverallStatus()

	assert.Equal(t, StatusHealthy, report.Status)
}

func TestReconciliationReport_DetermineOverallStatus_Warning(t *testing.T) {
	t.Parallel()

	report := &ReconciliationReport{
		BalanceCheck:     &BalanceCheckResult{Status: StatusWarning},
		DoubleEntryCheck: &DoubleEntryCheckResult{Status: StatusHealthy},
		ReferentialCheck: &ReferentialCheckResult{Status: StatusHealthy},
		SyncCheck:        &SyncCheckResult{Status: StatusHealthy},
		OrphanCheck:      &OrphanCheckResult{Status: StatusHealthy},
		MetadataCheck:    &MetadataCheckResult{Status: StatusHealthy},
		DLQCheck:         &DLQCheckResult{Status: StatusHealthy},
	}

	report.DetermineOverallStatus()

	assert.Equal(t, StatusWarning, report.Status)
}

func TestReconciliationReport_DetermineOverallStatus_Critical(t *testing.T) {
	t.Parallel()

	report := &ReconciliationReport{
		BalanceCheck:     &BalanceCheckResult{Status: StatusWarning},
		DoubleEntryCheck: &DoubleEntryCheckResult{Status: StatusCritical}, // Even one CRITICAL = overall CRITICAL
		ReferentialCheck: &ReferentialCheckResult{Status: StatusHealthy},
		SyncCheck:        &SyncCheckResult{Status: StatusHealthy},
		OrphanCheck:      &OrphanCheckResult{Status: StatusHealthy},
		MetadataCheck:    &MetadataCheckResult{Status: StatusHealthy},
		DLQCheck:         &DLQCheckResult{Status: StatusHealthy},
	}

	report.DetermineOverallStatus()

	assert.Equal(t, StatusCritical, report.Status)
}

func TestReconciliationReport_DetermineOverallStatus_MultipleWarnings(t *testing.T) {
	t.Parallel()

	report := &ReconciliationReport{
		BalanceCheck:     &BalanceCheckResult{Status: StatusWarning},
		DoubleEntryCheck: &DoubleEntryCheckResult{Status: StatusHealthy},
		ReferentialCheck: &ReferentialCheckResult{Status: StatusWarning},
		SyncCheck:        &SyncCheckResult{Status: StatusWarning},
		OrphanCheck:      &OrphanCheckResult{Status: StatusHealthy},
		MetadataCheck:    &MetadataCheckResult{Status: StatusHealthy},
		DLQCheck:         &DLQCheckResult{Status: StatusHealthy},
	}

	report.DetermineOverallStatus()

	// Multiple warnings still = WARNING (not CRITICAL)
	assert.Equal(t, StatusWarning, report.Status)
}

func TestReconciliationReport_DetermineOverallStatus_NilChecks(t *testing.T) {
	t.Parallel()

	report := &ReconciliationReport{
		BalanceCheck:     nil, // Some checks may be nil
		DoubleEntryCheck: &DoubleEntryCheckResult{Status: StatusHealthy},
		ReferentialCheck: nil,
		SyncCheck:        &SyncCheckResult{Status: StatusHealthy},
		OrphanCheck:      nil,
		MetadataCheck:    nil,
		DLQCheck:         nil,
	}

	report.DetermineOverallStatus()

	assert.Equal(t, StatusHealthy, report.Status)
}

func TestReconciliationReport_DetermineOverallStatus_AllNil(t *testing.T) {
	t.Parallel()

	report := &ReconciliationReport{}

	report.DetermineOverallStatus()

	// No checks means HEALTHY (no issues detected)
	assert.Equal(t, StatusHealthy, report.Status)
}

func TestReconciliationReport_DetermineOverallStatus_CriticalOverridesWarning(t *testing.T) {
	t.Parallel()

	report := &ReconciliationReport{
		BalanceCheck:     &BalanceCheckResult{Status: StatusWarning},
		DoubleEntryCheck: &DoubleEntryCheckResult{Status: StatusWarning},
		ReferentialCheck: &ReferentialCheckResult{Status: StatusCritical},
		SyncCheck:        &SyncCheckResult{Status: StatusWarning},
		OrphanCheck:      &OrphanCheckResult{Status: StatusWarning},
		MetadataCheck:    &MetadataCheckResult{Status: StatusWarning},
		DLQCheck:         &DLQCheckResult{Status: StatusWarning},
	}

	report.DetermineOverallStatus()

	// CRITICAL overrides all warnings
	assert.Equal(t, StatusCritical, report.Status)
}
