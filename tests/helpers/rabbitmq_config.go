// Package helpers provides test utilities for the Midaz test suite.
//
// SECURITY NOTICE: These helpers are designed for LOCAL TESTING ONLY
// against Docker containers. They do NOT support production-grade security
// configurations (password auth, TLS encryption, IAM). DO NOT use for production.
package helpers

import "os"

const (
	// Default RabbitMQ credentials for local testing
	// These match the defaults in components/infra/.env.example
	defaultRabbitMQUser = "midaz"
	defaultRabbitMQPass = "lerian"

	// Environment variable names
	envRabbitMQUser = "RABBITMQ_DEFAULT_USER"
	envRabbitMQPass = "RABBITMQ_DEFAULT_PASS"
)

// RabbitMQCredentials holds RabbitMQ authentication credentials
type RabbitMQCredentials struct {
	User string
	Pass string
}

// GetRabbitMQCredentials returns RabbitMQ credentials from environment variables
// Falls back to default values for local Docker testing
func GetRabbitMQCredentials() RabbitMQCredentials {
	user := os.Getenv(envRabbitMQUser)
	if user == "" {
		user = defaultRabbitMQUser
	}

	pass := os.Getenv(envRabbitMQPass)
	if pass == "" {
		pass = defaultRabbitMQPass
	}

	return RabbitMQCredentials{
		User: user,
		Pass: pass,
	}
}
