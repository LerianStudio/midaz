package tests

import (
	"context"
	"testing"
	"time"

	"demo-data/internal/adapters/secondary/sdk"
	"demo-data/internal/domain/entities"
	"demo-data/internal/domain/ports"
)

// TestMidazClientAdapter tests the Midaz client adapter
func TestMidazClientAdapter(t *testing.T) {
	ctx := context.Background()

	// Create test configuration
	config := &entities.Configuration{
		APIBaseURL:      "https://api.test.com",
		AuthToken:       "test-token-123456",
		TimeoutDuration: 30 * time.Second,
		Debug:           true,
		LogLevel:        "debug",
	}

	t.Run("creates client successfully", func(t *testing.T) {
		client, err := sdk.NewMidazClientAdapter(config)
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		if client == nil {
			t.Fatal("client should not be nil")
		}
	})

	t.Run("fails with nil configuration", func(t *testing.T) {
		_, err := sdk.NewMidazClientAdapter(nil)
		if err == nil {
			t.Error("should fail with nil configuration")
		}
	})

	t.Run("fails with empty auth token", func(t *testing.T) {
		invalidConfig := &entities.Configuration{
			APIBaseURL:      "https://api.test.com",
			AuthToken:       "",
			TimeoutDuration: 30 * time.Second,
		}

		_, err := sdk.NewMidazClientAdapter(invalidConfig)
		if err == nil {
			t.Error("should fail with empty auth token")
		}
	})

	t.Run("health check works", func(t *testing.T) {
		client, err := sdk.NewMidazClientAdapter(config)
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		err = client.HealthCheck(ctx)
		if err != nil {
			t.Errorf("health check should pass: %v", err)
		}
	})

	t.Run("validates auth successfully", func(t *testing.T) {
		client, err := sdk.NewMidazClientAdapter(config)
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		err = client.ValidateAuth(ctx)
		if err != nil {
			t.Errorf("auth validation should pass: %v", err)
		}
	})

	t.Run("fails auth with short token", func(t *testing.T) {
		shortTokenConfig := &entities.Configuration{
			APIBaseURL:      "https://api.test.com",
			AuthToken:       "short",
			TimeoutDuration: 30 * time.Second,
		}

		client, err := sdk.NewMidazClientAdapter(shortTokenConfig)
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		err = client.ValidateAuth(ctx)
		if err == nil {
			t.Error("auth validation should fail with short token")
		}
	})

	t.Run("validates connection successfully", func(t *testing.T) {
		client, err := sdk.NewMidazClientAdapter(config)
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		err = client.ValidateConnection(ctx)
		if err != nil {
			t.Errorf("connection validation should pass: %v", err)
		}
	})
}

// TestOrganizationOperations tests organization-related operations
func TestOrganizationOperations(t *testing.T) {
	ctx := context.Background()
	config := &entities.Configuration{
		APIBaseURL:      "https://api.test.com",
		AuthToken:       "test-token-123456",
		TimeoutDuration: 30 * time.Second,
		Debug:           true,
	}

	client, err := sdk.NewMidazClientAdapter(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	t.Run("creates organization successfully", func(t *testing.T) {
		req := &ports.OrganizationRequest{
			LegalName:       "Test Organization",
			DoingBusinessAs: "Test Org",
			LegalDocument:   "12345678000199",
			Address: ports.AddressRequest{
				Line1:   "123 Test St",
				City:    "Test City",
				State:   "TS",
				Country: "US",
				ZipCode: "12345",
			},
			Metadata: map[string]any{
				"test": "value",
			},
		}

		resp, err := client.CreateOrganization(ctx, req)
		if err != nil {
			t.Fatalf("failed to create organization: %v", err)
		}

		if resp == nil {
			t.Fatal("response should not be nil")
		}

		if resp.ID == "" {
			t.Error("organization ID should not be empty")
		}

		if resp.LegalName != req.LegalName {
			t.Errorf("expected legal name %s, got %s", req.LegalName, resp.LegalName)
		}

		if resp.Status.Code != "ACTIVE" {
			t.Errorf("expected status ACTIVE, got %s", resp.Status.Code)
		}
	})

	t.Run("fails to create organization with nil request", func(t *testing.T) {
		_, err := client.CreateOrganization(ctx, nil)
		if err == nil {
			t.Error("should fail with nil request")
		}
	})

	t.Run("fails to create organization with empty legal name", func(t *testing.T) {
		req := &ports.OrganizationRequest{
			LegalName: "",
		}

		_, err := client.CreateOrganization(ctx, req)
		if err == nil {
			t.Error("should fail with empty legal name")
		}
	})

	t.Run("lists organizations successfully", func(t *testing.T) {
		resp, err := client.ListOrganizations(ctx, 10, "")
		if err != nil {
			t.Fatalf("failed to list organizations: %v", err)
		}

		if resp == nil {
			t.Fatal("response should not be nil")
		}

		if len(resp.Items) == 0 {
			t.Error("should return at least one organization")
		}

		if resp.Pagination.Limit != 10 {
			t.Errorf("expected limit 10, got %d", resp.Pagination.Limit)
		}
	})

	t.Run("gets organization successfully", func(t *testing.T) {
		orgID := "test-org-id"

		resp, err := client.GetOrganization(ctx, orgID)
		if err != nil {
			t.Fatalf("failed to get organization: %v", err)
		}

		if resp == nil {
			t.Fatal("response should not be nil")
		}

		if resp.ID != orgID {
			t.Errorf("expected ID %s, got %s", orgID, resp.ID)
		}
	})

	t.Run("fails to get organization with empty ID", func(t *testing.T) {
		_, err := client.GetOrganization(ctx, "")
		if err == nil {
			t.Error("should fail with empty organization ID")
		}
	})
}

// TestLedgerOperations tests ledger-related operations
func TestLedgerOperations(t *testing.T) {
	ctx := context.Background()
	config := &entities.Configuration{
		APIBaseURL:      "https://api.test.com",
		AuthToken:       "test-token-123456",
		TimeoutDuration: 30 * time.Second,
		Debug:           true,
	}

	client, err := sdk.NewMidazClientAdapter(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	orgID := "test-org-id"

	t.Run("creates ledger successfully", func(t *testing.T) {
		req := &ports.LedgerRequest{
			Name: "Test Ledger",
			Metadata: map[string]any{
				"purpose": "testing",
			},
		}

		resp, err := client.CreateLedger(ctx, orgID, req)
		if err != nil {
			t.Fatalf("failed to create ledger: %v", err)
		}

		if resp == nil {
			t.Fatal("response should not be nil")
		}

		if resp.ID == "" {
			t.Error("ledger ID should not be empty")
		}

		if resp.Name != req.Name {
			t.Errorf("expected name %s, got %s", req.Name, resp.Name)
		}
	})

	t.Run("lists ledgers successfully", func(t *testing.T) {
		resp, err := client.ListLedgers(ctx, orgID)
		if err != nil {
			t.Fatalf("failed to list ledgers: %v", err)
		}

		if resp == nil {
			t.Fatal("response should not be nil")
		}

		if len(resp.Items) == 0 {
			t.Error("should return at least one ledger")
		}
	})
}

// TestAssetOperations tests asset-related operations
func TestAssetOperations(t *testing.T) {
	ctx := context.Background()
	config := &entities.Configuration{
		APIBaseURL:      "https://api.test.com",
		AuthToken:       "test-token-123456",
		TimeoutDuration: 30 * time.Second,
		Debug:           true,
	}

	client, err := sdk.NewMidazClientAdapter(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	orgID := "test-org-id"
	ledgerID := "test-ledger-id"

	t.Run("creates asset successfully", func(t *testing.T) {
		req := &ports.AssetRequest{
			Name:  "US Dollar",
			Type:  "currency",
			Code:  "USD",
			Scale: 2,
			Metadata: map[string]any{
				"country": "US",
			},
		}

		resp, err := client.CreateAsset(ctx, orgID, ledgerID, req)
		if err != nil {
			t.Fatalf("failed to create asset: %v", err)
		}

		if resp == nil {
			t.Fatal("response should not be nil")
		}

		if resp.ID == "" {
			t.Error("asset ID should not be empty")
		}

		if resp.Name != req.Name {
			t.Errorf("expected name %s, got %s", req.Name, resp.Name)
		}

		if resp.Code != req.Code {
			t.Errorf("expected code %s, got %s", req.Code, resp.Code)
		}
	})

	t.Run("lists assets successfully", func(t *testing.T) {
		resp, err := client.ListAssets(ctx, orgID, ledgerID)
		if err != nil {
			t.Fatalf("failed to list assets: %v", err)
		}

		if resp == nil {
			t.Fatal("response should not be nil")
		}

		if len(resp.Items) == 0 {
			t.Error("should return at least one asset")
		}
	})
}
