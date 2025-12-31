package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCreateOrCheckIdempotencyKey_PanicsOnNilOrganizationID(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	assert.Panics(t, func() {
		_, _ = uc.CreateOrCheckIdempotencyKey(ctx, uuid.Nil, uuid.New(), "key", "hash", time.Minute)
	}, "Expected panic on nil OrganizationID")
}

func TestCreateOrCheckIdempotencyKey_PanicsOnNilLedgerID(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	assert.Panics(t, func() {
		_, _ = uc.CreateOrCheckIdempotencyKey(ctx, uuid.New(), uuid.Nil, "key", "hash", time.Minute)
	}, "Expected panic on nil LedgerID")
}
