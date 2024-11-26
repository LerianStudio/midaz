package pkg

import (
	"context"
	"github.com/LerianStudio/midaz/pkg/mlog"
	gid "github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type customContextKey string

var CustomContextKey = customContextKey("custom_context")

type CustomContextKeyValue struct {
	MidazID string
	Tracer  trace.Tracer
	Logger  mlog.Logger
}

// NewLoggerFromContext extract the Logger from "logger" value inside context
//
//nolint:ireturn
func NewLoggerFromContext(ctx context.Context) mlog.Logger {
	if customContext, ok := ctx.Value(CustomContextKey).(*CustomContextKeyValue); ok &&
		customContext.Logger != nil {
		return customContext.Logger
	}

	return &mlog.NoneLogger{}
}

// ContextWithLogger returns a context within a Logger in "logger" value.
func ContextWithLogger(ctx context.Context, logger mlog.Logger) context.Context {
	values, _ := ctx.Value(CustomContextKey).(*CustomContextKeyValue)
	if values == nil {
		values = &CustomContextKeyValue{}
	}

	values.Logger = logger

	return context.WithValue(ctx, CustomContextKey, values)
}

// NewTracerFromContext returns a new tracer from the context.
//
//nolint:ireturn
func NewTracerFromContext(ctx context.Context) trace.Tracer {
	if customContext, ok := ctx.Value(CustomContextKey).(*CustomContextKeyValue); ok &&
		customContext.Tracer != nil {
		return customContext.Tracer
	}

	return otel.Tracer("default")
}

// ContextWithTracer returns a context within a trace.Tracer in "tracer" value.
func ContextWithTracer(ctx context.Context, tracer trace.Tracer) context.Context {
	values, _ := ctx.Value(CustomContextKey).(*CustomContextKeyValue)
	if values == nil {
		values = &CustomContextKeyValue{}
	}

	values.Tracer = tracer

	return context.WithValue(ctx, CustomContextKey, values)
}

// ContextWithMidazID returns a context within a MidazID in "midazID" value.
func ContextWithMidazID(ctx context.Context, midazID string) context.Context {
	values, _ := ctx.Value(CustomContextKey).(*CustomContextKeyValue)
	if values == nil {
		values = &CustomContextKeyValue{}
	}

	values.MidazID = midazID

	return context.WithValue(ctx, CustomContextKey, values)
}

// NewMidazIDFromContext returns a MidazID from the context.
func NewMidazIDFromContext(ctx context.Context) string {
	if customContext, ok := ctx.Value(CustomContextKey).(*CustomContextKeyValue); ok &&
		!IsNilOrEmpty(&customContext.MidazID) {
		return customContext.MidazID
	}

	return gid.New().String()
}
