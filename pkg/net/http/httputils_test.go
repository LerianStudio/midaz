package http

import (
	"net/http/httptest"
	"strconv"
	"testing"

	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/gofiber/fiber/v2"
)

func TestValidateParameters_Invalids(t *testing.T) {
	// invalid sort_order
	_, err := ValidateParameters(map[string]string{"sort_order": "sideways"})
	if err == nil {
		t.Fatalf("expected error for invalid sort order")
	}

	// invalid cursor
	_, err = ValidateParameters(map[string]string{"cursor": "not-a-cursor"})
	if err == nil {
		t.Fatalf("expected error for invalid cursor")
	}

	// invalid date range (only one date)
	_, err = ValidateParameters(map[string]string{"start_date": "2025-01-01"})
	if err == nil {
		t.Fatalf("expected error for invalid date range")
	}

	// invalid date format
	_, err = ValidateParameters(map[string]string{"start_date": "2025-99-99", "end_date": "2025-99-99"})
	if err == nil {
		t.Fatalf("expected error for invalid date format")
	}
}

func TestGetIdempotencyKeyAndTTL_Defaults(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/", func(c *fiber.Ctx) error {
		key, ttl := GetIdempotencyKeyAndTTL(c)
		c.Set("X-Key", key)
		c.Set("X-TTL", strconv.Itoa(int(ttl)))
		return c.SendStatus(200)
	})

	// No headers
	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.Header.Get("X-Key") != "" {
		t.Fatalf("expected empty idempotency key, got %q", resp.Header.Get("X-Key"))
	}
	if resp.Header.Get("X-TTL") == "0" || resp.Header.Get("X-TTL") == "" {
		t.Fatalf("expected positive ttl, got %q", resp.Header.Get("X-TTL"))
	}

	// With headers
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set(libConstants.IdempotencyKey, "ik")
	req2.Header.Set(libConstants.IdempotencyTTL, "30")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp2.Header.Get("X-Key") != "ik" {
		t.Fatalf("expected key 'ik', got %q", resp2.Header.Get("X-Key"))
	}
	if resp2.Header.Get("X-TTL") == "0" || resp2.Header.Get("X-TTL") == "" {
		t.Fatalf("expected ttl > 0, got %q", resp2.Header.Get("X-TTL"))
	}
}

func TestFindUnknownFields(t *testing.T) {
	original := map[string]any{
		"known": "v",
		"extra": 1,
		"nested": map[string]any{
			"keep":   "ok",
			"extra2": "x",
		},
	}
	marshaled := map[string]any{
		"known": "v",
		"nested": map[string]any{
			"keep": "ok",
		},
	}

	diff := FindUnknownFields(original, marshaled)
	if _, ok := diff["extra"]; !ok {
		t.Fatalf("expected to find unknown 'extra'")
	}
	nested, ok := diff["nested"].(map[string]any)
	if !ok || nested["extra2"] == nil {
		t.Fatalf("expected to find nested unknown 'extra2'")
	}
}
