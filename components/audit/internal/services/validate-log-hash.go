package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/transparency-dev/merkle/rfc6962"
	"strings"
)

// ValidatedLogHash checks if the leaf value was tampered
func (uc *UseCase) ValidatedLogHash(ctx context.Context, treeID int64, identityHash string) (string, string, bool, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "services.get_validated_log_hash")
	defer span.End()

	log, err := uc.TrillianRepo.GetLogByHash(ctx, treeID, identityHash)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get log by hash", err)

		logger.Errorf("Error getting log by hash: %v", err)

		return "", "", false, err
	}

	recalculatedHash := rfc6962.DefaultHasher.HashLeaf(log.LeafValue)

	return formatHash(log.MerkleLeafHash), formatHash(recalculatedHash), !bytes.Equal(log.MerkleLeafHash, recalculatedHash), nil
}

func formatHash(input []byte) string {
	return strings.ToUpper(hex.EncodeToString(input))
}
