// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import "strings"

// TopicPrefix is the canonical prefix every streaming Kafka topic name uses.
const TopicPrefix = "lerian.streaming."

// TopicName renders the consumer-facing Kafka topic name for a producing
// service ("ledger"/"crm") and a definition key ("<resource>.<event>").
//
// The streaming-hub ingest consumer subscribes via kgo.ConsumeRegex to
// ^lerian.streaming.<seg>.<seg>$ over the [a-z0-9_] charset — exactly two
// segments, no hyphen. To satisfy that grammar while still namespacing topics by
// producing service, the service is folded into the first segment
// ("<service>_<resource>") and hyphens are normalized to underscores. The route
// Key/DefinitionKey/ResourceType/EventType and the CloudEvents type keep their
// hyphens: lib-streaming's route-key grammar requires hyphens and rejects "_",
// so the underscore form lives ONLY on the wire topic name, not on the event
// identity.
func TopicName(service, key string) string {
	return TopicPrefix + service + "_" + strings.ReplaceAll(key, "-", "_")
}
