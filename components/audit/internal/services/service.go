package services

import (
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/grpc"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementation.
type UseCase struct {
	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo rabbitmq.ConsumerRepository

	// TrillianRepo provides an abstraction on top of Trillian gRPC.
	TrillianRepo grpc.Repository

	// AuditRepo provides an abstraction on top of the audit data source
	AuditRepo audit.Repository
}
