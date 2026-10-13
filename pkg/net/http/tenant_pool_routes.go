// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"context"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v3"
)

// TenantPoolConstructor builds the handle that replaces the PostgreSQL pool
// lib-commons pinned into the request context for one module. pinned is the
// pool WithTenantDB stored; the returned handle is stored under the same
// module key.
type TenantPoolConstructor func(ctx context.Context, tenantID string, pinned dbresolver.DB) dbresolver.DB

// TenantPoolModule pairs a PostgreSQL module name, as registered with the
// tenant middleware, with the constructor of its replacement handle.
type TenantPoolModule struct {
	Name string
	New  TenantPoolConstructor
}

// WithTenantPoolResolution returns a handler, mounted right after lib-commons'
// WithTenantDB, that replaces the PostgreSQL pool pinned into the request
// context with the handle built by each module's constructor (the ledger
// passes a lazy adapter that re-resolves the tenant pool per operation).
//
// Only module keys already present in the context are replaced: an absent
// module key is never created and the generic key is never read or written,
// so per-chain isolation set up by WithTenantDB is preserved. A request with
// no tenant in the context passes through untouched. Modules with an empty
// name (which would address the generic key) or a nil constructor are ignored,
// and a constructor returning nil keeps the pinned pool. Modules are
// deduplicated by name, keeping the first: a repeated name would otherwise
// hand the second constructor the first one's handle as pinned instead of the
// pool WithTenantDB stored.
func WithTenantPoolResolution(modules ...TenantPoolModule) fiber.Handler {
	active := make([]TenantPoolModule, 0, len(modules))
	seen := make(map[string]struct{}, len(modules))

	for _, m := range modules {
		if m.Name == "" || m.New == nil {
			continue
		}

		if _, dup := seen[m.Name]; dup {
			continue
		}

		seen[m.Name] = struct{}{}
		active = append(active, m)
	}

	if len(active) == 0 {
		return func(c fiber.Ctx) error {
			return c.Next()
		}
	}

	return func(c fiber.Ctx) error {
		ctx := c.Context()

		tenantID := tmcore.GetTenantIDContext(ctx)
		if tenantID == "" {
			return c.Next()
		}

		replaced := false

		for _, m := range active {
			pinned := tmcore.GetPGContext(ctx, m.Name)
			if pinned == nil {
				continue
			}

			handle := m.New(ctx, tenantID, pinned)
			if handle == nil {
				continue
			}

			ctx = tmcore.ContextWithPG(ctx, handle, m.Name)
			replaced = true
		}

		if replaced {
			c.SetContext(ctx)
		}

		return c.Next()
	}
}
