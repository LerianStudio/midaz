package assetrate

import (
	"testing"
	"time"
)

func TestAssetRateModel_RoundTrip(t *testing.T) {
	now := time.Now().UTC()
	scale := 2.0
	src := &AssetRate{
		ID:             "ar-1",
		OrganizationID: "org-1",
		LedgerID:       "led-1",
		ExternalID:     "ext-1",
		From:           "USD",
		To:             "BRL",
		Rate:           5.25,
		Scale:          &scale,
		Source:         nil,
		TTL:            3600,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	var model AssetRatePostgreSQLModel
	model.FromEntity(src)
	dst := model.ToEntity()

	if dst.From != src.From || dst.To != src.To || *dst.Scale != *src.Scale || dst.TTL != src.TTL {
		t.Fatalf("mismatch: %+v vs %+v", dst, src)
	}
}
