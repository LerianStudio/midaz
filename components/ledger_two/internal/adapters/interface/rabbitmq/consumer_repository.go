package rabbitmq

// ConsumerRepository provides an interface for Consumer related to rabbitmq.
//
//go:generate mockgen --destination=../../mock/rabbitmq/consumer_repository_mock.go --package=rabbitmq . ConsumerRepository
type ConsumerRepository interface {
	ConsumerDefault(message chan string)
}
