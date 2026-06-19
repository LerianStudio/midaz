// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// RabbitMQ Consumer Defaults
const (
	DefaultWorkerCount   = 5
	DefaultPrefetchCount = 1
)

// FetcherNotificationRoutingSource is the "source" value stamped on
// extraction-mapping notifications the reporter emits, identifying the reporter
// as the originator.
const FetcherNotificationRoutingSource = "reporter"
