package rabbitmq

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
)

type StreamHandlerFunc func(ctx context.Context, data []byte) error

type ConsumerStreamRabbit struct {
	env      *stream.Environment
	handler  StreamHandlerFunc
	consumer *stream.Consumer
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewConsumerStreamRabbit(host, user, pass string, port int) (*ConsumerStreamRabbit, error) {
	env, err := stream.NewEnvironment(
		stream.NewEnvironmentOptions().
			SetHost(host).
			SetPort(port).
			SetUser(user).
			SetPassword(pass).
			SetAddressResolver(stream.AddressResolver{
				Host: host,
				Port: port,
			}),
	)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ConsumerStreamRabbit{
		env:    env,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (csr *ConsumerStreamRabbit) SetHandler(handler StreamHandlerFunc) {
	csr.handler = handler
}

func (csr *ConsumerStreamRabbit) RunConsumer(streamName string) error {
	if csr.handler == nil {
		return fmt.Errorf("handler not set - call SetHandler() before RunConsumer()")
	}

	// Create message handler that wraps the user handler
	messageHandler := func(consumerContext stream.ConsumerContext, message *amqp.Message) {
		if message.Data != nil && len(message.Data) > 0 {
			err := csr.handler(csr.ctx, message.Data[0])
			if err != nil {
				log.Printf("Error processing message: %v", err)
			}
		}
	}

	consumer, err := csr.env.NewConsumer(
		streamName,
		messageHandler,
		stream.NewConsumerOptions().
			SetOffset(stream.OffsetSpecification{}.First()).
			SetConsumerName("midaz-stream-consumer"),
	)
	if err != nil {
		return err
	}

	csr.consumer = consumer
	log.Println("Consumer de stream iniciado para:", streamName)

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("Received shutdown signal, closing consumer...")
		return csr.Close()
	case <-csr.ctx.Done():
		log.Println("Context cancelled, closing consumer...")
		return csr.Close()
	}
}

func (csr *ConsumerStreamRabbit) Close() error {
	if csr.cancel != nil {
		csr.cancel()
	}

	if csr.consumer != nil {
		if err := csr.consumer.Close(); err != nil {
			log.Printf("Error closing consumer: %v", err)
		}
	}

	if csr.env != nil {
		// Give some time for graceful shutdown
		time.Sleep(100 * time.Millisecond)
		if err := csr.env.Close(); err != nil {
			log.Printf("Error closing environment: %v", err)
			return err
		}
	}

	return nil
}
