package rabbitmq

import "context"

// ConsumerRepository provides an interface for Consumer related to rabbitmq.
//
//go:generate mockgen --destination=../../gen/mock/rabbitmq/consumer_repository_mock.go --package=mock . ConsumerRepository
type ConsumerRepository interface {
	Consumer(ctx context.Context, queue string, message chan string)
}
