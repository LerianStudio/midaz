package http

import (
	"context"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
)

// RabbitMQHandler struct that handle rabbitmq to use producers ans consumers.
type RabbitMQHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateProducer method that create producers to rabbitmq.
func (handler *RabbitMQHandler) CreateProducer(c context.Context) {
	logger := mlog.NewLoggerFromContext(c)

	response, err := handler.Command.RabbitMQRepo.Producer(c, "", "", "")
	if err != nil {
		logger.Errorf("Failed to create producer: %s", err.Error())
	}

	logger.Infof("Response to create producer: %v", response)

}

// CreateConsumer method that create consumers to rabbitmq.
func (handler *RabbitMQHandler) CreateConsumer(c context.Context) {
	logger := mlog.NewLoggerFromContext(c)

	message := make(chan string)

	handler.Query.RabbitMQRepo.Consumer(c, "", message)

	logger.Infof("Response message to consumer: %v", message)
}
