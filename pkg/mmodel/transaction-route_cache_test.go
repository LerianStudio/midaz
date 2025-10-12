package mmodel

import (
	"testing"

	"github.com/google/uuid"
)

func TestTransactionRoute_ToCache(t *testing.T) {
	srcID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	dstID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174001")

	tr := &TransactionRoute{
		OperationRoutes: []OperationRoute{
			{
				ID:            srcID,
				OperationType: "source",
				Account:       &AccountRule{RuleType: "alias", ValidIf: "@cash"},
			},
			{
				ID:            dstID,
				OperationType: "destination",
				Account:       &AccountRule{RuleType: "account_type", ValidIf: []string{"deposit"}},
			},
		},
	}

	cache := tr.ToCache()
	if len(cache.Source) != 1 || len(cache.Destination) != 1 {
		t.Fatalf("expected 1 source and 1 destination, got %d and %d", len(cache.Source), len(cache.Destination))
	}

	if _, ok := cache.Source[srcID.String()]; !ok {
		t.Fatalf("missing source route %s", srcID.String())
	}
	if _, ok := cache.Destination[dstID.String()]; !ok {
		t.Fatalf("missing destination route %s", dstID.String())
	}
}

func TestTransactionRouteCache_Msgpack_Roundtrip(t *testing.T) {
	cache := TransactionRouteCache{
		Source: map[string]OperationRouteCache{
			"a": {Account: &AccountCache{RuleType: "alias", ValidIf: "@x"}},
		},
		Destination: map[string]OperationRouteCache{
			"b": {Account: &AccountCache{RuleType: "account_type", ValidIf: []string{"y"}}},
		},
	}

	b, err := cache.ToMsgpack()
	if err != nil {
		t.Fatalf("ToMsgpack error: %v", err)
	}

	var decoded TransactionRouteCache
	if err := decoded.FromMsgpack(b); err != nil {
		t.Fatalf("FromMsgpack error: %v", err)
	}

	if len(decoded.Source) != 1 || len(decoded.Destination) != 1 {
		t.Fatalf("roundtrip mismatch: got %v", decoded)
	}
}

// no extra helpers needed
