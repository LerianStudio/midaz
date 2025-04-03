package builders

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// mockAssetRateClient is a simple implementation of AssetRateClientInterface for testing
type mockAssetRateClient struct {
	createOrUpdateAssetRateFunc func(ctx context.Context, organizationID, ledgerID string, input *models.UpdateAssetRateInput) (*models.AssetRate, error)
}

// func (m *mockAssetRateClient) CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID string, input *models.UpdateAssetRateInput) (*models.AssetRate, error) { performs an operation
func (m *mockAssetRateClient) CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID string, input *models.UpdateAssetRateInput) (*models.AssetRate, error) {
	if m.createOrUpdateAssetRateFunc != nil {
		return m.createOrUpdateAssetRateFunc(ctx, organizationID, ledgerID, input)
	}
	return &models.AssetRate{ID: "rate-123"}, nil
}

// \1 performs an operation
func TestAssetRateBuilder(t *testing.T) {
	t.Run("Create with basic fields", func(t *testing.T) {
		now := time.Now()
		expiration := now.Add(24 * time.Hour * 30)

		client := &mockAssetRateClient{
			createOrUpdateAssetRateFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.UpdateAssetRateInput) (*models.AssetRate, error) {
				// Verify input values are correctly set
				if organizationID != "org-123" {
					t.Errorf("unexpected organization ID, got: %s, want: org-123", organizationID)
				}

				if ledgerID != "ledger-123" {
					t.Errorf("unexpected ledger ID, got: %s, want: ledger-123", ledgerID)
				}

				if input.FromAsset != "USD" {
					t.Errorf("unexpected source asset, got: %s, want: USD", input.FromAsset)
				}

				if input.ToAsset != "EUR" {
					t.Errorf("unexpected destination asset, got: %s, want: EUR", input.ToAsset)
				}

				if input.Rate != 0.85 {
					t.Errorf("unexpected rate, got: %f, want: 0.85", input.Rate)
				}

				return &models.AssetRate{
					ID:           "rate-123",
					FromAsset:    input.FromAsset,
					ToAsset:      input.ToAsset,
					Rate:         input.Rate,
					EffectiveAt:  input.EffectiveAt,
					ExpirationAt: input.ExpirationAt,
				}, nil
			},
		}

		builder := NewAssetRate(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithFromAsset("USD").
			WithToAsset("EUR").
			WithRate(0.85).
			WithEffectiveAt(now).
			WithExpirationAt(expiration)

		assetRate, err := builder.CreateOrUpdate(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if assetRate.ID != "rate-123" {
			t.Errorf("unexpected ID, got: %s, want: rate-123", assetRate.ID)
		}

		if assetRate.FromAsset != "USD" {
			t.Errorf("unexpected source asset, got: %s, want: USD", assetRate.FromAsset)
		}

		if assetRate.ToAsset != "EUR" {
			t.Errorf("unexpected destination asset, got: %s, want: EUR", assetRate.ToAsset)
		}

		if assetRate.Rate != 0.85 {
			t.Errorf("unexpected rate, got: %f, want: 0.85", assetRate.Rate)
		}
	})

	t.Run("Create with default dates", func(t *testing.T) {
		client := &mockAssetRateClient{
			createOrUpdateAssetRateFunc: func(ctx context.Context, organizationID, ledgerID string, input *models.UpdateAssetRateInput) (*models.AssetRate, error) {
				// Verify effective and expiration dates are set
				if input.EffectiveAt.IsZero() {
					t.Error("expected non-zero effective date")
				}

				if input.ExpirationAt.IsZero() {
					t.Error("expected non-zero expiration date")
				}
				// Verify expiration is after effective by roughly 30 days
				expectedDiff := 30 * 24 * time.Hour
				actualDiff := input.ExpirationAt.Sub(input.EffectiveAt)

				if actualDiff < (expectedDiff-time.Minute) || actualDiff > (expectedDiff+time.Minute) {
					t.Errorf("unexpected date difference, got: %v, want: ~%v", actualDiff, expectedDiff)
				}

				return &models.AssetRate{
					ID:           "rate-123",
					FromAsset:    input.FromAsset,
					ToAsset:      input.ToAsset,
					Rate:         input.Rate,
					EffectiveAt:  input.EffectiveAt,
					ExpirationAt: input.ExpirationAt,
				}, nil
			},
		}

		builder := NewAssetRate(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithFromAsset("USD").
			WithToAsset("EUR").
			WithRate(0.85)

		_, err := builder.CreateOrUpdate(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Validation errors", func(t *testing.T) {
		client := &mockAssetRateClient{}

		// Missing organization ID
		builder1 := NewAssetRate(client).
			WithLedger("ledger-123").
			WithFromAsset("USD").
			WithToAsset("EUR").
			WithRate(0.85)

		_, err := builder1.CreateOrUpdate(context.Background())

		if err == nil {
			t.Fatal("expected error for missing organization ID, got nil")
		}

		if !strings.Contains(err.Error(), "organization ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: organization ID is required", err.Error())
		}

		// Missing ledger ID
		builder2 := NewAssetRate(client).
			WithOrganization("org-123").
			WithFromAsset("USD").
			WithToAsset("EUR").
			WithRate(0.85)

		_, err = builder2.CreateOrUpdate(context.Background())
		if err == nil {
			t.Fatal("expected error for missing ledger ID, got nil")
		}

		if !strings.Contains(err.Error(), "ledger ID is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: ledger ID is required", err.Error())
		}

		// Missing source asset
		builder3 := NewAssetRate(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithToAsset("EUR").
			WithRate(0.85)

		_, err = builder3.CreateOrUpdate(context.Background())
		if err == nil {
			t.Fatal("expected error for missing source asset, got nil")
		}

		if !strings.Contains(err.Error(), "from asset is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: from asset is required", err.Error())
		}

		// Missing destination asset
		builder4 := NewAssetRate(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithFromAsset("USD").
			WithRate(0.85)

		_, err = builder4.CreateOrUpdate(context.Background())
		if err == nil {
			t.Fatal("expected error for missing destination asset, got nil")
		}

		if !strings.Contains(err.Error(), "to asset is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: to asset is required", err.Error())
		}

		// Missing rate
		builder5 := NewAssetRate(client).
			WithOrganization("org-123").
			WithLedger("ledger-123").
			WithFromAsset("USD").
			WithToAsset("EUR")

		_, err = builder5.CreateOrUpdate(context.Background())
		if err == nil {
			t.Fatal("expected error for missing rate, got nil")
		}

		if !strings.Contains(err.Error(), "rate is required") {
			t.Errorf("unexpected error message, got: %s, want to contain: rate is required", err.Error())
		}
	})
}
