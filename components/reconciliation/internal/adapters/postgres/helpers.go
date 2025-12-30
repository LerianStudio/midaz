package postgres

import "github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"

// StatusThresholds defines the thresholds for status determination.
type StatusThresholds struct {
	// WarningThreshold: if issues are within the threshold, status is WARNING (else CRITICAL).
	// If issues == 0, status is HEALTHY.
	WarningThreshold int

	// WarningThresholdExclusive makes the warning threshold exclusive when true.
	// Example: threshold=10 => WARNING for <10, CRITICAL for >=10.
	WarningThresholdExclusive bool

	// CriticalOnAny: if true, any issue is CRITICAL (used for double-entry).
	CriticalOnAny bool
}

// DetermineStatus calculates the status based on issue count and thresholds.
func DetermineStatus(issueCount int, thresholds StatusThresholds) domain.ReconciliationStatus {
	if issueCount == 0 {
		return domain.StatusHealthy
	}

	if thresholds.CriticalOnAny {
		return domain.StatusCritical
	}

	if thresholds.WarningThresholdExclusive {
		if issueCount < thresholds.WarningThreshold {
			return domain.StatusWarning
		}

		return domain.StatusCritical
	}

	if issueCount <= thresholds.WarningThreshold {
		return domain.StatusWarning
	}

	return domain.StatusCritical
}

// DetermineStatusWithPartial calculates status when there are both critical and partial issues.
// criticalCount: issues that are critical (e.g., orphan transactions)
// partialCount: issues that are warnings (e.g., partial transactions)
func DetermineStatusWithPartial(criticalCount, partialCount int) domain.ReconciliationStatus {
	if criticalCount == 0 && partialCount == 0 {
		return domain.StatusHealthy
	}

	if criticalCount > 0 {
		return domain.StatusCritical
	}

	return domain.StatusWarning
}
