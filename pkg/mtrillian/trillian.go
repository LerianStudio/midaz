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
	Addr   string
	Conn   *grpc.ClientConn
	Logger mlog.Logger
}

// Connect keeps a singleton connection with Trillian gRPC.
func (c *TrillianConnection) Connect() error {
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

// CreateAndInitTree creates and initializes a tree for storing log entries
func (c *TrillianConnection) CreateAndInitTree(ctx context.Context, name, description string) (int64, error) {

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
	}

	logClient := trillian.NewTrillianLogClient(c.Conn)

	_, err = logClient.InitLog(ctx, &trillian.InitLogRequest{LogId: tree.TreeId})
	if err != nil {
		c.Logger.Fatalf("Error initializing tree: %v", zap.Error(err))
	}

	c.Logger.Infof("Log tree created and initialized: %v", tree.TreeId)
	return tree.TreeId, nil
}

func (c *TrillianConnection) WriteLog(ctx context.Context, treeID int64, content []byte) ([]byte, error) {
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
