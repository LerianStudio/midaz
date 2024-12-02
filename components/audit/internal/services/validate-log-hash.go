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

type HashValidation struct {
	OperationID    string `json:"operationId"`
	ExpectedHash   string `json:"expectedHash"`
	CalculatedHash string `json:"calculatedHash"`
	WasTempered    bool   `json:"wasTempered"`
}

func (uc *UseCase) ValidatedLogHash(ctx context.Context, treeID int64, identityHash string) (*HashValidation, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_validated_log_hash")
	defer span.End()

	log, err := uc.TrillianRepo.GetLogByHash(ctx, treeID, identityHash)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get log by hash", err)

		logger.Errorf("Error getting log by hash: %v", err)

		return nil, err
	}

	recalculatedHash := rfc6962.DefaultHasher.HashLeaf(log.LeafValue)
	return &HashValidation{
		ExpectedHash:   strings.ToUpper(hex.EncodeToString(log.MerkleLeafHash)),
		CalculatedHash: strings.ToUpper(hex.EncodeToString(recalculatedHash)),
		WasTempered:    !bytes.Equal(log.MerkleLeafHash, recalculatedHash),
	}, nil
}
