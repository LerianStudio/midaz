// Package mretry provides shared retry configuration for workers and consumers.
//
// This package defines a unified Config struct that can be used across different
// retry mechanisms in the codebase, such as:
//   - MetadataOutboxWorker: processes pending metadata outbox entries
//   - DLQConsumer: processes messages from Dead Letter Queues
//
// Usage:
//
//	// Use default configuration
//	cfg := mretry.DefaultMetadataOutboxConfig()
//
//	// Or customize using builder pattern
//	cfg := mretry.DefaultDLQConfig().
//		WithMaxRetries(5).
//		WithInitialBackoff(2 * time.Second)
//
// The package provides sensible defaults that follow best practices for
// exponential backoff with jitter to prevent thundering herd problems.
package mretry
