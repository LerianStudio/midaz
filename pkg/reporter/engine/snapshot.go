// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"sort"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/lib/pq"
)

// pqStringArray adapts a []string into a driver value usable with a Postgres
// `= ANY($1)` predicate, matching the reporter's existing schema introspection
// (lib/pq.Array). It is the single binding point for the schema array so the
// embedded engine and the legacy repository send identical wire arguments.
func pqStringArray(values []string) any {
	return pq.Array(values)
}

// buildSnapshot assembles a deterministic SchemaSnapshot from a
// qualified-table -> columns map. Tables and their fields are sorted so the
// snapshot is byte-stable regardless of map iteration order — the engine's
// integrity model relies on deterministic schema output.
func buildSnapshot(configName string, tables map[string][]string) fetcher.SchemaSnapshot {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}

	sort.Strings(names)

	snapshot := fetcher.SchemaSnapshot{
		ConfigName: configName,
		Tables:     make([]fetcher.TableSnapshot, 0, len(names)),
	}

	for _, name := range names {
		fields := append([]string(nil), tables[name]...)
		sort.Strings(fields)

		snapshot.Tables = append(snapshot.Tables, fetcher.TableSnapshot{
			Name:   name,
			Fields: fields,
		})
	}

	return snapshot
}

// classifyQueryError maps a datasource read failure to the correct engine error
// category. A cancelled or deadline-exceeded context is surfaced as the
// engine's context-mapped category (timeout vs canceled) so the host can tell a
// withdrawn request apart from a slow datasource; everything else is a live
// dependency failure (CategoryUnavailable). The raw cause is preserved for
// errors.Is/As transparency but never rendered, keeping any DSN out of the
// boundary message.
func classifyQueryError(ctx context.Context, message string, cause error) error {
	switch {
	case errors.Is(ctx.Err(), context.Canceled), errors.Is(cause, context.Canceled):
		return fetcher.NewWrappedEngineError(fetcher.CategoryCanceled, message+": canceled", cause)
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(cause, context.DeadlineExceeded):
		return fetcher.NewWrappedEngineError(fetcher.CategoryTimeout, message+": deadline exceeded", cause)
	default:
		return NewEngineUnavailableError(message, cause)
	}
}
