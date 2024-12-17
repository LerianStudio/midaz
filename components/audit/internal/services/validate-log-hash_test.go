package services

import (
	"context"
	"encoding/hex"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/grpc/out"
	"github.com/google/trillian"
	"github.com/stretchr/testify/assert"
	"github.com/transparency-dev/merkle/rfc6962"
	"go.uber.org/mock/gomock"
	"strings"
	"testing"
)

func Test_ValidatedLogHash(t *testing.T) {
	treeID := int64(9080682816463212189)
	logValue := "Log A"
	identityHash := rfc6962.DefaultHasher.HashLeaf([]byte(logValue))
	identityHashString := strings.ToUpper(hex.EncodeToString(identityHash))

	mockedLogLeaf := &trillian.LogLeaf{
		MerkleLeafHash: identityHash,
		LeafValue:      []byte(logValue),
	}

	uc := UseCase{
		TrillianRepo: out.NewMockRepository(gomock.NewController(t)),
	}

	uc.TrillianRepo.(*out.MockRepository).
		EXPECT().
		GetLogByHash(gomock.Any(), treeID, identityHashString).
		Return(mockedLogLeaf, nil).
		Times(1)

	merkleLeafHash, recalculatedHash, isTampered, err := uc.ValidatedLogHash(context.TODO(), treeID, identityHashString)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert.Equal(t, identityHashString, merkleLeafHash)
	assert.Equal(t, identityHashString, recalculatedHash)
	assert.Equal(t, false, isTampered)
}

func Test_ValidatedLogHash_Tampered(t *testing.T) {
	treeID := int64(9080682816463212189)
	logValue := "Log A"
	identityHash := rfc6962.DefaultHasher.HashLeaf([]byte(logValue))
	identityHashString := strings.ToUpper(hex.EncodeToString(identityHash))

	tamperedLogValue := "This was tampered"
	tamperedIdentityHash := rfc6962.DefaultHasher.HashLeaf([]byte(tamperedLogValue))
	tamperedIdentityHashString := strings.ToUpper(hex.EncodeToString(tamperedIdentityHash))

	mockedLogLeaf := &trillian.LogLeaf{
		MerkleLeafHash: identityHash,
		LeafValue:      []byte(tamperedLogValue),
	}

	uc := UseCase{
		TrillianRepo: out.NewMockRepository(gomock.NewController(t)),
	}

	uc.TrillianRepo.(*out.MockRepository).
		EXPECT().
		GetLogByHash(gomock.Any(), treeID, identityHashString).
		Return(mockedLogLeaf, nil).
		Times(1)

	merkleLeafHash, recalculatedHash, isTampered, err := uc.ValidatedLogHash(context.TODO(), treeID, identityHashString)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert.Equal(t, identityHashString, merkleLeafHash)
	assert.Equal(t, tamperedIdentityHashString, recalculatedHash)
	assert.Equal(t, true, isTampered)
}
