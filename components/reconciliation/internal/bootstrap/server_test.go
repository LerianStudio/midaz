package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

func TestGetStatusColor_MappedStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status domain.ReconciliationStatus
		want   string
	}{
		{"healthy", domain.StatusHealthy, "#22c55e"},
		{"warning", domain.StatusWarning, "#eab308"},
		{"critical", domain.StatusCritical, "#ef4444"},
		{"error", domain.StatusError, "#b91c1c"},
		{"skipped", domain.StatusSkipped, "#3b82f6"},
		{"unknown", domain.StatusUnknown, "#6b7280"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, getStatusColor(tt.status))
		})
	}
}

func TestGetStatusColor_FallsBackToUnknownForUnmappedStatus(t *testing.T) {
	t.Parallel()

	// If a new status is introduced but not added to statusColorMap, it should
	// deterministically fall back to UNKNOWN until explicitly mapped.
	got := getStatusColor(domain.ReconciliationStatus("BOGUS_STATUS"))
	assert.Equal(t, "#6b7280", got)
}
