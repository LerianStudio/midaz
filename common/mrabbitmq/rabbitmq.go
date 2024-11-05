package mrabbitmq

import (
	"context"
	"errors"
	"log"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/streadway/amqp"
	"go.uber.org/zap"
)

// RabbitMQConnection is a hub which deal with rabbitmq connections.
type RabbitMQConnection struct {
	ConnectionStringSource string
	Consumer               string
	Producer               string
	Channel                amqp.Channel
	Connected              bool
	Logger                 mlog.Logger
}

// Connect keeps a singleton connection with rabbitmq.
func (rc *RabbitMQConnection) Connect(ctx context.Context) error {
	rc.Logger.Info("Connecting on rabbitmq...")

	conn, err := amqp.Dial(rc.ConnectionStringSource)
	if err != nil {
		rc.Logger.Fatal("failed to connect on rabbitmq", zap.Error(err))
		return nil
	}

	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		rc.Logger.Fatal("failed to open channel on rabbitmq", zap.Error(err))
		return nil
	}

	defer ch.Close()

	if ch == nil || !rc.healthCheck() {
		rc.Connected = false
		err := errors.New("can't connect rabbitmq")
		rc.Logger.Fatalf("RabbitMQ.HealthCheck %v", zap.Error(err))

		return err
	}

	rc.Logger.Info("Connected on rabbitmq ✅ \n")

	rc.Connected = true

	rc.Channel = *ch

	return nil
}

// GetChannel returns a pointer to the rabbitmq connection, initializing it if necessary.
func (rc *RabbitMQConnection) GetChannel(ctx context.Context) (*amqp.Channel, error) {
	if !rc.Connected {
		err := rc.Connect(ctx)
		if err != nil {
			rc.Logger.Infof("ERRCONECT %s", err)

			return nil, err
		}
	}

	return &rc.Channel, nil
}

// healthCheck rabbitmq when server is started
func (rc *RabbitMQConnection) healthCheck() bool {
	_, err := rc.Channel.QueueDeclarePassive(
		"health_check_queue",
		true,
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		log.Println("Erro ao verificar a fila no RabbitMQ:", err)
		return false
	}

	rc.Logger.Error("rabbitmq unhealthy...")

	return false
}
