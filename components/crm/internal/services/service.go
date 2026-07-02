// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
)

type UseCase struct {
	HolderRepo holder.Repository
	AliasRepo  alias.Repository

	// Streaming is the lib-streaming event emitter. A nil value means
	// streaming is disabled; emit sites must guard with `if uc.Streaming != nil`.
	// When STREAMING_ENABLED=false, bootstrap injects a NoopEmitter.
	Streaming libStreaming.Emitter
}
