package main

import (
	"encoding/json"
	"fmt"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq/transaction"
	"github.com/LerianStudio/midaz/components/audit/internal/bootstrap"
	"github.com/LerianStudio/midaz/pkg"
)

func main() {

	pkg.InitLocalEnvConfig()
	rabbit, uc, telemetry, logger := bootstrap.InitServers()

	telemetry.InitializeTelemetry(logger)
	defer telemetry.ShutdownTelemetry()

	defer func() {
		if err := logger.Sync(); err != nil {
			logger.Infof("Launcher: App (%s) error:", bootstrap.ApplicationName)
			logger.Infof("Failed to sync logger: %s", err)
			logger.Fatalf("\u001b[31m%s\u001b[0m", err)

		}
	}()

	logger.Infof("Launcher: App (%s) finished\n", bootstrap.ApplicationName)

	message, err := rabbit.Channel.Consume(
		rabbit.Queue,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		logger.Errorf("Failed to register a consumer: %s", err)
	}

	forever := make(chan bool)

	go func() {
		for d := range message {
			var transactionMessage transaction.Transaction

			err = json.Unmarshal(d.Body, &transactionMessage)
			if err != nil {
				fmt.Println("Error unmarshalling JSON:", err)
				return
			}

			uc.CreateLog(logger, transactionMessage)

			logger.Infof("message consumed: %s", transactionMessage.ID)
		}
	}()

	<-forever
}
