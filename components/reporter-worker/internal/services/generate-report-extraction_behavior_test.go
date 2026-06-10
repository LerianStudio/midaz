// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	fetcherEngine "github.com/LerianStudio/fetcher/pkg/engine"
	memengine "github.com/LerianStudio/fetcher/pkg/engine/memory"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestEngineTenantContext_FailsClosedInMultiTenant covers the multi-tenant
// isolation guard: an empty request tenant must NOT be substituted with the
// single-tenant placeholder in MT mode (that would read a wrong/shared "default"
// tenant database), it must fail closed; in ST mode the placeholder is applied.
func TestEngineTenantContext_FailsClosedInMultiTenant(t *testing.T) {
	t.Parallel()

	t.Run("multi-tenant + empty tenant fails closed", func(t *testing.T) {
		t.Parallel()

		uc := &UseCase{EngineMultiTenant: true}

		_, err := uc.engineTenantContext(context.Background())
		require.Error(t, err, "empty tenant in MT mode must be rejected, never defaulted")
	})

	t.Run("multi-tenant + present tenant resolves that tenant", func(t *testing.T) {
		t.Parallel()

		uc := &UseCase{EngineMultiTenant: true}
		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-abc")

		tenant, err := uc.engineTenantContext(ctx)
		require.NoError(t, err)
		assert.Equal(t, "tenant-abc", tenant.TenantID)
	})

	t.Run("single-tenant + empty tenant uses placeholder", func(t *testing.T) {
		t.Parallel()

		uc := &UseCase{EngineMultiTenant: false}

		tenant, err := uc.engineTenantContext(context.Background())
		require.NoError(t, err)
		assert.Equal(t, singleTenantEngineTenantID, tenant.TenantID)
	})
}

// newMultiTableEngine builds a real *engine.Engine over a memengine connector
// whose schema and rows are seeded per the given map (qualified table -> rows),
// reachable for every datasource config name in configNames. It lets a handler
// test drive PlanExtraction/ExecuteExtraction across multiple sections.
func newMultiTableEngine(t *testing.T, rows map[string][]map[string]any, configNames ...string) *fetcherEngine.Engine {
	t.Helper()

	const datasourceType = "postgresql"

	tables := make([]fetcherEngine.TableSnapshot, 0, len(rows))
	for name, rs := range rows {
		fields := make([]string, 0)
		if len(rs) > 0 {
			for k := range rs[0] {
				fields = append(fields, k)
			}
		}

		tables = append(tables, fetcherEngine.TableSnapshot{Name: name, Fields: fields})
	}

	connector := memengine.NewTemplateConnector(memengine.ConnectorBehavior{
		Rows:   rows,
		Schema: fetcherEngine.SchemaSnapshot{Tables: tables},
	})

	registry := memengine.NewConnectorRegistry()
	registry.Register(datasourceType, memengine.NewConnectorFactory(connector))

	store := memengine.NewConnectionStore()
	tenant, err := fetcherEngine.NewTenantContext(singleTenantEngineTenantID)
	require.NoError(t, err)

	for _, name := range configNames {
		require.NoError(t, store.Create(context.Background(), tenant, fetcherEngine.ConnectionDescriptor{
			ConfigName: name,
			Type:       datasourceType,
		}, nil))
	}

	engine, err := fetcherEngine.New(
		fetcherEngine.WithConnectorRegistry(registry),
		fetcherEngine.WithConnectionStore(store),
	)
	require.NoError(t, err)

	return engine
}

func newExtractionUseCase(engine *fetcherEngine.Engine) *UseCase {
	return &UseCase{
		Logger: log.NewNop(),
		Tracer: noop.NewTracerProvider().Tracer("test"),
		Engine: engine,
	}
}

// TestExtractViaEngine_PartialAndErrorClassification protects the worker's most
// important behavioral contract: when one section's table is absent from the
// discovered schema (it fails PlanExtraction validation) the section is dropped
// and recorded, and decideReportStatus classifies the report PARTIAL when other
// sections survive, ERROR when every section fails.
func TestExtractViaEngine_PartialAndErrorClassification(t *testing.T) {
	t.Parallel()

	t.Run("one of two sections fails -> Partial with per-section code", func(t *testing.T) {
		t.Parallel()

		// Only "ds_ok"'s table exists in the schema; "ds_bad" maps a table absent
		// from the snapshot, so its PlanExtraction validation fails.
		engine := newMultiTableEngine(t, map[string][]map[string]any{
			"public.organization": {{"name": "World"}},
		}, "ds_ok", "ds_bad")

		uc := newExtractionUseCase(engine)

		message := GenerateReportMessage{
			ReportID:   uuid.New(),
			TemplateID: uuid.New(),
			DataQueries: map[string]map[string][]string{
				"ds_ok":  {"organization": {"name"}},
				"ds_bad": {"missing_table": {"id"}},
			},
		}

		result := make(map[string]map[string][]map[string]any)

		failures, err := uc.extractViaEngine(context.Background(), message, result)
		require.NoError(t, err)
		require.Len(t, failures, 1)
		assert.Equal(t, "ds_bad", failures[0].database)
		assert.NotEmpty(t, failures[0].errorCode, "failed section carries a classified code (E9)")

		// The failed section is dropped; the good one rendered under its bare key.
		assert.NotContains(t, result, "ds_bad")
		assert.Equal(t, []map[string]any{{"name": "World"}}, result["ds_ok"]["organization"])

		status, metadata := decideReportStatus(len(message.DataQueries), failures)
		assert.Equal(t, constant.PartialStatus, status)
		sections, ok := metadata["sections"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, sections, "ds_bad")
	})

	t.Run("all sections fail -> Error", func(t *testing.T) {
		t.Parallel()

		engine := newMultiTableEngine(t, map[string][]map[string]any{
			"public.organization": {{"name": "World"}},
		}, "ds_bad")

		uc := newExtractionUseCase(engine)

		message := GenerateReportMessage{
			ReportID:   uuid.New(),
			TemplateID: uuid.New(),
			DataQueries: map[string]map[string][]string{
				"ds_bad": {"missing_table": {"id"}},
			},
		}

		result := make(map[string]map[string][]map[string]any)

		failures, err := uc.extractViaEngine(context.Background(), message, result)
		require.NoError(t, err)
		require.Len(t, failures, 1)

		status, _ := decideReportStatus(len(message.DataQueries), failures)
		assert.Equal(t, constant.ErrorStatus, status)
	})
}

// recordingConnector is a real engine.Connector that records the filter payload
// it receives in QueryStream, so a test can prove the model.FilterCondition leaf
// survives the planner round-trip (PlanExtraction -> ExecuteExtraction) intact.
type recordingConnector struct {
	snapshot fetcherEngine.SchemaSnapshot
	rows     map[string][]map[string]any
	captured map[string]any
}

func (c *recordingConnector) TestConnection(context.Context) error { return nil }

func (c *recordingConnector) DiscoverSchema(context.Context) (fetcherEngine.SchemaSnapshot, error) {
	return c.snapshot, nil
}

func (c *recordingConnector) QueryStream(_ context.Context, request fetcherEngine.ExtractionRequest) (fetcherEngine.RowCursor, error) {
	c.captured = request.Filters
	return fetcherEngine.NewEagerCursor(c.rows), nil
}

func (c *recordingConnector) Close(context.Context) error { return nil }

type recordingFactory struct{ connector *recordingConnector }

func (f *recordingFactory) Build(context.Context, fetcherEngine.ConnectionDescriptor) (fetcherEngine.Connector, error) {
	return f.connector, nil
}

// TestExtractEngineDatasource_FilterReachesConnectorThroughPlanner proves the
// highest-stakes filter-correctness property end-to-end: a model.FilterCondition
// placed in the request survives PlanExtraction -> ExecuteExtraction and arrives
// at the connector as a model.FilterCondition (not a JSON-normalized copy), so
// the connector can apply it. A regression here would silently change which rows
// render on a financial report.
func TestExtractEngineDatasource_FilterReachesConnectorThroughPlanner(t *testing.T) {
	t.Parallel()

	const datasourceType = "postgresql"

	connector := &recordingConnector{
		snapshot: fetcherEngine.SchemaSnapshot{
			Tables: []fetcherEngine.TableSnapshot{{Name: "public.organization", Fields: []string{"name"}}},
		},
		rows: map[string][]map[string]any{"public.organization": {{"name": "World"}}},
	}

	registry := memengine.NewConnectorRegistry()
	registry.Register(datasourceType, &recordingFactory{connector: connector})

	store := memengine.NewConnectionStore()
	tenant, err := fetcherEngine.NewTenantContext(singleTenantEngineTenantID)
	require.NoError(t, err)
	require.NoError(t, store.Create(context.Background(), tenant, fetcherEngine.ConnectionDescriptor{
		ConfigName: "onboarding",
		Type:       datasourceType,
	}, nil))

	engine, err := fetcherEngine.New(
		fetcherEngine.WithConnectorRegistry(registry),
		fetcherEngine.WithConnectionStore(store),
	)
	require.NoError(t, err)

	uc := newExtractionUseCase(engine)

	condition := model.FilterCondition{Equals: []any{"World"}}
	message := GenerateReportMessage{
		ReportID:   uuid.New(),
		TemplateID: uuid.New(),
		DataQueries: map[string]map[string][]string{
			"onboarding": {"organization": {"name"}},
		},
		Filters: map[string]map[string]map[string]model.FilterCondition{
			"onboarding": {"organization": {"name": condition}},
		},
	}

	result := make(map[string]map[string][]map[string]any)
	require.NoError(t, uc.extractEngineDatasource(context.Background(), message, "onboarding",
		message.DataQueries["onboarding"], result))

	// The connector received the filter payload, keyed by the QUALIFIED table the
	// bare template key resolved to, with the leaf still a model.FilterCondition.
	require.NotNil(t, connector.captured, "QueryStream must have received filters")
	dsFilters, ok := connector.captured["onboarding"].(map[string]any)
	require.True(t, ok)
	tableFilters, ok := dsFilters["public.organization"].(map[string]any)
	require.True(t, ok, "filter must be keyed by the qualified table the bare key resolved to")
	got, ok := tableFilters["name"].(model.FilterCondition)
	require.True(t, ok, "leaf must survive the planner round-trip as model.FilterCondition")
	assert.Equal(t, condition, got)
}
