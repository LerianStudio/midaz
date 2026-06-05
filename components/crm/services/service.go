// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"github.com/LerianStudio/midaz/v3/components/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/components/crm/adapters/mongodb/instrument"
)

type UseCase struct {
	HolderRepo     holder.Repository
	InstrumentRepo instrument.Repository
}
