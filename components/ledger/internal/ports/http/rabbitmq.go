package http

import (
	"context"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
)

// RabbitMQHandler struct that handle rabbitmq to use producers a consumers.
type RabbitMQHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateProducer method that create producers to rabbitmq.
func (handler *RabbitMQHandler) CreateProducer(c context.Context) {
	logger := mlog.NewLoggerFromContext(c)

	response, err := handler.Command.RabbitMQRepo.ProducerDefault("test producer transaction")
	if err != nil {
		logger.Errorf("Failed to create producer: %s", err.Error())
	}

	logger.Infof("Response to create producer: %v", response)
}

// CreateConsumer method that create consumers to rabbitmq.
func (handler *RabbitMQHandler) CreateConsumer(c context.Context) {
	message := make(chan string)

	handler.Query.RabbitMQRepo.ConsumerDefault(message)
}
