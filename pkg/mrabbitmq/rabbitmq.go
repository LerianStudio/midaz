package mrabbitmq

import (
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"

	"github.com/LerianStudio/midaz/pkg/mlog"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQConnection is a hub which deal with rabbitmq connections.
type RabbitMQConnection struct {
	ConnectionStringSource string
	Queue                  string
	Exchange               string
	Key                    string
	Host                   string
	Port                   string
	User                   string
	Pass                   string
	Channel                *amqp.Channel
	Logger                 mlog.Logger
	Connected              bool
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
