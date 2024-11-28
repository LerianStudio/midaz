package transaction

import (
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq/operation"
	"time"
)

type Transaction struct {
	ID             string                `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID       string                `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string                `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	CreatedAt      time.Time             `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	Operations     []operation.Operation `json:"operations"`
}
