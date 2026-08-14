// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import "strings"

// TopicPrefix is the canonical prefix every streaming Kafka topic name uses.
const TopicPrefix = "lerian.streaming."

// TopicName renders the consumer-facing Kafka topic name for a producing
// service ("ledger"/"crm"/"fee"/"tracer") and a route key
// ("<resource>.<event>", the hyphenated routing handle).
//
// The streaming-hub ingest consumer subscribes via kgo.ConsumeRegex to
// ^lerian.streaming.<seg>.<seg>$ over the [a-z0-9_] charset — exactly two
// segments, no hyphen. To satisfy that grammar while still namespacing topics by
// producing service, the service is folded into the first segment
// ("<service>_<resource>") and hyphens are normalized to underscores. Only
// RouteDefinition.Key stays hyphenated (lib-streaming's route-key grammar
// accepts only [a-z0-9-]); the DefinitionKey/ResourceType/EventType and the
// CloudEvents type are the underscored canonical form. TopicName receives the
// hyphenated route key and folds it to the underscore wire form.
//
// A leading "<service>-" on the key is stripped before the fold. Fee resources
// carry a "fee-" prefix on their keys ("fee-packages.created"); without the strip
// the "fee_" service segment would double it into "lerian.streaming.fee_fee_*".
// Stripping keeps the topic at the required two segments and drops the redundant
// prefix. The strip is a no-op for services whose keys never begin with
// "<service>-" (ledger/crm/tracer), so it does not affect existing producers.
func TopicName(service, key string) string {
	key = strings.TrimPrefix(key, service+"-")
	return TopicPrefix + service + "_" + strings.ReplaceAll(key, "-", "_")
}
