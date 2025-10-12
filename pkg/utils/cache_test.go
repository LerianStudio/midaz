package utils

import (
	"testing"

	"github.com/google/uuid"
)

func TestGenericInternalKey(t *testing.T) {
	got := GenericInternalKey("transaction", "transactions", "org123", "led456", "key789")
	want := "transaction:{transactions}:org123:led456:key789"
	if got != want {
		t.Fatalf("unexpected key. want=%q got=%q", want, got)
	}
}

func TestTransactionInternalKey(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	got := TransactionInternalKey(orgID, ledgerID, "k")
	want := "transaction:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":k"
	if got != want {
		t.Fatalf("unexpected transaction key. want=%q got=%q", want, got)
	}
}

func TestBalanceInternalKey(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()

	got := BalanceInternalKey(orgID, ledgerID, "bal")
	want := "balance:{transactions}:" + orgID.String() + ":" + ledgerID.String() + ":bal"
	if got != want {
		t.Fatalf("unexpected balance key. want=%q got=%q", want, got)
	}
}
