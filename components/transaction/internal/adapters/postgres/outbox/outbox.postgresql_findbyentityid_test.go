package outbox

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboxPostgreSQLRepository_FindByEntityID_EmptyEntityID_ReturnsValidationError(t *testing.T) {
	r := &OutboxPostgreSQLRepository{}

	assert.NotPanics(t, func() {
		entry, err := r.FindByEntityID(context.Background(), "", EntityTypeTransaction)
		require.Nil(t, entry)
		require.Error(t, err)

		var vErr pkg.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Equal(t, constant.ErrBadRequest.Error(), vErr.Code)
		assert.Contains(t, vErr.Message, "entityID")
	})
}

func TestOutboxPostgreSQLRepository_FindByEntityID_EmptyEntityType_ReturnsValidationError(t *testing.T) {
	r := &OutboxPostgreSQLRepository{}

	assert.NotPanics(t, func() {
		entry, err := r.FindByEntityID(context.Background(), "some-id", "")
		require.Nil(t, entry)
		require.Error(t, err)

		var vErr pkg.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Equal(t, constant.ErrBadRequest.Error(), vErr.Code)
		assert.Contains(t, vErr.Message, "entityType")
	})
}

func TestOutboxPostgreSQLRepository_FindByEntityID_WhitespaceOnlyInputs_ReturnsValidationError(t *testing.T) {
	r := &OutboxPostgreSQLRepository{}

	assert.NotPanics(t, func() {
		entry, err := r.FindByEntityID(context.Background(), "   ", "\t")
		require.Nil(t, entry)
		require.Error(t, err)

		var vErr pkg.ValidationError
		require.True(t, errors.As(err, &vErr))
		assert.Equal(t, constant.ErrBadRequest.Error(), vErr.Code)
	})
}
