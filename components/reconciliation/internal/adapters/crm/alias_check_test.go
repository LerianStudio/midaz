package crm

import (
	"context"
	"strings"
	"testing"
)

func TestAliasChecker_existsInOnboarding_EntityAllowlist(t *testing.T) {
	t.Parallel()

	c := &AliasChecker{onboardingDB: nil, crmMongo: nil}

	_, err := c.existsInOnboarding(context.Background(), "ledger; DROP TABLE ledger;--", "any", map[string]bool{})
	if err == nil {
		t.Fatalf("expected error for disallowed entity, got nil")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected error to mention allowlist, got: %v", err)
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected error to mention blocked lookup, got: %v", err)
	}
}
