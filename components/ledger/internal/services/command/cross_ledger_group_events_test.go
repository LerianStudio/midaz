// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

var (
	groupEventOrganization = uuid.MustParse("0199a700-0000-7000-8000-000000000001")
	groupEventLedgerA      = uuid.MustParse("0199a700-0000-7000-8000-000000000002")
	groupEventLedgerB      = uuid.MustParse("0199a700-0000-7000-8000-000000000003")
	groupEventGroupID      = uuid.MustParse("0199a700-0000-7000-8000-000000000004")
)

func crossLedgerGroupEventMember(ledgerID, groupID uuid.UUID, status string, source, destination []string) *transaction.Transaction {
	group := groupID.String()
	amount := decimal.NewFromInt(100)

	return &transaction.Transaction{
		ID:             uuid.NewString(),
		OrganizationID: groupEventOrganization.String(),
		LedgerID:       ledgerID.String(),
		GroupID:        &group,
		Status:         transaction.Status{Code: status, Description: &status},
		AssetCode:      "BRL",
		Amount:         &amount,
		Source:         source,
		Destination:    destination,
		CreatedAt:      time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC),
	}
}

func crossLedgerGroupEventInput() CreateCrossLedgerTransactionV2Input {
	return CreateCrossLedgerTransactionV2Input{
		Transaction: crossLedgerTestTransaction("100",
			[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "100", true)},
			[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "100", false)}),
		Scopes: CrossLedgerTransactionScopes{
			Debits:  []CrossLedgerLegScope{{OrganizationID: groupEventOrganization, LedgerID: groupEventLedgerA}},
			Credits: []CrossLedgerLegScope{{OrganizationID: groupEventOrganization, LedgerID: groupEventLedgerB}},
		},
		IdempotencyKey: "group-event-key",
		IdempotencyTTL: time.Minute,
	}
}

func groupLifecycleEvents(emitter *pkgStreaming.MockEmitter) []libStreaming.EmitRequest {
	result := make([]libStreaming.EmitRequest, 0)

	for _, event := range emitter.Events() {
		if strings.HasPrefix(event.DefinitionKey, "transaction_group.") {
			result = append(result, event)
		}
	}

	return result
}

func requireOneGroupEvent(t *testing.T, emitter *pkgStreaming.MockEmitter, key string) events.TransactionGroupPayload {
	t.Helper()

	require.Eventually(t, func() bool { return len(groupLifecycleEvents(emitter)) > 0 }, 2*time.Second, 5*time.Millisecond,
		"the group operation must publish its group event")

	emitted := groupLifecycleEvents(emitter)
	require.Len(t, emitted, 1, "exactly one group event per group operation")
	assert.Equal(t, key, emitted[0].DefinitionKey)

	var payload events.TransactionGroupPayload
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, payload.GroupID, emitted[0].Subject, "ce-subject is the group id")

	return payload
}

func requireNoGroupEvent(t *testing.T, emitter *pkgStreaming.MockEmitter) {
	t.Helper()

	require.Never(t, func() bool { return len(groupLifecycleEvents(emitter)) > 0 }, 150*time.Millisecond, 5*time.Millisecond,
		"no group event may be published")
}

func spanAttribute(s sdktrace.ReadOnlySpan, key string) (attribute.Value, bool) {
	for _, candidate := range s.Attributes() {
		if string(candidate.Key) == key {
			return candidate.Value, true
		}
	}

	return attribute.Value{}, false
}

func requireSpanAttributes(t *testing.T, s sdktrace.ReadOnlySpan, want map[string]any) {
	t.Helper()

	for key, expected := range want {
		value, ok := spanAttribute(s, key)
		require.Truef(t, ok, "span %q must carry %q", s.Name(), key)

		switch typed := expected.(type) {
		case string:
			assert.Equalf(t, typed, value.AsString(), "span attribute %q", key)
		case int:
			assert.Equalf(t, int64(typed), value.AsInt64(), "span attribute %q", key)
		}
	}
}

func collectGroupLedgerHistogram(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.HistogramDataPoint[int64] {
	t.Helper()

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))

	result := make(map[string]metricdata.HistogramDataPoint[int64])

	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "cross_ledger_group_ledgers" {
				continue
			}

			histogram, ok := metric.Data.(metricdata.Histogram[int64])
			require.True(t, ok)

			for _, point := range histogram.DataPoints {
				require.Equal(t, 1, point.Attributes.Len(), "cross_ledger_group_ledgers carries only the action label")
				action, _ := point.Attributes.Value("action")
				result[action.AsString()] = point
			}
		}
	}

	return result
}

func TestCreateCrossLedgerTransactionV2_PublishesOneGroupPostedEvent(t *testing.T) {
	emitter := pkgStreaming.NewMockEmitter()
	reader, factory := newReaderFactory(t)
	ctx, recorder := recordingContext()
	origin := crossLedgerGroupEventMember(groupEventLedgerA, groupEventGroupID, constant.APPROVED, []string{"@debit"}, []string{"@external/BRL"})
	destination := crossLedgerGroupEventMember(groupEventLedgerB, groupEventGroupID, constant.APPROVED, []string{"@external/BRL"}, []string{"@credit"})

	uc := &UseCase{
		Streaming:       emitter,
		MetricsFactory:  factory,
		UUIDv7Generator: func() (uuid.UUID, error) { return groupEventGroupID, nil },
		createAtomicTransactionBatchV2: func(_ context.Context, batch CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
			require.True(t, batch.CrossLedgerGroup)
			return &CreateAtomicTransactionBatchV2Result{BatchID: groupEventGroupID, Transactions: []*transaction.Transaction{origin, destination}}, nil
		},
	}

	result, err := uc.CreateCrossLedgerTransactionV2(ctx, crossLedgerGroupEventInput())
	require.NoError(t, err)
	require.Len(t, result.Transactions, 2)

	payload := requireOneGroupEvent(t, emitter, events.TransactionGroupPostedDefinition.Key())
	assert.Equal(t, groupEventGroupID.String(), payload.GroupID)
	assert.Nil(t, payload.RevertedGroupID)
	assert.Equal(t, constant.APPROVED, payload.Status)
	assert.Equal(t, "BRL", payload.AssetCode)
	assert.Equal(t, 2, payload.LedgerCount)
	require.Len(t, payload.Parts, 2)
	assert.Equal(t, events.TransactionGroupPart{
		TransactionID: origin.ID, OrganizationID: origin.OrganizationID, LedgerID: origin.LedgerID,
		Role: events.TransactionGroupRoleOrigin, Status: constant.APPROVED,
	}, payload.Parts[0])
	assert.Equal(t, events.TransactionGroupPart{
		TransactionID: destination.ID, OrganizationID: destination.OrganizationID, LedgerID: destination.LedgerID,
		Role: events.TransactionGroupRoleDestination, Status: constant.APPROVED,
	}, payload.Parts[1])

	span := findSpan(t, recorder, "command.create_cross_ledger_transaction_v2")
	requireSpanAttributes(t, span, map[string]any{
		"app.request.action":       constant.ActionDirect,
		"app.request.part_count":   2,
		"app.request.ledger_count": 2,
		"app.response.group_id":    groupEventGroupID.String(),
	})

	totals := collectDomainCounters(t, reader)
	assert.Equal(t, int64(1), totals["ledger/create_cross_ledger_transaction/success"])

	ledgers := collectGroupLedgerHistogram(t, reader)
	require.Contains(t, ledgers, constant.ActionDirect)
	assert.Equal(t, uint64(1), ledgers[constant.ActionDirect].Count)
	assert.Equal(t, int64(2), ledgers[constant.ActionDirect].Sum)
}

func TestCreateCrossLedgerTransactionV2_ReplayPublishesNoGroupEvent(t *testing.T) {
	emitter := pkgStreaming.NewMockEmitter()
	origin := crossLedgerGroupEventMember(groupEventLedgerA, groupEventGroupID, constant.APPROVED, []string{"@debit"}, []string{"@external/BRL"})
	destination := crossLedgerGroupEventMember(groupEventLedgerB, groupEventGroupID, constant.APPROVED, []string{"@external/BRL"}, []string{"@credit"})

	uc := &UseCase{
		Streaming:       emitter,
		UUIDv7Generator: func() (uuid.UUID, error) { return groupEventGroupID, nil },
		createAtomicTransactionBatchV2: func(context.Context, CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
			return &CreateAtomicTransactionBatchV2Result{
				BatchID: groupEventGroupID, Transactions: []*transaction.Transaction{origin, destination}, Replayed: true,
			}, nil
		},
	}

	_, err := uc.CreateCrossLedgerTransactionV2(context.Background(), crossLedgerGroupEventInput())
	require.NoError(t, err)
	requireNoGroupEvent(t, emitter)
}

func TestCreateCrossLedgerTransactionV2_RejectionPublishesNothingAndCountsBusinessError(t *testing.T) {
	emitter := pkgStreaming.NewMockEmitter()
	reader, factory := newReaderFactory(t)
	ctx, recorder := recordingContext()
	executed := false
	uc := &UseCase{
		Streaming:       emitter,
		MetricsFactory:  factory,
		UUIDv7Generator: func() (uuid.UUID, error) { return groupEventGroupID, nil },
		createAtomicTransactionBatchV2: func(context.Context, CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
			executed = true
			return nil, nil
		},
	}

	input := crossLedgerGroupEventInput()
	input.Scopes.Credits = nil

	_, err := uc.CreateCrossLedgerTransactionV2(ctx, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrTransactionScopeMismatch.Error())
	assert.False(t, executed, "a rejected decomposition never reaches the batch")
	requireNoGroupEvent(t, emitter)

	span := findSpan(t, recorder, "command.create_cross_ledger_transaction_v2")
	assert.NotEqual(t, "Error", span.Status().Code.String(), "a business rejection keeps the span green")

	totals := collectDomainCounters(t, reader)
	assert.Equal(t, int64(1), totals["ledger/create_cross_ledger_transaction/business_error"])
}

func TestCreateCrossLedgerHoldV2_PublishesNoGroupEventButMeasuresTheGroup(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	emitter := pkgStreaming.NewMockEmitter()
	reader, factory := newReaderFactory(t)
	ctx, recorder := recordingContext()
	settings := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	origin := crossLedgerGroupEventMember(groupEventLedgerA, groupEventGroupID, constant.PENDING, []string{"@debit"}, []string{"@external/BRL"})

	uc := &UseCase{
		Streaming:            emitter,
		MetricsFactory:       factory,
		TransactionGroupRepo: repo,
		TransactionReader: &atomicTransactionBatchSettingsReader{settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
			{organizationID: groupEventOrganization, ledgerID: groupEventLedgerA}: settings,
			{organizationID: groupEventOrganization, ledgerID: groupEventLedgerB}: settings,
		}},
		UUIDv7Generator: func() (uuid.UUID, error) { return groupEventGroupID, nil },
		Clock:           func() time.Time { return time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC) },
		createAtomicTransactionBatchV2: func(context.Context, CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
			return &CreateAtomicTransactionBatchV2Result{BatchID: groupEventGroupID, Transactions: []*transaction.Transaction{origin}}, nil
		},
	}
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	_, err := uc.CreateCrossLedgerHoldV2(ctx, crossLedgerGroupEventInput())
	require.NoError(t, err)
	requireNoGroupEvent(t, emitter)

	span := findSpan(t, recorder, "command.create_cross_ledger_hold_v2")
	requireSpanAttributes(t, span, map[string]any{
		"app.request.action":       constant.ActionHold,
		"app.request.part_count":   2,
		"app.request.ledger_count": 2,
		"app.response.group_id":    groupEventGroupID.String(),
	})

	totals := collectDomainCounters(t, reader)
	assert.Equal(t, int64(1), totals["ledger/create_cross_ledger_hold/success"])

	ledgers := collectGroupLedgerHistogram(t, reader)
	require.Contains(t, ledgers, constant.ActionHold)
	assert.Equal(t, int64(2), ledgers[constant.ActionHold].Sum)
}

func TestTransitionCrossLedgerGroupV2_PublishesTheTerminalGroupEvent(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    string
		key       string
		operation string
		action    string
		parts     int
	}{
		{name: "commit", status: constant.APPROVED, key: events.TransactionGroupCommittedDefinition.Key(), operation: "commit_cross_ledger_group", action: constant.ActionCommit, parts: 2},
		{name: "cancel", status: constant.CANCELED, key: events.TransactionGroupCanceledDefinition.Key(), operation: "cancel_cross_ledger_group", action: constant.ActionCancel, parts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			uc, repo, _, target, in, group := newCrossLedgerLifecycleFixture(t, test.status)
			emitter := pkgStreaming.NewMockEmitter()
			reader, factory := newReaderFactory(t)
			uc.Streaming = emitter
			uc.MetricsFactory = factory
			ctx, recorder := recordingContext()

			repo.EXPECT().FindByID(gomock.Any(), group.ID).Return(group, nil)
			repo.EXPECT().UpdateStatus(gomock.Any(), group.ID, constant.PENDING, test.status).Return(true, nil)

			result, err := uc.transitionCrossLedgerGroupV2(ctx, in, target, test.status)
			require.NoError(t, err)

			payload := requireOneGroupEvent(t, emitter, test.key)
			assert.Equal(t, group.ID.String(), payload.GroupID)
			assert.Equal(t, test.status, payload.Status)
			assert.Equal(t, "BRL", payload.AssetCode)
			require.Len(t, payload.Parts, test.parts)
			assert.Equal(t, result.Transactions[0].ID, payload.Parts[0].TransactionID)
			assert.Equal(t, events.TransactionGroupRoleOrigin, payload.Parts[0].Role)
			assert.Equal(t, test.status, payload.Parts[0].Status)

			if test.parts == 2 {
				assert.Equal(t, events.TransactionGroupRoleDestination, payload.Parts[1].Role)
				assert.Equal(t, 2, payload.LedgerCount)
			}

			span := findSpan(t, recorder, "command.transition_cross_ledger_group_v2")
			requireSpanAttributes(t, span, map[string]any{
				"app.request.action":       test.action,
				"app.request.group_id":     group.ID.String(),
				"app.request.part_count":   2,
				"app.request.ledger_count": 2,
			})

			totals := collectDomainCounters(t, reader)
			assert.Equal(t, int64(1), totals["ledger/"+test.operation+"/success"])

			ledgers := collectGroupLedgerHistogram(t, reader)
			require.Contains(t, ledgers, test.action)
			assert.Equal(t, int64(2), ledgers[test.action].Sum)
		})
	}
}

func TestTransitionCrossLedgerGroupV2_AnotherWriterAlignedTheGroupFirst(t *testing.T) {
	uc, repo, _, target, in, group := newCrossLedgerLifecycleFixture(t, constant.APPROVED)
	emitter := pkgStreaming.NewMockEmitter()
	uc.Streaming = emitter

	aligned := *group
	aligned.Status = constant.APPROVED

	gomock.InOrder(
		repo.EXPECT().FindByID(gomock.Any(), group.ID).Return(group, nil),
		repo.EXPECT().UpdateStatus(gomock.Any(), group.ID, constant.PENDING, constant.APPROVED).Return(false, nil),
		repo.EXPECT().FindByID(gomock.Any(), group.ID).Return(&aligned, nil),
	)

	result, err := uc.transitionCrossLedgerGroupV2(context.Background(), in, target, constant.APPROVED)
	require.NoError(t, err, "the movement was applied; a reconciler that recorded the same status first is not a failure")
	require.Len(t, result.Transactions, 2)
	requireNoGroupEvent(t, emitter)
}

func TestTransitionCrossLedgerGroupV2_TerminalGroupCountsBusinessErrorPerAction(t *testing.T) {
	for _, test := range []struct {
		status    string
		operation string
	}{
		{status: constant.APPROVED, operation: "commit_cross_ledger_group"},
		{status: constant.CANCELED, operation: "cancel_cross_ledger_group"},
	} {
		t.Run(test.operation, func(t *testing.T) {
			groupID := uuid.MustParse("0199a700-0000-7000-8000-000000000010")
			target := pendingTransaction(false)
			groupText := groupID.String()
			target.GroupID = &groupText

			ctrl := gomock.NewController(t)
			repo := transactiongroup.NewMockRepository(ctrl)
			repo.EXPECT().FindByID(gomock.Any(), groupID).Return(&transactiongroup.TransactionGroup{
				ID: groupID, Status: test.status,
			}, nil)

			reader, factory := newReaderFactory(t)
			uc := &UseCase{TransactionGroupRepo: repo, MetricsFactory: factory}

			_, err := uc.transitionCrossLedgerGroupV2(context.Background(), pendingTransitionInputFor(target), target, test.status)
			require.Error(t, err)

			totals := collectDomainCounters(t, reader)
			assert.Equal(t, int64(1), totals["ledger/"+test.operation+"/business_error"])
		})
	}
}

func TestRevertCrossLedgerGroupV2_IncompleteGroupCountsBusinessError(t *testing.T) {
	groupID := uuid.NewString()
	origin := revertibleOrigin()
	origin.GroupID = &groupID
	reader := &revertReader{
		byID:         origin,
		groupMembers: []*transaction.Transaction{origin},
	}
	uc := newRevertUseCase(t, reader)
	metricsReader, factory := newReaderFactory(t)
	uc.MetricsFactory = factory
	ctx, recorder := recordingContext()

	_, _, err := uc.RevertCrossLedgerGroupV2(ctx, RevertTransactionInput{
		OrganizationID: uuid.MustParse(origin.OrganizationID),
		LedgerID:       uuid.MustParse(origin.LedgerID),
		TransactionID:  uuid.MustParse(origin.ID),
	})
	require.Error(t, err)

	span := findSpan(t, recorder, "command.revert_cross_ledger_group")
	requireSpanAttributes(t, span, map[string]any{
		"app.request.action":   constant.ActionRevert,
		"app.request.group_id": groupID,
	})

	totals := collectDomainCounters(t, metricsReader)
	assert.Equal(t, int64(1), totals["ledger/revert_cross_ledger_group/business_error"])
}

func TestPublishTransactionGroupEvent_RevertedCarriesBothGroupsAndReversedRoles(t *testing.T) {
	emitter := pkgStreaming.NewMockEmitter()
	uc := &UseCase{Streaming: emitter}
	revertedGroupID := uuid.MustParse("0199a700-0000-7000-8000-000000000020")
	newGroupID := uuid.MustParse("0199a700-0000-7000-8000-000000000021")
	destinationReversal := crossLedgerGroupEventMember(groupEventLedgerB, newGroupID, constant.APPROVED, []string{"@credit"}, []string{"@external/BRL"})
	originReversal := crossLedgerGroupEventMember(groupEventLedgerA, newGroupID, constant.APPROVED, []string{"@external/BRL"}, []string{"@debit"})

	uc.publishTransactionGroupEvent(context.Background(), transactionGroupEventReverted, newGroupID, &revertedGroupID,
		[]*transaction.Transaction{destinationReversal, originReversal})

	payload := requireOneGroupEvent(t, emitter, events.TransactionGroupRevertedDefinition.Key())
	assert.Equal(t, newGroupID.String(), payload.GroupID)
	require.NotNil(t, payload.RevertedGroupID)
	assert.Equal(t, revertedGroupID.String(), *payload.RevertedGroupID)
	require.Len(t, payload.Parts, 2)
	assert.Equal(t, events.TransactionGroupRoleOrigin, payload.Parts[0].Role, "the reversal debits the former destination")
	assert.Equal(t, events.TransactionGroupRoleDestination, payload.Parts[1].Role, "the reversal credits the former origin")
}

func TestSendTransactionEvents_GroupMemberCarriesItsRole(t *testing.T) {
	for _, test := range []struct {
		name        string
		source      []string
		destination []string
		wantRole    string
	}{
		{name: "origin", source: []string{"@debit"}, destination: []string{"@external/BRL"}, wantRole: events.TransactionGroupRoleOrigin},
		{name: "destination", source: []string{"@external/BRL"}, destination: []string{"@credit"}, wantRole: events.TransactionGroupRoleDestination},
		{name: "net-zero participant", source: []string{"@debit"}, destination: []string{"@credit"}, wantRole: events.TransactionGroupRoleOrigin},
	} {
		t.Run(test.name, func(t *testing.T) {
			emitter := pkgStreaming.NewMockEmitter()
			uc := &UseCase{Streaming: emitter}
			member := crossLedgerGroupEventMember(groupEventLedgerA, groupEventGroupID, constant.APPROVED, test.source, test.destination)

			uc.SendTransactionEvents(context.Background(), member, TransactionLifecyclePhaseCreated)

			emitted := emitter.Events()
			require.Len(t, emitted, 1)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
			assert.Equal(t, groupEventGroupID.String(), payload["groupId"])
			assert.Equal(t, test.wantRole, payload["groupRole"])
		})
	}

	t.Run("singular transaction has no role", func(t *testing.T) {
		emitter := pkgStreaming.NewMockEmitter()
		uc := &UseCase{Streaming: emitter}
		member := crossLedgerGroupEventMember(groupEventLedgerA, groupEventGroupID, constant.APPROVED, []string{"@debit"}, []string{"@external/BRL"})
		member.GroupID = nil

		uc.SendTransactionEvents(context.Background(), member, TransactionLifecyclePhaseCreated)

		emitted := emitter.Events()
		require.Len(t, emitted, 1)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
		assert.NotContains(t, payload, "groupRole")
	})
}
