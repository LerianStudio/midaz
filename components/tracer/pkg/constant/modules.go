// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// ModuleName is the canonical module identifier for the tracer service when
// registering with the multi-tenant Tenant Manager.
// The Tenant Manager uses the hierarchy Service → Module → Resource.
// Tracer is a single-module service; this constant is the only module name.
const ModuleName = "tracer"
