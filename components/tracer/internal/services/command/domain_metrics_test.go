// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libObservability "github.com/LerianStudio/lib-observability"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/observability"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// findDomainOpCounter returns the value of the domain_operations_total series
// matching (component, operation, result). Returns -1 when the series is not
// present so a missing emission is distinguishable from an emitted zero.
func findDomainOpCounter(mf *dto.MetricFamily, component, operation, result string) float64 {
	for _, m := range mf.GetMetric() {
		labels := map[string]string{}
		for _, l := range m.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}

		if labels["component"] == component &&
			labels["operation"] == operation &&
			labels["result"] == result {
			return m.GetCounter().GetValue()
		}
	}

	return -1
}

// TestCreateRule_EmitsDomainMetrics asserts that a successful command emits the
// D6 domain metrics (domain_operations_total + domain_operation_duration_ms)
// through a real MetricsFactory. The factory is injected into the request
// context exactly as the telemetry middleware does in production, and scraped
// via a fresh per-test Prometheus registry bridged through the OTel exporter
// (the same NewPrometheusBackedFactory path the readyz recorder uses).
func TestCreateRule_EmitsDomainMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()

	factory, shutdown, err := observability.NewPrometheusBackedFactory(reg, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = shutdown()
	})

	ctrl := gomock.NewController(t)

	mockRepo := NewMockRuleRepository(ctrl)
	mockCEL := NewMockExpressionCompiler(ctrl)
	auditWriter := NewMockAuditWriter(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	mockCEL.EXPECT().
		Compile(gomock.Any(), "amount > 1000000").
		Return(nil, nil)

	expectRuleCreateTxSuccess(
		t,
		txBeginner, mockTx,
		mockRepo, auditWriter,
		model.AuditEventRuleCreated,
		model.AuditActionCreate,
		"Rule created via API",
	)

	cmd, err := NewCreateRuleCommand(mockRepo, mockCEL, testutil.NewDefaultMockClock(), auditWriter, txBeginner)
	require.NoError(t, err)

	// Inject the factory into the context the way the telemetry middleware
	// does — the use case reads it via libObservability.NewTrackingFromContext.
	ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

	input := &CreateRuleInput{
		Name:       "High Value Transaction Rule",
		Expression: "amount > 1000000",
		Action:     model.DecisionDeny,
	}

	result, err := cmd.Execute(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result)

	families, err := reg.Gather()
	require.NoError(t, err)

	byName := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		byName[f.GetName()] = f
	}

	countMF, ok := byName["domain_operations_total"]
	require.True(t, ok, "domain_operations_total family must be emitted")
	assert.Equal(t, float64(1),
		findDomainOpCounter(countMF, "tracer", "rule_create", "success"),
		"rule_create success counter must increment exactly once")

	durMF, ok := byName["domain_operation_duration_ms"]
	require.True(t, ok, "domain_operation_duration_ms family must be emitted")

	var sampled bool

	for _, m := range durMF.GetMetric() {
		labels := map[string]string{}
		for _, l := range m.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}

		if labels["component"] == "tracer" && labels["operation"] == "rule_create" {
			assert.Greater(t, m.GetHistogram().GetSampleCount(), uint64(0),
				"rule_create duration histogram must record at least one sample")

			sampled = true
		}
	}

	assert.True(t, sampled, "rule_create duration histogram series must be present")
}
