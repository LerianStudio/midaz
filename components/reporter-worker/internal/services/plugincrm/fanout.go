// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package plugincrm

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
)

// CollectionQuerier is the minimal MongoDB seam the org fan-out needs: list the
// physical collection names in the datasource, and query one physical
// collection with the report's field selection and advanced filters. Stage C
// satisfies this over the resolved per-tenant mongo connector; tests satisfy it
// with a fake. Keeping the seam this narrow means the fan-out logic is
// unit-testable without a broker or driver, and never reaches for a connection
// of its own.
type CollectionQuerier interface {
	// ListCollectionNames returns every physical collection name in the
	// datasource, in unspecified order (the fan-out sorts the matches itself).
	ListCollectionNames(ctx context.Context) ([]string, error)

	// QueryCollection returns the rows of a single physical collection projected
	// to the given fields and constrained by the given advanced filters. The
	// filters are the hash-transformed search.* conditions produced by
	// TransformFilters; a nil/empty filter map means no filtering (the whole
	// collection). Applying the filters to EACH physical org collection is what
	// keeps a filtered plugin_crm report from silently widening to every org's
	// full row set.
	QueryCollection(ctx context.Context, physicalCollection string, fields []string, filters map[string]model.FilterCondition) ([]map[string]any, error)
}

// FanOutOrgCollections reproduces the legacy processPluginCRMCollection
// org-collection fan-out for a logical plugin_crm collection (e.g. "holders"):
// it discovers every physical collection named "<collection>_<organizationID>",
// queries each one with the report's field selection and the transformed
// advanced filters, injects the source organization_id into every returned
// record, and merges the per-org rows into a single deterministic slice.
//
// The filters MUST be the hash-transformed conditions from TransformFilters and
// are applied to EACH physical org collection — exactly as the legacy worker
// did via queryMongoCollectionWithFilters. Passing them through here is what
// prevents a filtered plugin_crm report from silently returning every org
// collection's full row set.
//
// The matching collections are sorted before querying so the merge order is
// stable: templates using index-based access (e.g. holders.0.document) must
// render the same organization regardless of ListCollectionNames order.
//
// It performs NO decryption — decryption is the separate DecryptRecords seam the
// handler applies to the merged rows. It never logs record values or PII; the
// only identifier logged is the organization_id derived from the physical
// collection name.
func FanOutOrgCollections(ctx context.Context, querier CollectionQuerier, collection string, fields []string, filters map[string]model.FilterCondition) ([]map[string]any, error) {
	allCollections, err := querier.ListCollectionNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list plugin_crm collections: %w", err)
	}

	prefix := collection + "_"

	matching := make([]string, 0, len(allCollections))

	for _, name := range allCollections {
		if strings.HasPrefix(name, prefix) {
			matching = append(matching, name)
		}
	}

	sort.Strings(matching)

	var merged []map[string]any

	for _, physicalCollection := range matching {
		orgID := strings.TrimPrefix(physicalCollection, prefix)

		rows, queryErr := querier.QueryCollection(ctx, physicalCollection, fields, filters)
		if queryErr != nil {
			return nil, fmt.Errorf("failed to query plugin_crm collection %s: %w", physicalCollection, queryErr)
		}

		for i := range rows {
			rows[i]["organization_id"] = orgID
		}

		merged = append(merged, rows...)
	}

	return merged, nil
}
