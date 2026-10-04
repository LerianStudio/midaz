// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package backfill

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
)

// metadataDedupeTimeout bounds one collection's scan, folds and unique index build, which the Mongo
// client's 30s default operation timeout would cut short on a large collection.
const metadataDedupeTimeout = time.Hour

// ErrMetadataRaced names collections whose entity_id index stayed as it was because a document was
// written during the dedupe; a re-run folds it.
var ErrMetadataRaced = errors.New("metadata written during the dedupe, re-run the backfill runner for")

// DedupeMetadata folds duplicate metadata documents and makes entity_id unique in each entity's
// collection of db, and returns how many entities it folded. Raced collections do not stop the
// others: they are named together in one ErrMetadataRaced.
func DedupeMetadata(ctx context.Context, db *mongo.Database, entities []string) (int, error) {
	folded := 0

	var raced []string

	for _, entity := range entities {
		coll := db.Collection(strings.ToLower(entity))

		collCtx, cancel := context.WithTimeout(ctx, metadataDedupeTimeout)
		collFolded, collRaced, err := mongodb.DedupeEntityIDAndMakeUnique(collCtx, coll)

		cancel()

		folded += collFolded
		name := db.Name() + "." + coll.Name()

		if err != nil {
			return folded, fmt.Errorf("dedupe %s metadata: %w", name, err)
		}

		if collRaced {
			raced = append(raced, name)
		}
	}

	if len(raced) > 0 {
		return folded, fmt.Errorf("%w %s", ErrMetadataRaced, strings.Join(raced, ", "))
	}

	return folded, nil
}
