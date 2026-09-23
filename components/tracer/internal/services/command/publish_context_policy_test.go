// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func publicationCompiler(t *testing.T) *query.ContextPolicyEvaluator {
	t.Helper()
	engine, err := cel.NewContextAdapter(cel.ContextAdapterConfig{
		Limits:    tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128},
		CostLimit: 100000, MaxExpressionBytes: 5000,
	})
	require.NoError(t, err)
	compiler, err := query.NewContextPolicyEvaluator(engine, query.ContextPolicyConfig{MaxRules: 10, TotalCost: 100000})
	require.NoError(t, err)
	return compiler
}

func publicationPolicy() model.ContextPolicy {
	return model.ContextPolicy{
		ID: testutil.MustDeterministicUUID(62001), Revision: 2, DefaultDecision: model.DecisionDeny,
		Rules: []model.ContextPolicyRule{{ID: testutil.MustDeterministicUUID(62002), Revision: 3, Expression: "true", Action: model.DecisionReview}},
	}
}

func publicationContext() context.Context {
	return contextutil.WithPrincipal(context.Background(), contextutil.Principal{ID: "operator-1", Type: "user", Name: "Operator"})
}

func TestPublishContextPolicyAtomicity(t *testing.T) {
	t.Parallel()
	failure := errors.New("storage failure")
	for _, stage := range []string{"success", "publish", "audit", "commit", "begin", "conflict"} {
		t.Run(stage, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := mocks.NewMockContextPolicyPublisher(ctrl)
			audit := mocks.NewMockAuditEventRepository(ctrl)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			tx := dbmocks.NewMockTx(ctrl)
			c, err := NewPublishContextPolicyCommand(repo, audit, beginner, publicationCompiler(t), clock.NewFixedClock(testutil.FixedTime()))
			require.NoError(t, err)
			policy := publicationPolicy()
			want := failure
			if stage == "conflict" {
				want = constant.ErrContextPolicyConflict
			}
			if stage == "begin" {
				beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, failure)
			} else {
				calls := []any{beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil)}
				publishErr := error(nil)
				if stage == "publish" || stage == "conflict" {
					publishErr = want
				}
				calls = append(calls, repo.EXPECT().PublishWithTx(gomock.Any(), tx, policy, "operator-1", testutil.FixedTime()).Return(publishErr))
				if publishErr == nil {
					auditErr := error(nil)
					if stage == "audit" {
						auditErr = failure
					}
					calls = append(calls, audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.DB, event *model.AuditEvent) error {
						require.Equal(t, model.AuditEventPolicyPublished, event.EventType)
						require.Equal(t, model.ResourceTypePolicy, event.ResourceType)
						require.Equal(t, model.AuditActionCreate, event.Action)
						require.Equal(t, model.AuditResultSuccess, event.Result)
						require.Equal(t, policy.ID.String(), event.ResourceID)
						require.Equal(t, "operator-1", event.Actor.ID)
						require.Equal(t, model.ActorTypeUser, event.Actor.ActorType)
						require.Equal(t, testutil.FixedTime(), event.CreatedAt)
						require.Equal(t, policy, event.Context["policy"])
						return auditErr
					}))
				}
				if stage == "success" || stage == "commit" {
					commitErr := error(nil)
					if stage == "commit" {
						commitErr = failure
					}
					calls = append(calls, tx.EXPECT().Commit().Return(commitErr))
				}
				if stage != "success" {
					calls = append(calls, tx.EXPECT().Rollback().Return(nil))
				}
				gomock.InOrder(calls...)
			}
			err = c.Execute(publicationContext(), policy)
			if stage == "success" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, want)
			}
		})
	}
}

func TestPublishContextPolicyRejectsBeforeTransaction(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"absent principal", "empty principal", "invalid actor", "invalid expression", "invalid default", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			c, err := NewPublishContextPolicyCommand(mocks.NewMockContextPolicyPublisher(ctrl), mocks.NewMockAuditEventRepository(ctrl), dbmocks.NewMockTxBeginner(ctrl), publicationCompiler(t), clock.NewFixedClock(testutil.FixedTime()))
			require.NoError(t, err)
			ctx := publicationContext()
			policy := publicationPolicy()
			switch scenario {
			case "absent principal":
				ctx = context.Background()
			case "empty principal":
				ctx = contextutil.WithPrincipal(ctx, contextutil.Principal{Type: "user", ID: " "})
			case "invalid actor":
				ctx = contextutil.WithPrincipal(ctx, contextutil.Principal{Type: "unknown", ID: "operator"})
			case "invalid expression":
				policy.Rules[0].Expression = `decimal("0.1") + decimal("0.2") == decimal("0.3")`
			case "invalid default":
				policy.DefaultDecision = model.DecisionReview
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			require.Error(t, c.Execute(ctx, policy))
		})
	}
}

func TestPublishContextPolicyRequiresDependencies(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockContextPolicyPublisher(ctrl)
	audit := mocks.NewMockAuditEventRepository(ctrl)
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	compiler := publicationCompiler(t)
	clk := clock.NewFixedClock(testutil.FixedTime())
	for _, omitted := range []string{"repo", "audit", "transaction", "compiler", "clock"} {
		t.Run(omitted, func(t *testing.T) {
			var r ContextPolicyPublisher = repo
			var a AuditEventRepository = audit
			var b pgdb.TxBeginner = beginner
			var c ContextPolicyCompiler = compiler
			var k clock.Clock = clk
			switch omitted {
			case "repo":
				r = nil
			case "audit":
				a = nil
			case "transaction":
				b = nil
			case "compiler":
				c = nil
			case "clock":
				k = nil
			}
			command, err := NewPublishContextPolicyCommand(r, a, b, c, k)
			require.Error(t, err)
			require.Nil(t, command)
		})
	}
}

// A repository must not be able to change the caller's policy or the audited
// snapshot after compilation; each recipient owns a detached rule slice.
func TestPublishContextPolicyOwnsSnapshot(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockContextPolicyPublisher(ctrl)
	audit := mocks.NewMockAuditEventRepository(ctrl)
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	tx := dbmocks.NewMockTx(ctrl)
	c, err := NewPublishContextPolicyCommand(repo, audit, beginner, publicationCompiler(t), clock.NewFixedClock(testutil.FixedTime()))
	require.NoError(t, err)
	policy := publicationPolicy()
	beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil)
	repo.EXPECT().PublishWithTx(gomock.Any(), tx, policy, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.Tx, stored model.ContextPolicy, _ string, _ time.Time) error {
		stored.Rules[0].Expression = "false"
		return nil
	})
	audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.DB, event *model.AuditEvent) error {
		require.Equal(t, publicationPolicy(), event.Context["policy"])
		return nil
	})
	tx.EXPECT().Commit().Return(nil)
	require.NoError(t, c.Execute(publicationContext(), policy))
	require.Equal(t, publicationPolicy(), policy)
}
