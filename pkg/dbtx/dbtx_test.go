package dbtx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextWithTx_NilTx(t *testing.T) {
	ctx := context.Background()
	ctxWithTx := ContextWithTx(ctx, nil)

	tx := TxFromContext(ctxWithTx)
	assert.Nil(t, tx, "nil tx should return nil from context")
}

func TestContextWithTx_RoundTrip(t *testing.T) {
	// This test will fail until we implement the type
	ctx := context.Background()
	// Just verify the functions exist and compile
	ctxWithTx := ContextWithTx(ctx, nil)
	_ = TxFromContext(ctxWithTx)
}
