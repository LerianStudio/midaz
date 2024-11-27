package out

import (
	"context"
	"github.com/LerianStudio/midaz/pkg/mtrillian"
)

// Repository provides an interface for gRPC operations related to Trillian
//
//go:generate mockgen --destination=trillian.mock.go --package=out . Repository
type Repository interface {
	CreateLogTree(ctx context.Context, name, description string) (int64, error)
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
