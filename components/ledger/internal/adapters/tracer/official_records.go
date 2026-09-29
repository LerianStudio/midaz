// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"errors"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
)

// ErrOfficialRecordsUnavailable marks a read that failed because the record
// store could not answer (connection, transaction or driver failure), as
// opposed to records that were read and found missing or invalid. Admission
// treats it as an availability failure, so the ledger's fail posture decides.
var ErrOfficialRecordsUnavailable = errors.New("official tracer records unavailable")

// OfficialRecordsReader returns complete records from one tenant-primary
// snapshot, scoped to organization/ledger. Account IDs must be unique;
// missing, deleted or ambiguous accounts fail. A store failure wraps
// ErrOfficialRecordsUnavailable.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=official_records.go -destination=mocks/official_records_mock.go -package=mocks
type OfficialRecordsReader interface {
	Read(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) ([]*mmodel.Account, error)
}
