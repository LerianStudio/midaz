package rabbitmq

import (
	"context"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
//
//go:generate mockgen --destination=../../gen/mock/rabbitmq/producer_repository_mock.go --package=mock . ProducerRepository
type ProducerRepository interface {
	Producer(ctx context.Context, exchange, key, body string) (*string, error)
}
