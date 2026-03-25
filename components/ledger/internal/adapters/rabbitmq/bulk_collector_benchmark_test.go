// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// =============================================================================
// BENCHMARK TESTS - BulkCollector
// =============================================================================

// BenchmarkBulkCollector_Add benchmarks adding messages to the BulkCollector.
func BenchmarkBulkCollector_Add(b *testing.B) {
	benchmarks := []struct {
		name     string
		bulkSize int
	}{
		{"BulkSize10", 10},
		{"BulkSize50", 50},
		{"BulkSize100", 100},
		{"BulkSize500", 500},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			bc := NewBulkCollector(bm.bulkSize, 1*time.Second)
			bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
				// No-op callback for benchmark
				return nil
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start collector
			go func() {
				_ = bc.Start(ctx)
			}()

			// Wait for collector to start
			time.Sleep(10 * time.Millisecond)

			msg := amqp.Delivery{Body: []byte(`{"id":"test","data":"benchmark"}`)}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = bc.Add(msg)
			}
			b.StopTimer()

			bc.Stop()
		})
	}
}

// BenchmarkBulkCollector_Flush benchmarks the flush operation.
func BenchmarkBulkCollector_Flush(b *testing.B) {
	benchmarks := []struct {
		name         string
		messageCount int
	}{
		{"Messages10", 10},
		{"Messages50", 50},
		{"Messages100", 100},
		{"Messages500", 500},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				bc := NewBulkCollector(bm.messageCount*2, 1*time.Second)

				// Manually populate messages
				bc.mu.Lock()
				bc.messages = make([]amqp.Delivery, bm.messageCount)
				for j := 0; j < bm.messageCount; j++ {
					bc.messages[j] = amqp.Delivery{Body: []byte(`{"id":"test","data":"benchmark"}`)}
				}
				bc.mu.Unlock()

				b.StartTimer()
				_ = bc.Flush()
			}
		})
	}
}

// BenchmarkBulkCollector_ConcurrentAdd benchmarks concurrent message addition.
func BenchmarkBulkCollector_ConcurrentAdd(b *testing.B) {
	benchmarks := []struct {
		name       string
		goroutines int
	}{
		{"Goroutines1", 1},
		{"Goroutines4", 4},
		{"Goroutines8", 8},
		{"Goroutines16", 16},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			bc := NewBulkCollector(1000, 1*time.Second)
			bc.SetFlushCallback(func(ctx context.Context, messages []amqp.Delivery) error {
				return nil
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start collector
			go func() {
				_ = bc.Start(ctx)
			}()

			time.Sleep(10 * time.Millisecond)

			msg := amqp.Delivery{Body: []byte(`{"id":"test","data":"benchmark"}`)}

			b.ResetTimer()

			var wg sync.WaitGroup
			messagesPerGoroutine := b.N / bm.goroutines

			for g := 0; g < bm.goroutines; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < messagesPerGoroutine; i++ {
						_ = bc.Add(msg)
					}
				}()
			}

			wg.Wait()
			b.StopTimer()

			bc.Stop()
		})
	}
}

// =============================================================================
// BENCHMARK TESTS - Bulk vs Individual Processing
// =============================================================================

// BenchmarkProcessing_Individual simulates individual message processing overhead.
func BenchmarkProcessing_Individual(b *testing.B) {
	// Simulate individual message processing with context creation per message
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.Background()
		// Simulate minimal processing work
		_ = ctx.Value("key")
	}
}

// BenchmarkProcessing_Bulk simulates bulk message processing with shared context.
func BenchmarkProcessing_Bulk(b *testing.B) {
	benchmarks := []struct {
		name      string
		batchSize int
	}{
		{"BatchSize10", 10},
		{"BatchSize50", 50},
		{"BatchSize100", 100},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Pre-create batch of messages
			messages := make([]amqp.Delivery, bm.batchSize)
			for i := 0; i < bm.batchSize; i++ {
				messages[i] = amqp.Delivery{Body: []byte(`{"id":"test"}`)}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Create context once for the batch
				ctx := context.Background()
				// Process entire batch
				for range messages {
					_ = ctx.Value("key")
				}
			}
		})
	}
}

// =============================================================================
// BENCHMARK TESTS - BulkConfig Operations
// =============================================================================

// BenchmarkConsumerRoutes_IsBulkModeEnabled benchmarks the bulk mode check.
func BenchmarkConsumerRoutes_IsBulkModeEnabled(b *testing.B) {
	benchmarks := []struct {
		name   string
		config *BulkConfig
	}{
		{"NilConfig", nil},
		{"DisabledConfig", &BulkConfig{Enabled: false}},
		{"EnabledConfig", &BulkConfig{Enabled: true}},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			cr := &ConsumerRoutes{
				bulkConfig: bm.config,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = cr.IsBulkModeEnabled()
			}
		})
	}
}

// BenchmarkConsumerRoutes_ConfigureBulk benchmarks setting bulk configuration.
func BenchmarkConsumerRoutes_ConfigureBulk(b *testing.B) {
	cr := &ConsumerRoutes{}
	config := &BulkConfig{
		Enabled:      true,
		Size:         100,
		FlushTimeout: 100 * time.Millisecond,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr.ConfigureBulk(config)
	}
}

// =============================================================================
// BENCHMARK TESTS - Message Header Resolution
// =============================================================================

// BenchmarkResolveMessageHeaderID benchmarks header ID resolution.
func BenchmarkResolveMessageHeaderID(b *testing.B) {
	benchmarks := []struct {
		name    string
		headers amqp.Table
	}{
		{
			name:    "NilHeaders",
			headers: nil,
		},
		{
			name:    "EmptyHeaders",
			headers: amqp.Table{},
		},
		{
			name: "StringHeaderID",
			headers: amqp.Table{
				"Midaz-Id": "existing-id-12345",
			},
		},
		{
			name: "ByteSliceHeaderID",
			headers: amqp.Table{
				"Midaz-Id": []byte("existing-id-12345"),
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = resolveMessageHeaderID(bm.headers)
			}
		})
	}
}

// =============================================================================
// BENCHMARK TESTS - BulkMessageResult Operations
// =============================================================================

// BenchmarkBulkMessageResult_Creation benchmarks creating result slices.
func BenchmarkBulkMessageResult_Creation(b *testing.B) {
	benchmarks := []struct {
		name string
		size int
	}{
		{"Size10", 10},
		{"Size50", 50},
		{"Size100", 100},
		{"Size500", 500},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				results := make([]BulkMessageResult, bm.size)
				for j := 0; j < bm.size; j++ {
					results[j] = BulkMessageResult{
						Index:   j,
						Success: true,
						Error:   nil,
					}
				}
			}
		})
	}
}

// BenchmarkBulkMessageResult_AllSucceeded benchmarks checking if all results succeeded.
func BenchmarkBulkMessageResult_AllSucceeded(b *testing.B) {
	benchmarks := []struct {
		name    string
		size    int
		allPass bool
	}{
		{"Size10_AllPass", 10, true},
		{"Size100_AllPass", 100, true},
		{"Size100_OneFail", 100, false},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			results := make([]BulkMessageResult, bm.size)
			for j := 0; j < bm.size; j++ {
				results[j] = BulkMessageResult{
					Index:   j,
					Success: true,
				}
			}
			if !bm.allPass && bm.size > 0 {
				results[bm.size/2].Success = false
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				allSucceeded := true
				for _, r := range results {
					if !r.Success {
						allSucceeded = false
						break
					}
				}
				_ = allSucceeded
			}
		})
	}
}
