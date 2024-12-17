package pkg

import (
	"context"
	"testing"
)

func TestNewLoggerFromContext(t *testing.T) {
	t.Log(NewLoggerFromContext(context.Background()))
}

func TestContextWithLogger(t *testing.T) {
	t.Log(ContextWithLogger(context.Background(), nil))
}

func TestNewTracerFromContext(t *testing.T) {
	t.Log(NewTracerFromContext(context.Background()))
}

func TestContextWithTracer(t *testing.T) {
	t.Log(ContextWithTracer(context.Background(), nil))
}

func TestContextWithMidazID(t *testing.T) {
	t.Log(ContextWithMidazID(context.Background(), ""))
}

func TestNewMidazIDFromContext(t *testing.T) {
	t.Log(NewMidazIDFromContext(context.Background()))
}
