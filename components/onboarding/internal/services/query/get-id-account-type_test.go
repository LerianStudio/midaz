package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetAccountTypeByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountTypeByID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetAccountTypeByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountTypeByID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetAccountTypeByID_NilAccountTypeID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil accountTypeID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "accountTypeID must not be nil UUID"),
			"panic message should mention accountTypeID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountTypeByID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
