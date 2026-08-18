// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import "strings"

// sanitizeServiceSegment reduces a producing-service name to the leading,
// ACL-scoped segment of a streaming topic name. The rule is: lowercase the
// input and keep ONLY [a-z0-9], dropping every other rune.
//
// This MUST stay byte-compatible with the tenant-manager's Kafka-segment
// sanitizer — the canonical ACL-prefix boundary, the same rule lib-commons
// exposes as secretsmanager.SanitizeKafkaSegment. A Kafka ACL granted on the
// prefix "{sanitize(service)}." only matches every topic the service emits if
// this function produces the identical segment the tenant-manager granted.
//
// It is a deliberate local copy rather than a call into
// secretsmanager.SanitizeKafkaSegment: that lib-commons package pulls in the
// AWS Secrets Manager SDK, an unacceptable transitive dependency to force onto
// this leaf streaming helper. The rule is six lines and stable, so it is
// duplicated here and kept byte-for-byte identical instead.
//
// The rule also converges with lib-streaming's EventDefinition.Topic derivation
// for the plain service names midaz uses ("ledger", "tracer"), which contain no
// runes outside [a-z0-9] and therefore pass through both sanitizers unchanged.
func sanitizeServiceSegment(s string) string {
	lowered := strings.ToLower(s)

	var b strings.Builder
	b.Grow(len(lowered))

	for _, r := range lowered {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}

	return b.String()
}

// TopicName renders the consumer-facing Kafka topic name for a producing
// service ("ledger"/"tracer") and the underscore-canonical event key
// ("<resource>.<event>", i.e. Definition.Key()).
//
// The grammar is "{sanitize(service)}.{resource}.{event}": the sanitized
// service name is its own leading segment, followed verbatim by the canonical
// key. The service segment being a standalone prefix lets a Kafka ACL scoped to
// "{service}." cover every topic that service emits, and the result converges
// with lib-streaming's EventDefinition.Topic(source) for the same service name.
//
// The key is already underscore-canonical (Definition.Key()), so no
// hyphen-to-underscore folding is performed here.
func TopicName(service, key string) string {
	return sanitizeServiceSegment(service) + "." + key
}
