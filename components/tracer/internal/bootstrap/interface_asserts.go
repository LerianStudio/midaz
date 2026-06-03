// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/command"
)

// Compile-time interface conformance checks. If RecordAuditEventCommand's
// method set ever drifts from either AuditWriter interface, the build fails
// here rather than at a distant call site (handler wiring, service
// instantiation, or downstream caller).
//
// Placement rationale: the command package cannot import services, and the
// services package importing command would create a cycle for the
// services.AuditWriter assertion. The bootstrap package already imports
// both packages to wire the composition root, so asserting conformance
// here keeps the import graph acyclic.
var (
	_ command.AuditWriter  = (*command.RecordAuditEventCommand)(nil)
	_ services.AuditWriter = (*command.RecordAuditEventCommand)(nil)
)
