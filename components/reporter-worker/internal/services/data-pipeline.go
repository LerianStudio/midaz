// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	constant "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"

	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// downloadExtractedData downloads the extracted data from Fetcher's storage.
// Supports both SeaweedFS Filer HTTP (single-tenant) and S3 (multi-tenant).
// The path comes from the Fetcher notification result.path (e.g., "/external-data/{jobId}.json").
func (uc *UseCase) downloadExtractedData(ctx context.Context, dataPath string) ([]byte, error) {
	ctx, span := uc.Tracer.Start(ctx, "service.notification.download_extracted_data")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.data_path", dataPath))

	if uc.FetcherDataStorage == nil {
		return nil, pkg.FailedPreconditionError{Code: constant.ErrCodeStorageNotConfigured.Error(), Title: "Storage Not Configured", Message: "fetcher data storage client is not configured"}
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Downloading extracted data from Fetcher storage",
		log.String("path", dataPath))

	data, err := uc.FetcherDataStorage.DownloadFile(ctx, dataPath)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to download extracted data from Fetcher storage", err)
		return nil, fmt.Errorf("download extracted data from %s: %w", dataPath, err)
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Downloaded extracted data",
		log.String("path", dataPath), log.Int("size_bytes", len(data)))

	return data, nil
}

// decryptExtractedData decrypts the raw data using AES-GCM with the HKDF-derived storage key.
// Gap 10: If no derived key is set, returns the data as-is (unencrypted mode).
func (uc *UseCase) decryptExtractedData(ctx context.Context, rawData []byte) ([]byte, error) {
	ctx, span := uc.Tracer.Start(ctx, "service.notification.decrypt_extracted_data")
	defer span.End()

	if len(uc.StorageDecryptKey) == 0 {
		uc.Logger.Log(ctx, log.LevelInfo, "Storage decryption key not configured, skipping decryption")
		return rawData, nil
	}

	decrypted, err := decryptFetcherData(rawData, uc.StorageDecryptKey)
	if err != nil {
		// Decrypt errors are already typed; propagate directly to preserve the code.
		return nil, err
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Decrypted extracted data",
		log.Int("encrypted_size", len(rawData)),
		log.Int("decrypted_size", len(decrypted)))

	return decrypted, nil
}

// auditHMAC verifies the HMAC signature of extracted data for integrity auditing.
// Gap 11 (D6): Log-only in MVP -- logs match/mismatch but does NOT reject on mismatch.
func (uc *UseCase) auditHMAC(ctx context.Context, data []byte, receivedHMAC string) {
	ctx, span := uc.Tracer.Start(ctx, "service.notification.audit_hmac")
	defer span.End()

	if receivedHMAC == "" {
		uc.Logger.Log(ctx, log.LevelInfo, "No HMAC signature in notification, skipping verification")
		span.SetAttributes(attribute.String("app.hmac.result", "skipped"))

		return
	}

	if len(uc.ExternalHMACKey) == 0 {
		uc.Logger.Log(ctx, log.LevelWarn, "External HMAC key not configured, cannot verify HMAC")
		span.SetAttributes(attribute.String("app.hmac.result", "skipped_no_key"))

		return
	}

	match := verifyHMAC(data, receivedHMAC, uc.ExternalHMACKey)

	if match {
		uc.Logger.Log(ctx, log.LevelInfo, "HMAC verification: match",
			log.Int("data_size", len(data)))
		span.SetAttributes(attribute.String("app.hmac.result", "match"))
	} else {
		uc.Logger.Log(ctx, log.LevelWarn, "HMAC verification: mismatch (log-only per D6, not rejecting)",
			log.Int("data_size", len(data)))
		span.SetAttributes(attribute.String("app.hmac.result", "mismatch"))
	}
}

// parseExtractedData unmarshals decrypted JSON data into the result map structure
// expected by the template rendering pipeline.
func parseExtractedData(data []byte) (map[string]map[string][]map[string]any, error) {
	var result map[string]map[string][]map[string]any

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, pkg.FailedPreconditionError{Code: constant.ErrCodeInvalidExtractedData.Error(), Title: "Invalid Extracted Data", Message: fmt.Sprintf("unmarshal extracted data: %s", err.Error()), Err: err}
	}

	return result, nil
}

// convertResultSchemaNotation applies datasource.ConvertSchemaNotation to all
// table-level keys in the result map. Gap 12: Converts "schema.table" keys to
// "schema__table" for Pongo2 template variable compatibility.
func convertResultSchemaNotation(result map[string]map[string][]map[string]any) map[string]map[string][]map[string]any {
	converted := make(map[string]map[string][]map[string]any, len(result))

	for dbName, tables := range result {
		convertedTables := make(map[string][]map[string]any, len(tables))

		for tableKey, rows := range tables {
			newKey := datasource.ConvertSchemaNotation(tableKey)
			convertedTables[newKey] = rows
		}

		converted[dbName] = convertedTables
	}

	return converted
}
