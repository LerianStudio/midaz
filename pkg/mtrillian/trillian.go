package mtrillian

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"log"
	"net/http"
	"time"
)

type TrillianConnection struct {
	AddrGRPC string
	AddrHTTP string
	Database string
	Conn     *grpc.ClientConn
	Logger   mlog.Logger
}

// Connect keeps a singleton connection with Trillian gRPC.
func (c *TrillianConnection) Connect() error {

	c.Logger.Infof("Connecting to Trillian at %v", c.AddrGRPC)

	if !c.healthCheck() {
		err := errors.New("can't connect to trillian")
		c.Logger.Fatalf("Trillian.HealthCheck %v", zap.Error(err))
		return err
	}

	conn, err := grpc.NewClient(c.AddrGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
		c.Logger.Errorf("Error while creating Tree: %v", zap.Error(err))
		return 0, err
	}

	return tree.TreeId, nil
}

// InitTree initializes a Trillian tree previously created
func (c *TrillianConnection) InitTree(ctx context.Context, treeId int64) error {
	logClient := trillian.NewTrillianLogClient(c.Conn)

	_, err := logClient.InitLog(ctx, &trillian.InitLogRequest{LogId: treeId})
	if err != nil {
		c.Logger.Errorf("Error initializing tree: %v", zap.Error(err))
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

func (c *TrillianConnection) GetInclusionProofByHash(ctx context.Context, treeID int64, identityHash string) ([]*trillian.Proof, error) {
	logClient := trillian.NewTrillianLogClient(c.Conn)

	leafHash, err := hex.DecodeString(identityHash)
	if err != nil {
		c.Logger.Errorf("Error decoding hash: %v", err.Error())
		return nil, err
	}

	respSLR, err := logClient.GetLatestSignedLogRoot(ctx, &trillian.GetLatestSignedLogRootRequest{LogId: treeID})
	if err != nil {
		c.Logger.Errorf("Error fetching Signed Log Root: %v", err)
	}
	signedLogRoot := respSLR.GetSignedLogRoot()

	var logRoot types.LogRootV1
	if err := logRoot.UnmarshalBinary(signedLogRoot.LogRoot); err != nil {
		c.Logger.Errorf("Failed to unmarshal LogRoot: %v", err)
	}

	proofResp, err := logClient.GetInclusionProofByHash(ctx, &trillian.GetInclusionProofByHashRequest{
		LogId:    treeID,
		LeafHash: leafHash,
		TreeSize: int64(logRoot.TreeSize),
	})
	if err != nil {
		c.Logger.Errorf("Error getting inclusion proof: %v", zap.Error(err))
		return nil, err
	}

	if len(proofResp.Proof) == 0 {
		c.Logger.Errorf("Inclusion proof is empty: %v", zap.Error(err))
		return nil, err
	}

	return proofResp.GetProof(), nil
}

func (c *TrillianConnection) GetLeafByIndex(ctx context.Context, treeID int64, leafIndex int64) (*trillian.LogLeaf, error) {
	logClient := trillian.NewTrillianLogClient(c.Conn)

	leaves, err := logClient.GetLeavesByRange(ctx, &trillian.GetLeavesByRangeRequest{
		LogId:      treeID,
		StartIndex: leafIndex,
		Count:      1,
	})
	if err != nil {
		return nil, err
	}

	if len(leaves.GetLeaves()) == 0 {
		c.Logger.Errorf("No leaves found: %v", zap.Error(err))
		return nil, err
	}

	return leaves.GetLeaves()[0], nil
}

func (c *TrillianConnection) healthCheck() bool {
	resp, err := http.Get(c.AddrHTTP + "/healthz")

	if err != nil {
		c.Logger.Errorf("failed to make GET request: %v", err.Error())
		return false
	}

	if resp.StatusCode == http.StatusOK {
		c.Logger.Info("Trillian health check passed")
		return true
	}

	c.Logger.Error("Trillian unhealthy...")

	return false
}
