package services

import (
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/grpc/out"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/mongodb/audit"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementation.
type UseCase struct {
	// TrillianRepo provides an abstraction on top of Trillian gRPC.
	TrillianRepo out.Repository

	// AuditRepo provides an abstraction on top of the audit data source
	AuditRepo audit.Repository
}
