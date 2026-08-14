// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

// TopicName renders the consumer-facing Kafka topic name for a producing
// service ("ledger"/"crm") and a definition key ("<resource>.<event>").
//
// The grammar is {service}.{resource}.{event}: the service is the first
// segment, and the underscore-canonical DefinitionKey supplies the remaining
// "<resource>.<event>" verbatim. Definition keys are already underscore-canonical
// (see events.Definition), so no normalization happens here — the route
// Key/ce-type keep their hyphens on the wire identity, while the topic name uses
// the underscore key as-is.
func TopicName(service, key string) string {
	return service + "." + key
}
