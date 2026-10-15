// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompareAndDeleteBackupRecordRemovesAnEmptyRecord removes a backup field
// whose recorded value is empty. Such a record is quarantined as unparseable;
// refusing to compare its empty value would keep it in the backup hash and
// re-quarantine it every cycle.
func TestCompareAndDeleteBackupRecordRemovesAnEmptyRecord(t *testing.T) {
	t.Parallel()

	const field = "transaction:{transactions}:org:ledger:tx"

	var capturedArgs []any

	client := &deleteIfValueEvalClient{
		evalFunc: func(ctx context.Context, _ string, _ []string, args ...any) *redis.Cmd {
			capturedArgs = append([]any(nil), args...)

			cmd := redis.NewCmd(ctx)
			cmd.SetVal(RecoveryAckDeleted)

			return cmd
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(client)}

	outcome, err := repo.CompareAndDeleteBackupRecord(context.Background(), field, "")

	require.NoError(t, err)
	assert.Equal(t, RecoveryAckDeleted, outcome)
	assert.Equal(t, []any{field, "", "", "0"}, capturedArgs, "the empty value is compared as recorded, without clearing the attempt counter")
}

func TestCompareAndDeleteBackupRecordRejectsAnEmptyField(t *testing.T) {
	t.Parallel()

	client := &deleteIfValueEvalClient{
		evalFunc: func(context.Context, string, []string, ...any) *redis.Cmd {
			t.Fatal("an empty field must be refused before Redis")

			return nil
		},
	}
	repo := &RedisConsumerRepository{conn: newDeleteIfValueConnection(client)}

	_, err := repo.CompareAndDeleteBackupRecord(context.Background(), "", `{"raw":true}`)

	require.Error(t, err)
}
