// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestBindContextPolicyAtomicity(t *testing.T) {
	t.Parallel()
	failure := errors.New("storage failure")
	for _, scenario := range []string{"create", "replace", "stale", "missing", "already exists", "read failure", "write failure", "audit failure", "commit failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := mocks.NewMockContextPolicyBindingRepository(ctrl)
			audit := mocks.NewMockAuditEventRepository(ctrl)
			tx := dbmocks.NewMockTx(ctrl)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			c, err := NewBindContextPolicyCommand(repo, audit, beginner, publicationCompiler(t), clock.NewFixedClock(testutil.FixedTime()))
			require.NoError(t, err)
			policy := publicationPolicy()
			target := model.PolicyRevision{ID: policy.ID, Revision: policy.Revision}
			key := model.PolicyBindingKey{IntegrationID: "producer", ContextID: "context-1"}
			before := &model.PolicyBindingState{Policy: model.PolicyRevision{ID: policy.ID, Revision: 1}, Version: 7}
			expectedVersion := int64(7)
			expected := &expectedVersion
			switch scenario {
			case "create":
				before = nil
				expected = nil
			case "stale":
				expectedVersion = 6
			case "missing":
				before = nil
			case "already exists":
				expected = nil
			}
			readErr := error(nil)
			if scenario == "read failure" {
				readErr = failure
			}
			calls := []any{
				repo.EXPECT().GetRevision(gomock.Any(), target).Return(&policy, nil),
				beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil),
				repo.EXPECT().GetBindingWithTx(gomock.Any(), tx, key).Return(before, readErr),
			}
			want := error(nil)
			switch scenario {
			case "stale", "missing", "already exists":
				want = constant.ErrContextPolicyConflict
			case "read failure", "write failure", "audit failure", "commit failure":
				want = failure
			}
			if scenario == "create" || scenario == "replace" || scenario == "write failure" || scenario == "audit failure" || scenario == "commit failure" {
				writeErr := error(nil)
				if scenario == "write failure" {
					writeErr = failure
				}
				calls = append(calls, repo.EXPECT().BindWithTx(gomock.Any(), tx, key, target, expected, "operator-1", testutil.FixedTime()).Return(writeErr))
				if writeErr == nil {
					calls = append(calls, audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.DB, event *model.AuditEvent) error {
						require.Equal(t, model.AuditEventPolicyBound, event.EventType)
						require.Equal(t, model.AuditActionActivate, event.Action)
						require.Equal(t, model.ResourceTypePolicy, event.ResourceType)
						require.Equal(t, target.ID.String(), event.ResourceID)
						require.Equal(t, "operator-1", event.Actor.ID)
						require.Equal(t, testutil.FixedTime(), event.CreatedAt)
						require.Equal(t, key, event.Context["binding"])
						if before == nil {
							require.NotContains(t, event.Context, "before")
							require.Equal(t, model.PolicyBindingState{Policy: target, Version: 1}, event.Context["after"])
						} else {
							require.Equal(t, *before, event.Context["before"])
							require.Equal(t, model.PolicyBindingState{Policy: target, Version: 8}, event.Context["after"])
						}
						if scenario == "audit failure" {
							return failure
						}
						return nil
					}))
				}
			}
			if want == nil || scenario == "commit failure" {
				calls = append(calls, tx.EXPECT().Commit().Return(want))
			}
			if want != nil {
				calls = append(calls, tx.EXPECT().Rollback().Return(nil))
			}
			gomock.InOrder(calls...)
			result, err := c.Execute(publicationContext(), key, target, expected)
			if want != nil {
				require.ErrorIs(t, err, want)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, target, result.Policy)
				if scenario == "create" {
					require.EqualValues(t, 1, result.Version)
				} else {
					require.EqualValues(t, 8, result.Version)
				}
			}
		})
	}
}

func TestBindContextPolicyRejectsBeforeTransaction(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"identity", "key", "target", "version", "overflow", "cancelled", "unpublished", "invalid CEL", "wrong revision", "nil revision"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := mocks.NewMockContextPolicyBindingRepository(ctrl)
			c, err := NewBindContextPolicyCommand(repo, mocks.NewMockAuditEventRepository(ctrl), dbmocks.NewMockTxBeginner(ctrl), publicationCompiler(t), clock.NewFixedClock(testutil.FixedTime()))
			require.NoError(t, err)
			ctx := publicationContext()
			key := model.PolicyBindingKey{IntegrationID: "producer", ContextID: "context-1"}
			policy := publicationPolicy()
			target := model.PolicyRevision{ID: policy.ID, Revision: policy.Revision}
			var expected *int64
			switch scenario {
			case "identity":
				ctx = context.Background()
			case "key":
				key.ContextID = ""
			case "target":
				target.Revision = 0
			case "version", "overflow":
				version := int64(0)
				if scenario == "overflow" {
					version = math.MaxInt64
				}
				expected = &version
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			case "unpublished":
				repo.EXPECT().GetRevision(gomock.Any(), target).Return(nil, constant.ErrContextPolicyUnavailable)
			case "invalid CEL":
				policy.Rules[0].Expression = "unknown.value"
				repo.EXPECT().GetRevision(gomock.Any(), target).Return(&policy, nil)
			case "wrong revision":
				policy.Revision++
				repo.EXPECT().GetRevision(gomock.Any(), target).Return(&policy, nil)
			case "nil revision":
				repo.EXPECT().GetRevision(gomock.Any(), target).Return(nil, nil)
			}
			result, err := c.Execute(ctx, key, target, expected)
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}
