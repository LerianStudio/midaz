package pkg

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func HandleSpanBusinessErrorEvent(span *trace.Span, eventName string, err error) {
	(*span).AddEvent(eventName, trace.WithAttributes(attribute.String("error", err.Error())))
}

func HandleSpanEvent(span *trace.Span, eventName string, attributes ...attribute.KeyValue) {
	(*span).AddEvent(eventName, trace.WithAttributes(attributes...))
}
