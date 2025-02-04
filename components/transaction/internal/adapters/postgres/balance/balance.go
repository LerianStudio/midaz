package balance

import (
	"database/sql"
	"time"
)

type BalancePostgreSQLModel struct {
	ID             string
	Alias          *string
	LedgerID       string
	OrganizationID string
	AssetCode      string
	Available      *float64
	OnHold         *float64
	Scale          *float64
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      sql.NullTime
}
