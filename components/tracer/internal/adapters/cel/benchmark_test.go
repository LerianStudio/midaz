// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"testing"
)

// benchSink prevents the compiler from optimizing away benchmark results.
var benchSink any

// BenchmarkCompile benchmarks expression compilation.
func BenchmarkCompile(b *testing.B) {
	adapter := newTestAdapter(b)
	ctx := context.Background()
	expression := "amount > 1000"

	for b.Loop() {
		compiled, err := adapter.Compile(ctx, expression)
		if err != nil {
			b.Fatalf("Compile failed: %v", err)
		}

		benchSink = compiled
	}
}

// BenchmarkCompile_ComplexExpression benchmarks compilation of complex expressions.
func BenchmarkCompile_ComplexExpression(b *testing.B) {
	adapter := newTestAdapter(b)
	ctx := context.Background()
	expression := `transactionType == "PIX" && amount > 1000 && account["status"] == "active" && currency == "BRL"`

	for b.Loop() {
		compiled, err := adapter.Compile(ctx, expression)
		if err != nil {
			b.Fatalf("Compile failed: %v", err)
		}

		benchSink = compiled
	}
}

// BenchmarkEvaluate benchmarks expression evaluation.
func BenchmarkEvaluate(b *testing.B) {
	adapter := newTestAdapter(b)
	ctx := context.Background()
	expression := "amount > 1000"

	program, err := adapter.Compile(ctx, expression)
	if err != nil {
		b.Fatalf("Compile failed: %v", err)
	}

	req := newTestRequest()

	for b.Loop() {
		result, err := adapter.Evaluate(ctx, program, req)
		if err != nil {
			b.Fatalf("Evaluate failed: %v", err)
		}

		benchSink = result
	}
}

// BenchmarkEvaluate_ComplexExpression benchmarks evaluation of complex expressions.
func BenchmarkEvaluate_ComplexExpression(b *testing.B) {
	adapter := newTestAdapter(b)
	ctx := context.Background()
	expression := `transactionType == "PIX" && amount > 1000 && account["status"] == "active" && currency == "BRL"`

	program, err := adapter.Compile(ctx, expression)
	if err != nil {
		b.Fatalf("Compile failed: %v", err)
	}

	req := newTestRequest()

	for b.Loop() {
		result, err := adapter.Evaluate(ctx, program, req)
		if err != nil {
			b.Fatalf("Evaluate failed: %v", err)
		}

		benchSink = result
	}
}

// BenchmarkCompileAndEvaluate benchmarks full compile + evaluate cycle.
func BenchmarkCompileAndEvaluate(b *testing.B) {
	adapter := newTestAdapter(b)
	ctx := context.Background()
	expression := "amount > 1000"
	req := newTestRequest()

	for b.Loop() {
		program, err := adapter.Compile(ctx, expression)
		if err != nil {
			b.Fatalf("Compile failed: %v", err)
		}

		result, err := adapter.Evaluate(ctx, program, req)
		if err != nil {
			b.Fatalf("Evaluate failed: %v", err)
		}

		benchSink = result
	}
}

// BenchmarkBuildActivation benchmarks building activation from ValidationRequest.
func BenchmarkBuildActivation(b *testing.B) {
	req := newTestRequest()

	for b.Loop() {
		activation, err := BuildActivation(req)
		if err != nil {
			b.Fatalf("BuildActivation failed: %v", err)
		}

		benchSink = activation
	}
}

// BenchmarkHashExpression benchmarks expression hashing.
func BenchmarkHashExpression(b *testing.B) {
	expression := `transactionType == "PIX" && amount > 1000 && account["status"] == "active"`

	for b.Loop() {
		benchSink = HashExpression(expression)
	}
}
