package mtrillian

import (
	"context"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/google/trillian"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"log"
	"time"
)

type TrillianConnection struct {
	Addr     string
	Database string
	Conn     *grpc.ClientConn
	Logger   mlog.Logger
}

// Connect keeps a singleton connection with Trillian gRPC.
func (c *TrillianConnection) Connect() error {

	c.Logger.Infof("Connecting to Trillian at %v", c.Addr)

	conn, err := grpc.NewClient(c.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect on Trillian gRPC: %v", zap.Error(err))
		return nil
	}

	c.Logger.Info("Connected to Trillian gRPC ✅ ")

	c.Conn = conn

	return nil
}

// GetNewClient returns a connection to Trillian gRPC, reconnect it if necessary.
func (c *TrillianConnection) GetNewClient() (*grpc.ClientConn, error) {
	if c.Conn == nil {
		if err := c.Connect(); err != nil {
			log.Printf("ERRCONNECT %v", zap.Error(err))
			return nil, err
		}
	}

	return c.Conn, nil
}

// CreateTree creates a Trillian tree to store log entries
func (c *TrillianConnection) CreateTree(ctx context.Context, name, description string) (int64, error) {
	adminClient := trillian.NewTrillianAdminClient(c.Conn)

	tree, err := adminClient.CreateTree(ctx, &trillian.CreateTreeRequest{
		Tree: &trillian.Tree{
			TreeState:       trillian.TreeState_ACTIVE,
			TreeType:        trillian.TreeType_LOG,
			DisplayName:     name[:],
			Description:     description,
			MaxRootDuration: durationpb.New(1 * time.Hour),
		},
	})
	if err != nil {
		c.Logger.Fatalf("Error while creating Tree: %v", zap.Error(err))
		return 0, err
	}

	return tree.TreeId, nil
}

// InitTree initializes a Trillian tree previously created
func (c *TrillianConnection) InitTree(ctx context.Context, treeId int64) error {
	logClient := trillian.NewTrillianLogClient(c.Conn)

	_, err := logClient.InitLog(ctx, &trillian.InitLogRequest{LogId: treeId})
	if err != nil {
		c.Logger.Fatalf("Error initializing tree: %v", zap.Error(err))
		return err
	}

	return nil
}

func (c *TrillianConnection) CreateLog(ctx context.Context, treeID int64, content []byte) ([]byte, error) {
	logClient := trillian.NewTrillianLogClient(c.Conn)

	response, err := logClient.QueueLeaf(ctx, &trillian.QueueLeafRequest{
		LogId: treeID,
		Leaf: &trillian.LogLeaf{
			LeafValue: content,
		},
	})

	if err != nil {
		log.Printf("Error sending log: %v", zap.Error(err))
	}

	return response.QueuedLeaf.Leaf.LeafIdentityHash, nil
}
