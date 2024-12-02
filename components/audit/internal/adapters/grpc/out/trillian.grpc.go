package out

import (
	"context"
	"encoding/hex"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mtrillian"
	"github.com/google/trillian"
)

// Repository provides an interface for gRPC operations related to Trillian
//
//go:generate mockgen --destination=trillian.mock.go --package=out . Repository
type Repository interface {
	CreateTree(ctx context.Context, name, description string) (int64, error)
	CreateLog(ctx context.Context, treeID int64, operation []byte) (string, error)
	GetLogByHash(ctx context.Context, treeID int64, hash string) (*trillian.LogLeaf, error)
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

// CreateTree creates a tree to store logs
func (t TrillianRepository) CreateTree(ctx context.Context, name, description string) (int64, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.trillian.create_tree")
	defer span.End()

	ctx, spanCreate := tracer.Start(ctx, "grpc.trillian.create_tree.create")

	treeID, err := t.conn.CreateTree(ctx, name, description)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanCreate, "Failed to create tree", err)
		return 0, err
	}

	spanCreate.End()

	ctx, spanInit := tracer.Start(ctx, "grpc.trillian.create_tree.init")

	err = t.conn.InitTree(ctx, treeID)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanInit, "Failed to init tree", err)
		return 0, err
	}

	spanInit.End()

	return treeID, nil
}

// CreateLog creates a log leaf on a tree
func (t TrillianRepository) CreateLog(ctx context.Context, treeID int64, logValue []byte) (string, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.trillian.create_log")
	defer span.End()

	logHash, err := t.conn.CreateLog(ctx, treeID, logValue)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create log", err)

		return "", err
	}

	return hex.EncodeToString(logHash), nil
}

func (t TrillianRepository) GetLogByHash(ctx context.Context, treeID int64, hash string) (*trillian.LogLeaf, error) {
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "grpc.trillian.get_log_by_hash")
	defer span.End()

	proof, err := t.conn.GetInclusionProofByHash(ctx, treeID, hash)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get inclusion proof", err)

		return nil, err
	}

	leaf, err := t.conn.GetLeafByIndex(ctx, treeID, proof[0].GetLeafIndex())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get leaf by index", err)

		return nil, err
	}

	return leaf, nil
}
