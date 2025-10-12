// Package bootstrap handles dependency injection and service initialization
// for the transaction service. It configures multiple consumers (HTTP,
// RabbitMQ, Redis), manages database connections, and orchestrates the
// complex startup sequence for synchronous and asynchronous transaction
// processing with proper lifecycle management.
package bootstrap
