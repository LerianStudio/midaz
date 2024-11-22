package rabbitmq

// ProducerRepository provides an interface for Producer related to rabbitmq.
//
//go:generate mockgen --destination=../../mock/rabbitmq/producer_repository_mock.go --package=rabbitmq . ProducerRepository
type ProducerRepository interface {
	ProducerDefault(message string) (*string, error)
}
