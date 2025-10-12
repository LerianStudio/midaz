// Package main is the entrypoint for the Midaz transaction service binary.
// It initializes environment configuration and starts multiple consumers
// (HTTP server, RabbitMQ consumer, Redis queue processor) that handle
// transaction processing, balance updates, and ledger operations.
package main
