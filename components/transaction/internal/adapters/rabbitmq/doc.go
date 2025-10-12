// Package rabbitmq provides RabbitMQ producer implementation for the transaction service.
//
// This package publishes transaction lifecycle events:
// - Transaction status changes (APPROVED, PENDING, CANCELED, NOTED)
// - Audit trail events for compliance logging
// - Integration events for downstream systems
// - Health checking for circuit breaking
//
// Enables event-driven integrations with external systems, analytics, and webhooks.
package rabbitmq
