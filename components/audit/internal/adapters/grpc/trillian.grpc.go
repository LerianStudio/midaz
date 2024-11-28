package grpc

import (
	"context"
	"encoding/hex"
	"github.com/LerianStudio/midaz/pkg/mtrillian"
)

// Repository provides an interface for gRPC operations related to Trillian
//
//go:generate mockgen --destination=trillian.mock.go --package=out . Repository
type Repository interface {
	CreateLogTree(ctx context.Context, name, description string) (int64, error)
	CreateLog(ctx context.Context, treeID int64, operation []byte) (string, error)
}

// TrillianRepository interacts with Trillian log server
type TrillianRepository struct {
	conn *mtrillian.TrillianConnection
}

// NewTrillianRepository returns a new instance of TrillianRepository using the given gRPC connection.
func NewTrillianRepository(conn *mtrillian.TrillianConnection) *TrillianRepository {
	trillianRepo := &TrillianRepository{
		conn: conn,
	}

	_, err := conn.GetNewClient()
	if err != nil {
		panic("Failed to connect to Trillian gRPC")
	}

	return trillianRepo
}

// CreateLogTree creates a tree to store logs
func (t TrillianRepository) CreateLogTree(ctx context.Context, name, description string) (int64, error) {

	tree, err := t.conn.CreateAndInitTree(ctx, name, description)
	if err != nil {
		return 0, err
	}

	return tree, nil
}

func (t TrillianRepository) CreateLog(ctx context.Context, treeID int64, operation []byte) (string, error) {
	logHash, err := t.conn.CreateLog(ctx, treeID, operation)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(logHash), nil
}
