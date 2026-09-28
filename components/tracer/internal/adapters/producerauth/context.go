// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package producerauth

import "context"

type producerContextKey struct{}

// WithProducer returns a context carrying the producer a transport resolved
// from a verified credential. Only the identity middleware and interceptor
// call it; everything downstream reads it with ProducerFromContext.
func WithProducer(ctx context.Context, producer Producer) context.Context {
	return context.WithValue(ctx, producerContextKey{}, producer)
}

// ProducerFromContext returns the producer bound by WithProducer. A missing
// producer, or one without a service, reports false.
func ProducerFromContext(ctx context.Context) (Producer, bool) {
	if ctx == nil {
		return Producer{}, false
	}

	producer, ok := ctx.Value(producerContextKey{}).(Producer)
	if !ok || producer.Service == "" {
		return Producer{}, false
	}

	return producer, true
}
