// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
)

// OfficialRecordsReader returns complete records from one tenant-primary
// snapshot, scoped to organization/ledger. Account IDs must be unique. Assets
// cover both the accounts and explicit entry codes; missing/ambiguous facts fail.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=official_records.go -destination=mocks/official_records_mock.go -package=mocks
type OfficialRecordsReader interface {
	Read(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID, []string) ([]*mmodel.Account, []*mmodel.Asset, error)
}
