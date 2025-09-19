package rabbitmq

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/LerianStudio/lib-commons/commons/log"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// RabbitMQConnection is a hub which deal with rabbitmq connections.
type RabbitMQConnection struct {
	ConnectionStringSource string
	Queue                  string
	Host                   string
	Port                   string
	User                   string
	Pass                   string
	Channel                *amqp.Channel
	Logger                 log.Logger
	Connected              bool
	Connection             *amqp.Connection
}

// Connect keeps a singleton connection with rabbitmq.
func (rc *RabbitMQConnection) Connect() error {
	rc.Logger.Info("Connecting on rabbitmq...")

	conn, err := amqp.Dial(rc.ConnectionStringSource)
	if err != nil {
		rc.Logger.Fatal("failed to connect on rabbitmq", zap.Error(err))

		return err
	}

	ch, err := conn.Channel()
	if err != nil {
		rc.Logger.Fatal("failed to open channel on rabbitmq", zap.Error(err))

		return err
	}

	if ch == nil || !rc.healthCheck() {
		rc.Connected = false
		err = errors.New("can't connect rabbitmq")
		rc.Logger.Fatalf("RabbitMQ.HealthCheck: %v", zap.Error(err))

		return err
	}

	rc.Logger.Info("Connected on rabbitmq ✅ \n")

	rc.Connected = true

	rc.Channel = ch

	return nil
}

// GetNewConnect returns a pointer to the rabbitmq connection, initializing it if necessary.
func (rc *RabbitMQConnection) GetNewConnect() (*amqp.Channel, error) {
	if !rc.Connected {
		err := rc.Connect()
		if err != nil {
			rc.Logger.Infof("ERRCONECT %s", err)

			return nil, err
		}
	}

	return rc.Channel, nil
}

// EnsureChannel ensures that the channel is open and connected.
func (rc *RabbitMQConnection) EnsureChannel() error {
	if rc.Connection == nil || rc.Connection.IsClosed() {
		conn, err := amqp.Dial(rc.ConnectionStringSource)
		if err != nil {
			rc.Logger.Errorf("can't connect to rabbitmq: %v", err)

			return err
		}

		rc.Connection = conn
	}

	if rc.Channel == nil || rc.Channel.IsClosed() {
		ch, err := rc.Connection.Channel()
		if err != nil {
			rc.Logger.Errorf("can't open channel on rabbitmq: %v", err)

			return err
		}

		rc.Channel = ch
	}

	rc.Connected = true

	return nil
}

// healthCheck rabbitmq when server is started
func (rc *RabbitMQConnection) healthCheck() bool {
	url := fmt.Sprintf("http://%s:%s/api/health/checks/alarms", rc.Host, rc.Port)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		rc.Logger.Errorf("failed to make GET request before client do: %v", err.Error())

		return false
	}

	req.SetBasicAuth(rc.User, rc.Pass)

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		rc.Logger.Errorf("failed to make GET request after client do: %v", err.Error())

		return false
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		rc.Logger.Errorf("failed to read response body: %v", err.Error())

		return false
	}

	var result map[string]any

	err = json.Unmarshal(body, &result)
	if err != nil {
		rc.Logger.Errorf("failed to unmarshal response: %v", err.Error())

		return false
	}

	if status, ok := result["status"].(string); ok && status == "ok" {
		return true
	}

	rc.Logger.Error("rabbitmq unhealthy...")

	return false
}
