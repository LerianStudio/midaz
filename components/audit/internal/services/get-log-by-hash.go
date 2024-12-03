package services

import (
	"context"
	"encoding/hex"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"strings"
)

// GetLogByHash search for leaf value by the leaf identity hash
func (uc *UseCase) GetLogByHash(ctx context.Context, treeID int64, identityHash string) (string, []byte, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "services.get_log_by_hash")
	defer span.End()

	log, err := uc.TrillianRepo.GetLogByHash(ctx, treeID, identityHash)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get log by hash", err)

		logger.Errorf("Error getting log by hash: %v", err)

		return "", nil, err
	}

	return strings.ToUpper(hex.EncodeToString(log.MerkleLeafHash)), log.LeafValue, nil
}
