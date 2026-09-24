// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/google/uuid"
)

// LimitAssetBinder requires authenticated producer and administrative context.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=limit_asset_service.go -destination=mock_limit_asset_service_test.go -package=in
type LimitAssetBinder interface {
	Execute(context.Context, uuid.UUID, []tracercontract.AccountAsset) (*tracercontract.AssetRef, error)
}
