// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// recordQueryErrorSpan runs recordQueryError inside a recorded span and returns the
// ended span, so each case can assert the status and events it left behind.
func recordQueryErrorSpan(t *testing.T, logger libLog.Logger, message string, err error) sdktrace.ReadOnlySpan {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	ctx, span := provider.Tracer("query-test").Start(context.Background(), "query.record_error")
	recordQueryError(ctx, span, logger, message, err)
	span.End()

	ended := recorder.Ended()
	require.Len(t, ended, 1)

	return ended[0]
}

func TestRecordQueryError(t *testing.T) {
	const message = "Failed to get ledger on repo by id"

	t.Run("business error records a span event and logs nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		// A strict mock with no expectations: any Log call fails the test.
		logger := libLog.NewMockLogger(ctrl)

		err := pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, constant.EntityLedger)
		require.True(t, pkg.IsBusinessError(err))

		span := recordQueryErrorSpan(t, logger, message, err)

		assert.Equal(t, codes.Unset, span.Status().Code)
		require.NotEmpty(t, span.Events())
		assert.Equal(t, message, span.Events()[0].Name)
	})

	t.Run("infrastructure error marks the span as error and logs once at Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		logger := libLog.NewMockLogger(ctrl)

		err := errors.New("connection refused")

		logger.EXPECT().Log(gomock.Any(), libLog.LevelError, message, gomock.Any()).Times(1)

		span := recordQueryErrorSpan(t, logger, message, err)

		assert.Equal(t, codes.Error, span.Status().Code)
	})

	t.Run("infrastructure error carries caller fields before the error on the log call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		logger := libLog.NewMockLogger(ctrl)

		err := errors.New("connection refused")
		entity := libLog.String("entity", "transaction")

		logger.EXPECT().
			Log(gomock.Any(), libLog.LevelError, message, []libLog.Field{entity, libLog.Err(err)}).
			Times(1)

		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

		ctx, span := provider.Tracer("query-test").Start(context.Background(), "query.record_error")
		recordQueryError(ctx, span, logger, message, err, entity)
		span.End()

		require.Len(t, recorder.Ended(), 1)
		assert.Equal(t, codes.Error, recorder.Ended()[0].Status().Code)
	})

	t.Run("nil error records nothing and logs nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		logger := libLog.NewMockLogger(ctrl)

		span := recordQueryErrorSpan(t, logger, message, nil)

		assert.Equal(t, codes.Unset, span.Status().Code)
		assert.Empty(t, span.Events())
	})
}
