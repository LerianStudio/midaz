// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v4/components/reporter-worker/internal/services/plugincrm"
	pkgErr "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"

	fetcherEngine "github.com/LerianStudio/fetcher/pkg/engine"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// singleTenantEngineTenantID is the synthetic tenant identity used to drive the
// embedded engine in single-tenant mode ONLY. The engine rejects an empty
// TenantContext (tenantId is its isolation boundary), but the single-tenant
// resolver ignores tenant identity entirely, so a stable non-empty placeholder
// satisfies the engine contract without affecting resolution. It mirrors the
// pkgStreaming.DefaultTenantID convention ("default").
//
// CRITICAL: this placeholder is applied ONLY when the engine resolver is
// single-tenant (UseCase.EngineMultiTenant == false). In multi-tenant mode it is
// never substituted: an empty request tenant fails closed so a tenant-less job
// can never resolve a wrong/shared database (the literal tenant named "default"
// passes the lib-commons tenant-id shape check, so substituting it in MT mode
// would silently read that tenant's data — a cross-tenant isolation breach).
const singleTenantEngineTenantID = "default"

// extractViaEngine drives the embedded extraction engine for every datasource in
// the message and populates result with rows re-keyed back into the
// Pongo2-compatible map[databaseName][schema__table][]rows shape the template
// renderer consumes. Per-datasource (per-section) failures are accumulated into
// the returned []sectionFailure rather than aborting the whole report, exactly
// as the legacy direct path did; the returned error is reserved for fatal
// conditions (context cancellation). E9 partial-failure semantics are preserved:
// a failed section is dropped from result and recorded with its classified code.
//
// plugin_crm datasources do NOT route through the generic engine: the engine
// queries literal collection names, while plugin_crm requires the holders_* org
// fan-out, hash-based pre-filter transform, and field decryption. Those run via
// the plugincrm stage against the host-owned per-tenant mongo repository.
func (uc *UseCase) extractViaEngine(
	ctx context.Context,
	message GenerateReportMessage,
	result map[string]map[string][]map[string]any,
) ([]sectionFailure, error) {
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.extract_via_engine")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.Int("app.request.datasource_count", len(message.DataQueries)),
	)

	var failures []sectionFailure

	for databaseName, tables := range message.DataQueries {
		if err := ctx.Err(); err != nil {
			return failures, err
		}

		if err := uc.extractDatasource(ctx, message, databaseName, tables, result); err != nil {
			uc.Logger.Log(ctx, log.LevelWarn, "Data section failed, recording for partial classification",
				log.String("database", databaseName), log.Err(err))

			failures = append(failures, sectionFailure{
				database:  databaseName,
				errorCode: classifyReporterErrorCode(err),
			})

			// Drop any partial map seeded for this section so it is not rendered.
			delete(result, databaseName)
		}
	}

	span.SetAttributes(attribute.Int("app.report.failed_section_count", len(failures)))

	return failures, nil
}

// extractDatasource extracts a single datasource (report section). It dispatches
// plugin_crm to the dedicated fan-out/decrypt path and every other datasource to
// the embedded engine.
func (uc *UseCase) extractDatasource(
	ctx context.Context,
	message GenerateReportMessage,
	databaseName string,
	tables map[string][]string,
	result map[string]map[string][]map[string]any,
) error {
	if plugincrm.Is(databaseName) {
		return uc.extractPluginCRM(ctx, databaseName, tables, message.Filters, result)
	}

	return uc.extractEngineDatasource(ctx, message, databaseName, tables, result)
}

// extractEngineDatasource plans and executes an engine extraction for one
// datasource and re-keys the decoded rows into result. The MappedFields and
// Filters are converted from the reporter's Pongo2 schema__table notation to the
// engine's dot-notation; the decoded result tables are converted back.
func (uc *UseCase) extractEngineDatasource(
	ctx context.Context,
	message GenerateReportMessage,
	databaseName string,
	tables map[string][]string,
	result map[string]map[string][]map[string]any,
) error {
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.extract_engine_datasource")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.database_name", databaseName),
	)

	if uc.Engine == nil {
		return pkgErr.FailedPreconditionError{
			Code:    cnErr.ErrCodeDataSourceUnavailable.Error(),
			Title:   "Extraction Engine Unavailable",
			Message: "embedded extraction engine is not configured",
		}
	}

	tenant, err := uc.engineTenantContext(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to build engine tenant context", err)
		return err
	}

	// Resolve every requested table key to the qualified table the connector
	// streams under BEFORE planning. The engine validates each mapped table by
	// exact name against the discovered schema snapshot, whose postgres tables
	// are keyed "schema.table"; a bare template key (e.g. "organization") would
	// otherwise miss validation and drop the whole section. This reproduces the
	// legacy SchemaResolver autodiscovery (single-schema match, public
	// preference, ambiguity error) the direct path performed.
	snapshot, err := uc.Engine.DiscoverSchema(ctx, tenant, databaseName)
	if err != nil {
		libOtel.HandleSpanError(span, "Engine DiscoverSchema failed", err)
		return err
	}

	keyMap, err := resolveTableKeys(databaseName, snapshot, tables)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to resolve table keys against schema", err)
		return err
	}

	request := buildExtractionRequest(message, databaseName, tables, keyMap)

	plan, err := uc.Engine.PlanExtraction(ctx, tenant, request)
	if err != nil {
		libOtel.HandleSpanError(span, "Engine PlanExtraction failed", err)
		return err
	}

	extraction, err := uc.Engine.ExecuteExtraction(ctx, plan)
	if err != nil {
		libOtel.HandleSpanError(span, "Engine ExecuteExtraction failed", err)
		return err
	}

	decoded, err := decodeDirectResult(extraction)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to decode engine extraction result", err)
		return err
	}

	rekeyEngineResult(databaseName, decoded, keyMap, result)

	if extraction.RowCounts != nil {
		span.SetAttributes(attribute.Int64("app.report.row_count", extraction.RowCounts[databaseName]))
	}

	return nil
}

// resolveTenantID extracts the tenant ID from context using the lib-commons
// tenant-manager API. Returns empty string in single-tenant mode (no tenant in
// context).
func resolveTenantID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	return tmcore.GetTenantIDContext(ctx)
}

// engineTenantContext builds the engine TenantContext from the request context.
// In multi-tenant mode the tenant ID MUST come from context; an empty tenant is
// returned unchanged so the multi-tenant resolver's requireTenant guard fails
// the extraction closed rather than reading a wrong/shared database. In
// single-tenant mode an empty tenant is replaced with a stable placeholder
// because the engine rejects an empty tenant and the single-tenant resolver
// ignores tenant identity anyway.
func (uc *UseCase) engineTenantContext(ctx context.Context) (fetcherEngine.TenantContext, error) {
	tenantID := resolveTenantID(ctx)
	if tenantID == "" && !uc.EngineMultiTenant {
		tenantID = singleTenantEngineTenantID
	}

	return fetcherEngine.NewTenantContext(tenantID)
}

// resolveTableKeys maps every requested template table key to the qualified
// table name the connector streams under, validated against the discovered
// schema snapshot. It reproduces the legacy SchemaResolver autodiscovery the
// direct path performed: an explicitly-qualified key (Pongo2 "schema__table" or
// dot "schema.table") must match the snapshot exactly; a bare key is resolved by
// autodiscovery — exact match wins (mongo collections and already-public tables),
// otherwise the single schema that owns the table, preferring "public" when
// several do, and erroring with actionable schema-qualification guidance when the
// table is genuinely ambiguous. A key that resolves to no snapshot table yields a
// not-found error so the section fails loudly rather than silently dropping rows.
//
// The returned map is keyed by the ORIGINAL template key; the value is the engine
// (snapshot) key. It drives both the request (original -> engine) and the result
// re-keying (engine -> original), so a bare-keyed template gets its rows stored
// back under the bare key the renderer reads.
func resolveTableKeys(
	databaseName string,
	snapshot fetcherEngine.SchemaSnapshot,
	tables map[string][]string,
) (map[string]string, error) {
	bySchema := indexSnapshotTablesBySchema(snapshot)

	keyMap := make(map[string]string, len(tables))

	for tableKey := range tables {
		engineKey := toEngineTableKey(tableKey)

		// An explicit or already-qualified key (or a bare mongo collection that
		// exists verbatim in the snapshot) is accepted as-is.
		if snapshot.HasTable(engineKey) {
			keyMap[tableKey] = engineKey
			continue
		}

		// A key carrying an explicit schema separator that did not match exactly
		// is a genuine miss — do not try to autodiscover a schema for it.
		if strings.Contains(tableKey, "__") || strings.Contains(tableKey, ".") {
			return nil, tableNotFoundError(databaseName, engineKey)
		}

		resolved, err := autodiscoverQualifiedTable(databaseName, tableKey, bySchema[tableKey])
		if err != nil {
			return nil, err
		}

		keyMap[tableKey] = resolved
	}

	return keyMap, nil
}

// autodiscoverQualifiedTable resolves a bare table name to its qualified
// "schema.table" snapshot key from the schemas that own it: zero owners is a
// not-found error, one owner resolves directly, several owners prefer "public"
// and otherwise return an ambiguity error mirroring the legacy
// SchemaAmbiguityError guidance.
func autodiscoverQualifiedTable(databaseName, tableName string, schemas []string) (string, error) {
	switch len(schemas) {
	case 0:
		return "", tableNotFoundError(databaseName, tableName)
	case 1:
		return schemas[0] + "." + tableName, nil
	default:
		for _, schema := range schemas {
			if schema == "public" {
				return "public." + tableName, nil
			}
		}

		return "", ambiguousTableError(databaseName, tableName, schemas)
	}
}

// indexSnapshotTablesBySchema groups the snapshot's qualified "schema.table"
// names by bare table name, recording the owning schemas. Bare snapshot keys
// (mongo collections) are skipped: they are resolved by exact match upstream.
func indexSnapshotTablesBySchema(snapshot fetcherEngine.SchemaSnapshot) map[string][]string {
	bySchema := make(map[string][]string)

	for _, name := range snapshot.TableNames() {
		dot := strings.Index(name, ".")
		if dot == -1 {
			continue
		}

		schema, table := name[:dot], name[dot+1:]
		bySchema[table] = append(bySchema[table], schema)
	}

	return bySchema
}

// tableNotFoundError reports a mapped table absent from the discovered schema as
// a validation error, so extractViaEngine records the section as a classified
// failure (E9) rather than silently dropping it.
func tableNotFoundError(databaseName, table string) error {
	return pkgErr.ValidationError{
		Code:    cnErr.ErrCodeDataSourceNotFound.Error(),
		Title:   "Table Not Found",
		Message: fmt.Sprintf("table %q not found in any configured schema of datasource %q", table, databaseName),
	}
}

// ambiguousTableError reports a bare table reference that exists in multiple
// schemas, carrying the explicit-schema guidance the legacy SchemaAmbiguityError
// produced so a template author can disambiguate with "schema__table".
func ambiguousTableError(databaseName, table string, schemas []string) error {
	suggestions := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		suggestions = append(suggestions, schema+"__"+table)
	}

	return pkgErr.ValidationError{
		Code:  cnErr.ErrCodeDataSourceNotFound.Error(),
		Title: "Ambiguous Table Reference",
		Message: fmt.Sprintf("table %q exists in multiple schemas [%s] of datasource %q; qualify it explicitly, e.g. %s",
			table, strings.Join(schemas, ", "), databaseName, strings.Join(suggestions, ", ")),
	}
}

// buildExtractionRequest assembles the engine ExtractionRequest for a single
// datasource. MappedFields are projected to the qualified engine table keys
// resolved against the schema (keyMap: original template key -> engine key);
// Filters carry the per-datasource table/field FilterConditions in the nested
// map[string]any shape the engine planner walks structurally (datasource ->
// table -> field -> model.FilterCondition value). The planner copies the field
// value opaquely and the reporter's connector adapter decodes it back to a
// FilterCondition at query time.
func buildExtractionRequest(
	message GenerateReportMessage,
	databaseName string,
	tables map[string][]string,
	keyMap map[string]string,
) fetcherEngine.ExtractionRequest {
	selection := make(fetcherEngine.FieldSelection, len(tables))
	for tableKey, fields := range tables {
		selection[keyMap[tableKey]] = fields
	}

	request := fetcherEngine.ExtractionRequest{
		MappedFields: map[string]fetcherEngine.FieldSelection{databaseName: selection},
		Metadata: map[string]any{
			"source":     constant.FetcherNotificationRoutingSource,
			"reportId":   message.ReportID.String(),
			"templateId": message.TemplateID.String(),
		},
	}

	if filters := engineFiltersForDatasource(message.Filters, databaseName, keyMap); filters != nil {
		request.Filters = map[string]any{databaseName: filters}
	}

	return request
}

// engineFiltersForDatasource projects the message's per-datasource filter tree
// onto the nested map[string]any shape the engine planner accepts, mapping each
// filter's table key to the qualified engine key resolved against the schema
// (keyMap) so it lines up with the table the connector streams under. The leaf
// value stays a typed model.FilterCondition; the planner copies it opaquely and
// the connector adapter decodes it back. A filter whose table key was not in the
// mapped-fields request (and so absent from keyMap) is dropped — there is nothing
// to filter. A datasource with no filters yields nil.
func engineFiltersForDatasource(
	allFilters map[string]map[string]map[string]model.FilterCondition,
	databaseName string,
	keyMap map[string]string,
) map[string]any {
	tables, ok := allFilters[databaseName]
	if !ok || len(tables) == 0 {
		return nil
	}

	converted := make(map[string]any, len(tables))

	for tableKey, fields := range tables {
		engineKey, ok := keyMap[tableKey]
		if !ok {
			continue
		}

		tableFields := make(map[string]any, len(fields))
		for field, condition := range fields {
			tableFields[field] = condition
		}

		converted[engineKey] = tableFields
	}

	if len(converted) == 0 {
		return nil
	}

	return converted
}

// toEngineTableKey converts a reporter table key from Pongo2 notation
// (schema__table) to the engine/SQL dot notation (schema.table) the connector
// queries under. A bare collection name (no separator) is returned unchanged.
func toEngineTableKey(tableKey string) string {
	return strings.ReplaceAll(tableKey, "__", ".")
}

// decodeDirectResult decodes the engine's DirectResult.Data JSON bytes into the
// nested map[configName]map[qualifiedTable][]rows shape and returns the inner
// per-table map for the single datasource the plan extracted. A store-mode
// result (nil Direct) is rejected: the worker drives the engine in direct mode.
func decodeDirectResult(extraction fetcherEngine.ExtractionResult) (map[string]map[string][]map[string]any, error) {
	if extraction.Direct == nil {
		return nil, pkgErr.FailedPreconditionError{
			Code:    cnErr.ErrCodeInvalidExtractedData.Error(),
			Title:   "Invalid Extraction Result",
			Message: "engine returned no direct result payload",
		}
	}

	if len(extraction.Direct.Data) == 0 {
		return map[string]map[string][]map[string]any{}, nil
	}

	var decoded map[string]map[string][]map[string]any
	if err := json.Unmarshal(extraction.Direct.Data, &decoded); err != nil {
		return nil, pkgErr.FailedPreconditionError{
			Code:    cnErr.ErrCodeInvalidExtractedData.Error(),
			Title:   "Invalid Extracted Data",
			Message: fmt.Sprintf("unmarshal engine extraction result: %s", err.Error()),
			Err:     err,
		}
	}

	return decoded, nil
}

// rekeyEngineResult merges the engine's decoded output for one datasource into
// result, restoring the ORIGINAL template table key each engine result was
// extracted under so the renderer finds its rows where the template references
// them.
//
// THE PARITY-CRITICAL STEP: the engine aggregates rows under config name then
// the qualified table it queried (postgres "schema.table", bare mongo
// "collection"); the renderer expects map[databaseName][originalTemplateKey][]rows.
// keyMap (original template key -> engine key) is inverted so each engine key is
// stored back under the original key the template used — bare or qualified —
// exactly as the legacy direct path preserved the Pongo2 key. An engine result
// table with no inverse mapping is stored under its own key as a safe fallback so
// rows are never silently dropped.
func rekeyEngineResult(
	databaseName string,
	decoded map[string]map[string][]map[string]any,
	keyMap map[string]string,
	result map[string]map[string][]map[string]any,
) {
	tables, ok := decoded[databaseName]
	if !ok {
		return
	}

	originalByEngine := make(map[string]string, len(keyMap))
	for original, engineKey := range keyMap {
		originalByEngine[engineKey] = original
	}

	section, exists := result[databaseName]
	if !exists {
		section = make(map[string][]map[string]any, len(tables))
		result[databaseName] = section
	}

	for engineKey, rows := range tables {
		original, ok := originalByEngine[engineKey]
		if !ok {
			original = engineKey
		}

		section[original] = rows
	}
}

// extractPluginCRM reproduces the legacy plugin_crm extraction path: for each
// requested logical collection it runs the PRE-FILTER hash transform, fans out
// over the org-scoped physical collections (holders_*) with organization_id
// injection, then runs the POST-EXTRACTION field decryption. The synthetic
// "organization" collection is skipped (metadata, not queryable).
func (uc *UseCase) extractPluginCRM(
	ctx context.Context,
	databaseName string,
	collections map[string][]string,
	allFilters map[string]map[string]map[string]model.FilterCondition,
	result map[string]map[string][]map[string]any,
) error {
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.extract_plugin_crm")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.database_name", databaseName),
	)

	dataSource, exists := uc.ExternalDataSources.Get(databaseName)
	if !exists {
		err := pkgErr.ValidationError{
			Code:    cnErr.ErrCodeDataSourceNotFound.Error(),
			Title:   "Data Source Not Found",
			Message: fmt.Sprintf("data source not found: %s", databaseName),
		}
		libOtel.HandleSpanError(span, "Referenced datasource is missing", err)

		return err
	}

	if dataSource.MongoDBRepository == nil {
		return pkgErr.FailedPreconditionError{
			Code:    cnErr.ErrCodeDataSourceUnavailable.Error(),
			Title:   "Data Source Unavailable",
			Message: fmt.Sprintf("plugin_crm datasource %s has no mongo connection", databaseName),
		}
	}

	if _, ready := result[databaseName]; !ready {
		result[databaseName] = make(map[string][]map[string]any)
	}

	databaseFilters := allFilters[databaseName]
	querier := &pluginCRMQuerier{repo: dataSource.MongoDBRepository}

	for collection, fields := range collections {
		if err := ctx.Err(); err != nil {
			return err
		}

		if !plugincrm.IsQueryableCollection(collection) {
			uc.Logger.Log(ctx, log.LevelDebug,
				"Skipping plugin_crm organization collection - metadata, not queryable",
				log.String("collection", collection))

			continue
		}

		if err := uc.extractPluginCRMCollection(ctx, querier, databaseName, collection, fields, databaseFilters, result); err != nil {
			libOtel.HandleSpanError(span, "Error extracting plugin_crm collection", err)
			return err
		}
	}

	return nil
}

// extractPluginCRMCollection runs the three plugin_crm seams for one logical
// collection: PRE-FILTER transform, org fan-out, POST-EXTRACTION decryption.
func (uc *UseCase) extractPluginCRMCollection(
	ctx context.Context,
	querier plugincrm.CollectionQuerier,
	databaseName, collection string,
	fields []string,
	databaseFilters map[string]map[string]model.FilterCondition,
	result map[string]map[string][]map[string]any,
) error {
	collectionFilters := getTableFilters(databaseFilters, collection)

	transformedFilters, err := plugincrm.TransformFilters(collectionFilters, uc.CryptoHashSecretKeyPluginCRM, uc.Logger)
	if err != nil {
		return err
	}

	rows, err := uc.runPluginCRMFanOut(ctx, querier, collection, fields, transformedFilters)
	if err != nil {
		return err
	}

	decrypted, err := plugincrm.DecryptRecords(rows, fields, uc.CryptoHashSecretKeyPluginCRM, uc.CryptoEncryptSecretKeyPluginCRM, uc.Logger)
	if err != nil {
		return pkgErr.ValidateBusinessError(cnErr.ErrDecryptionData, "", err)
	}

	result[databaseName][collection] = decrypted

	return nil
}

// runPluginCRMFanOut executes the org-collection fan-out under circuit-breaker
// protection, keeping the same breaker keying the legacy worker used (keyed by
// the plugin_crm datasource name) so resilience parity is preserved.
func (uc *UseCase) runPluginCRMFanOut(
	ctx context.Context,
	querier plugincrm.CollectionQuerier,
	collection string,
	fields []string,
	filters map[string]model.FilterCondition,
) ([]map[string]any, error) {
	fanOutResult, err := uc.CircuitBreakerManager.Execute(plugincrm.DatasourceName, func() (any, error) {
		return plugincrm.FanOutOrgCollections(ctx, querier, collection, fields, filters)
	})
	if err != nil {
		return nil, err
	}

	rows, ok := fanOutResult.([]map[string]any)
	if !ok {
		return nil, pkgErr.FailedPreconditionError{
			Code:    cnErr.ErrCodeUnexpectedCollectionResult.Error(),
			Title:   "Unexpected Query Result",
			Message: fmt.Sprintf("unexpected fan-out result type for plugin_crm collection %s", collection),
		}
	}

	return rows, nil
}

// pluginCRMQuerier adapts the reporter's mongodb.Repository onto the
// plugincrm.CollectionQuerier seam, routing the no-filter case to Query and the
// filtered case to QueryWithAdvancedFilters exactly as the legacy worker did.
type pluginCRMQuerier struct {
	repo pluginCRMRepository
}

// pluginCRMRepository is the narrow read surface the fan-out needs from the
// reporter's mongo repository. *mongodb.ExternalDataSource satisfies it.
type pluginCRMRepository interface {
	ListCollectionNames(ctx context.Context) ([]string, error)
	Query(ctx context.Context, collection string, fields []string, filter map[string][]any) ([]map[string]any, error)
	QueryWithAdvancedFilters(ctx context.Context, collection string, fields []string, filter map[string]model.FilterCondition) ([]map[string]any, error)
}

// ListCollectionNames forwards to the repository.
func (q *pluginCRMQuerier) ListCollectionNames(ctx context.Context) ([]string, error) {
	return q.repo.ListCollectionNames(ctx)
}

// QueryCollection queries one physical collection, using advanced filters when
// present and the legacy plain Query otherwise.
func (q *pluginCRMQuerier) QueryCollection(ctx context.Context, physicalCollection string, fields []string, filters map[string]model.FilterCondition) ([]map[string]any, error) {
	if len(filters) > 0 {
		return q.repo.QueryWithAdvancedFilters(ctx, physicalCollection, fields, filters)
	}

	return q.repo.Query(ctx, physicalCollection, fields, nil)
}

var _ plugincrm.CollectionQuerier = (*pluginCRMQuerier)(nil)
