package services

import (
	"context"
	"encoding/hex"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/grpc/out"
	"github.com/google/trillian"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"strings"
	"testing"
)

func Test_GetLogByHash(t *testing.T) {
	treeID := int64(9080682816463212189)
	identityHash := "18240592D5594FB370E73AC81DD5C8E5EC5AB07D5874F40154A8D54BDC390B3C"

	mockedLogLeaf := &trillian.LogLeaf{
		MerkleLeafHash: []byte(identityHash),
		LeafValue:      []byte("Log A"),
	}

	uc := UseCase{
		TrillianRepo: out.NewMockRepository(gomock.NewController(t)),
	}

	uc.TrillianRepo.(*out.MockRepository).
		EXPECT().
		GetLogByHash(gomock.Any(), treeID, identityHash).
		Return(mockedLogLeaf, nil).
		Times(1)

	merkleLeafHash, logValue, err := uc.GetLogByHash(context.TODO(), treeID, identityHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert.Equal(t, strings.ToUpper(hex.EncodeToString(mockedLogLeaf.MerkleLeafHash)), merkleLeafHash)
	assert.Equal(t, []byte("Log A"), logValue)
}
