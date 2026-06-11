// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"
)

// recordingContext wires an in-memory SpanRecorder into the lib-observability
// tracking context so service methods record onto inspectable SDK spans.
func recordingContext() (context.Context, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx := libObservability.ContextWithTracer(context.Background(), tp.Tracer("crm-test"))

	return ctx, recorder
}

// findSpan returns the first ended span whose name matches, failing the test if absent.
func findSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, s := range recorder.Ended() {
		if s.Name() == name {
			return s
		}
	}

	t.Fatalf("span %q was not recorded; recorded spans: %v", name, spanNames(recorder))

	return nil
}

func spanNames(recorder *tracetest.SpanRecorder) []string {
	names := make([]string, 0, len(recorder.Ended()))
	for _, s := range recorder.Ended() {
		names = append(names, s.Name())
	}

	return names
}

func hasEvent(s sdktrace.ReadOnlySpan, eventName string) bool {
	for _, e := range s.Events() {
		if e.Name == eventName {
			return true
		}
	}

	return false
}

// TestSpanStatus_BusinessFailureLeavesSpanUnset is Epic 5.4's done-when for the
// business-error class: a CRM validation failure records a business EVENT and
// leaves the span status UNSET (it must NOT flip the span red, so it does not
// inflate error-rate SLOs). This is the T5 contract for the business path.
func TestSpanStatus_BusinessFailureLeavesSpanUnset(t *testing.T) {
	ctx, recorder := recordingContext()

	uc := &UseCase{}

	// A nil related party is a validation failure routed through
	// HandleSpanBusinessErrorEvent inside ValidateRelatedParty.
	err := uc.ValidateRelatedParty(ctx, nil)
	require.Error(t, err, "nil related party must be a business validation error")

	span := findSpan(t, recorder, "service.validate_related_party")

	assert.Equal(t, codes.Unset, span.Status().Code,
		"a business/validation failure must leave the span status UNSET")
	assert.True(t, hasEvent(span, "Related party payload is nil"),
		"the business failure must be recorded as a span event")
}

// TestSpanStatus_InfraFailureSetsSpanError is Epic 5.4's done-when for the
// technical-error class: a non-business (infrastructure) repository failure
// flips the span status to Error so it feeds error-rate SLOs. This is the T5
// contract for the technical path, routed through recordSpanError ->
// HandleSpanError.
func TestSpanStatus_InfraFailureSetsSpanError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, recorder := recordingContext()

	mockRepo := holder.NewMockRepository(ctrl)
	// A plain (non-typed-business) error models an infrastructure failure.
	mockRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection reset by peer"))

	uc := &UseCase{HolderRepo: mockRepo}

	_, err := uc.CreateHolder(ctx, "org-1", &mmodel.CreateHolderInput{
		Name:     "John Smith",
		Document: "90217469051",
	})
	require.Error(t, err, "an infra repository failure must propagate as an error")

	span := findSpan(t, recorder, "service.create_holder")

	assert.Equal(t, codes.Error, span.Status().Code,
		"an infrastructure failure must set the span status to Error")
}
