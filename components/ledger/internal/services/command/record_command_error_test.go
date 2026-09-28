// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/codes"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestRecordCommandError_Business asserts the T5 business branch: the span stays green with
// a named business event and the log line lands at Warn, per docs/standards/telemetry.md.
func TestRecordCommandError_Business(t *testing.T) {
	ctx, recorder := recordingContext()
	logger := &capturingLogger{}

	_, tr, _, _ := libObservability.NewTrackingFromContext(ctx)
	_, span := tr.Start(ctx, "test.record_command_error.business")

	businessErr := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, constant.EntityLedger)

	recordCommandError(ctx, span, logger, "Failed to create ledger", businessErr)
	span.End()

	ended := findSpan(t, recorder, "test.record_command_error.business")
	assert.Equal(t, codes.Unset, ended.Status().Code)

	event, ok := findEvent(ended, "Failed to create ledger")
	assert.True(t, ok, "expected a named business event")
	assert.Contains(t, eventText(event), businessErr.Error())

	line := onlyLoggedLine(t, logger)
	assert.Equal(t, libLog.LevelWarn, line.Level)
	assert.Equal(t, "Failed to create ledger", line.Msg)
	assert.Contains(t, line.Fields, businessErr.Error())
}

// TestRecordCommandError_Technical asserts the T5 technical branch: the span flips to Error
// with an exception event and the log line lands at Error so it feeds error-rate SLOs.
func TestRecordCommandError_Technical(t *testing.T) {
	ctx, recorder := recordingContext()
	logger := &capturingLogger{}

	_, tr, _, _ := libObservability.NewTrackingFromContext(ctx)
	_, span := tr.Start(ctx, "test.record_command_error.technical")

	technicalErr := errors.New("connection reset by peer")

	recordCommandError(ctx, span, logger, "Failed to create ledger", technicalErr)
	span.End()

	ended := findSpan(t, recorder, "test.record_command_error.technical")
	assert.Equal(t, codes.Error, ended.Status().Code)

	event, ok := findEvent(ended, "exception")
	assert.True(t, ok, "expected an exception event")
	assert.Contains(t, eventText(event), technicalErr.Error())

	line := onlyLoggedLine(t, logger)
	assert.Equal(t, libLog.LevelError, line.Level)
	assert.Equal(t, "Failed to create ledger", line.Msg)
	assert.Contains(t, line.Fields, technicalErr.Error())
}

// TestRecordCommandError_ExtraFieldsPreserved asserts fields passed by the caller survive
// alongside the mandatory libLog.Err(err), in both classes.
func TestRecordCommandError_ExtraFieldsPreserved(t *testing.T) {
	extra := libLog.String("operation_route_id", "route-123")

	t.Run("business", func(t *testing.T) {
		ctx, _ := recordingContext()
		logger := &capturingLogger{}

		_, tr, _, _ := libObservability.NewTrackingFromContext(ctx)
		_, span := tr.Start(ctx, "test.record_command_error.business_fields")

		businessErr := pkg.ValidateBusinessError(constant.ErrLedgerNameConflict, constant.EntityLedger)
		recordCommandError(ctx, span, logger, "Failed to update operation route", businessErr, extra)
		span.End()

		line := onlyLoggedLine(t, logger)
		assert.Contains(t, line.Fields, "route-123")
		assert.Contains(t, line.Fields, businessErr.Error())
	})

	t.Run("technical", func(t *testing.T) {
		ctx, _ := recordingContext()
		logger := &capturingLogger{}

		_, tr, _, _ := libObservability.NewTrackingFromContext(ctx)
		_, span := tr.Start(ctx, "test.record_command_error.technical_fields")

		technicalErr := errors.New("connection reset by peer")
		recordCommandError(ctx, span, logger, "Failed to update operation route", technicalErr, extra)
		span.End()

		line := onlyLoggedLine(t, logger)
		assert.Contains(t, line.Fields, "route-123")
		assert.Contains(t, line.Fields, technicalErr.Error())
	})
}

// onlyLoggedLine fails the test unless logger captured exactly one line, then returns it.
func onlyLoggedLine(t *testing.T, logger *capturingLogger) capturedLogLine {
	t.Helper()

	logger.mu.Lock()
	defer logger.mu.Unlock()

	if len(logger.lines) != 1 {
		t.Fatalf("expected exactly one logged line, got %d: %v", len(logger.lines), logger.lines)
	}

	return logger.lines[0]
}
