package mmodel

import (
	"github.com/google/uuid"
	"time"
)

// CreateAssetRateInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateAssetRateInput
// @Description CreateAssetRateInput is the input payload to create an asset rate.
type CreateAssetRateInput struct {
	FromAssetCode string         `json:"fromAssetCode" validate:"required,max=100" example:"USD"`
	ToAssetCode   string         `json:"toAssetCode" validate:"required,max=100" example:"BRL"`
	Rate          int64          `json:"rate" validate:"required" example:"52000"`
	RateScale     int64          `json:"rateScale" validate:"required" example:"4"`
	Metadata      map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateAssetRateInput

// UpdateAssetRateInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateAssetRateInput
// @Description UpdateAssetRateInput is the input payload to update an asset rate.
type UpdateAssetRateInput struct {
	Rate      int64          `json:"rate" validate:"required" example:"52000"`
	RateScale int64          `json:"rateScale" validate:"required" example:"4"`
	Status    Status         `json:"status"`
	Metadata  map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name UpdateAssetRateInput

// AssetRate is a struct designed to encapsulate response payload data.
//
// swagger:model AssetRate
// @Description AssetRate is a struct designed to store asset rate data.
type AssetRate struct {
	ID            string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	FromAssetCode string         `json:"fromAssetCode" example:"USD"`
	ToAssetCode   string         `json:"toAssetCode" example:"BRL"`
	Rate          int64          `json:"rate" example:"52000"`
	RateScale     int64          `json:"rateScale" example:"4"`
	Status        *Status        `json:"status"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     time.Time      `json:"createdAt" example:"2023-01-01T00:00:00Z"`
	UpdatedAt     time.Time      `json:"updatedAt" example:"2023-01-01T00:00:00Z"`
} // @name AssetRate

// IDtoUUID is a func that convert UUID string to uuid.UUID
func (ar *AssetRate) IDtoUUID() uuid.UUID {
	id, _ := uuid.Parse(ar.ID)
	return id
}

// AssetRates struct to return get all.
//
// swagger:model AssetRates
// @Description AssetRates is the struct designed to return a list of asset rates with pagination.
type AssetRates struct {
	Items      []AssetRate `json:"items"`
	Pagination *Pagination `json:"pagination"`
} // @name AssetRates
