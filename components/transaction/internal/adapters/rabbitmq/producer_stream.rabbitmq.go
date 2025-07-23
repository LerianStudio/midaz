package rabbitmq

import (
	"context"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/message"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
)

type ProducerStreamRepository interface {
	SendToStream(ctx context.Context, streamName string, data []byte) error
}

type ProducerStreamRabbit struct {
	env *stream.Environment
}

func NewProducerStreamRabbit(host, user, pass string, port int) (*ProducerStreamRabbit, error) {
	env, err := stream.NewEnvironment(
		stream.NewEnvironmentOptions().
			SetHost(host).
			SetPort(port).
			SetUser(user).
			SetPassword(pass),
	)
	if err != nil {
		return nil, err
	}

	return &ProducerStreamRabbit{env: env}, nil
}

func (ps *ProducerStreamRabbit) SendToStream(ctx context.Context, streamName string, data []byte) error {
	producer, err := ps.env.NewProducer(streamName, nil)
	if err != nil {
		return err
	}
	defer producer.Close()

	var message message.StreamMessage

	message = amqp.NewMessage(data)

	return producer.Send(message)
}
