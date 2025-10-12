// Package rabbitmq provides RabbitMQ producer implementation for the onboarding service.
//
// This package publishes account creation events to the transaction service:
// - Account creation events for balance initialization
// - Health checking for circuit breaking
// - Default exchange/routing key configuration
// - Enables event-driven architecture between services
package rabbitmq
