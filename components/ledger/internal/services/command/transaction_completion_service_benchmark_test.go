// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type finalizationBenchmarkStore struct{}

func (*finalizationBenchmarkStore) Persist(context.Context, TransactionWriteSet) error {
	return nil
}

func (*finalizationBenchmarkStore) PersistWithOutcome(context.Context, TransactionWriteSet) (TransactionPersistenceOutcome, error) {
	return TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}, nil
}

type finalizationBenchmarkMetadata struct {
	records map[string]*mongodb.Metadata
}

func (repo *finalizationBenchmarkMetadata) Create(_ context.Context, collection string, metadata *mongodb.Metadata) error {
	repo.records[collection+":"+metadata.EntityID] = metadata

	return nil
}

func (repo *finalizationBenchmarkMetadata) FindByEntity(_ context.Context, collection, id string) (*mongodb.Metadata, error) {
	return repo.records[collection+":"+id], nil
}

// BenchmarkTransactionCompletionService characterizes deterministic recovery decoding,
// projection, cloning, and metadata verification without database or network I/O.
func BenchmarkTransactionCompletionService(b *testing.B) {
	ctx, envelope := finalizationFixture(b)
	metadata := &finalizationBenchmarkMetadata{records: make(map[string]*mongodb.Metadata)}
	finalizer := NewTransactionCompletionService(&finalizationBenchmarkStore{}, metadata)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		result, err := finalizer.Complete(ctx, envelope)
		if err != nil {
			b.Fatal(err)
		}

		if result.Outcome.TransactionStatus != constant.APPROVED {
			b.Fatalf("unexpected recovery outcome: %s", result.Outcome.TransactionStatus)
		}
	}
}
