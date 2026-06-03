// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// RabbitMQ Consumer Defaults
const (
	DefaultWorkerCount   = 5
	DefaultPrefetchCount = 1
)

// RabbitMQ Exchange and Queue names for Fetcher notification consumer (Consumer 2).
// The exchange is OWNED by the Fetcher service (topic type). The Reporter binds
// its own queue to consume notifications with routing keys job.completed.reporter
// and job.failed.reporter.
const (
	FetcherJobEventsExchange         = "fetcher.job.events"
	FetcherNotificationQueue         = "reporter.fetcher.job.events"
	FetcherNotificationRoutingSource = "reporter"
	FetcherNotificationBindCompleted = "job.completed.reporter"
	FetcherNotificationBindFailed    = "job.failed.reporter"
)

// Extraction mapping statuses.
const (
	ExtractionStatusPending    = "pending"
	ExtractionStatusProcessing = "processing"
	ExtractionStatusCompleted  = "completed"
	ExtractionStatusFailed     = "failed"
)

// Fetcher notification statuses (sent by Fetcher service).
const (
	FetcherStatusCompleted = "completed"
	FetcherStatusFailed    = "failed"
)
