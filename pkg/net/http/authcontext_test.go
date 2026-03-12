package http

import (
	"context"
	"testing"

	v3commons "github.com/LerianStudio/lib-commons/v3/commons"
	v4commons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
)

func TestBridgeLibAuthContext_PreservesHeaderIDAndTracer(t *testing.T) {
	t.Parallel()

	tracer := otel.Tracer("midaz-test")
	ctx := v4commons.ContextWithHeaderID(context.Background(), "req-123")
	ctx = v4commons.ContextWithTracer(ctx, tracer)

	bridgedCtx := bridgeLibAuthContext(ctx)

	_, bridgedTracer, requestID, _ := v3commons.NewTrackingFromContext(bridgedCtx)
	assert.Equal(t, "req-123", requestID)
	assert.Equal(t, tracer, bridgedTracer)

	_, originalTracer, originalRequestID, _ := v4commons.NewTrackingFromContext(bridgedCtx)
	assert.Equal(t, "req-123", originalRequestID)
	assert.Equal(t, tracer, originalTracer)
}
