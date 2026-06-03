// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

const ApplicationName = "reporter"

// HeaderXTenantID is the AMQP header name used to propagate tenant identity
// between the manager producer and worker consumer.
const HeaderXTenantID = "X-Tenant-ID"

// ModuleManager identifies the manager component (REST API) for multi-tenant context.
const ModuleManager = "reporter-manager"

// ModuleWorker identifies the worker component (RabbitMQ consumer) for multi-tenant context.
const ModuleWorker = "reporter-worker"

// DefaultPasswordPlaceholder is the placeholder value that must be replaced before production use.
const DefaultPasswordPlaceholder = "CHANGE_ME"

// RedactPlaceholder is the replacement value for masked credentials in connection strings.
const RedactPlaceholder = "REDACTED"

// MaxTagCollectionSize is the maximum number of items allowed in a collection
// to prevent resource exhaustion attacks in template tags.
const MaxTagCollectionSize = 100000
