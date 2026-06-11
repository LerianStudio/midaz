// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package dupkey classifies MongoDB duplicate-key errors by the name of the
// unique index that was violated, so CRM adapters can map specific index
// collisions to typed business errors (E6) instead of matching raw error text.
package dupkey

import (
	"errors"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// duplicateKeyCodes are the server error codes that denote a duplicate-key
// violation (mirrors mongo.IsDuplicateKeyError, minus the capped-collection and
// mongos message-coded variants which carry no index name to classify on).
var duplicateKeyCodes = map[int]struct{}{
	11000: {}, // duplicate key error
	11001: {}, // duplicate key error on update
}

// indexNamePattern extracts the index name from a duplicate-key error message.
//
// SANCTIONED STRING-TOUCH (E6 carve-out): the violated index name is not exposed
// as a structured field on mongo.WriteError — neither Details nor Raw carries it
// reliably for E11000 errors — so the name is available ONLY in the message text.
// This single regexp is the one place in the codebase permitted to parse a driver
// error string for classification; TestClassifyDuplicateKey pins the message
// format (mongo-driver v2.6.0) so a driver upgrade that rewords it fails loudly
// here instead of silently misclassifying.
//
// Server message shape: "E11000 duplicate key error collection: db.coll index: <name> dup key: {...}".
var indexNamePattern = regexp.MustCompile(`index:\s+(\S+)`)

// ClassifyDuplicateKey reports the name of the unique index violated by a
// duplicate-key error. It returns ok=false for any error that is not a
// duplicate-key write error or whose index name cannot be parsed — in which case
// the caller must let the raw error flow on (preserving, e.g., the idempotent
// re-fetch path that depends on mongo.IsDuplicateKeyError still matching a raw
// _id collision).
func ClassifyDuplicateKey(err error) (indexName string, ok bool) {
	if err == nil {
		return "", false
	}

	var writeException mongo.WriteException
	if errors.As(err, &writeException) {
		for _, writeErr := range writeException.WriteErrors {
			if name, found := indexNameFromWriteError(writeErr); found {
				return name, true
			}
		}
	}

	var bulkException mongo.BulkWriteException
	if errors.As(err, &bulkException) {
		for _, writeErr := range bulkException.WriteErrors {
			if name, found := indexNameFromWriteError(writeErr.WriteError); found {
				return name, true
			}
		}
	}

	return "", false
}

func indexNameFromWriteError(writeErr mongo.WriteError) (string, bool) {
	if _, isDuplicate := duplicateKeyCodes[writeErr.Code]; !isDuplicate {
		return "", false
	}

	matches := indexNamePattern.FindStringSubmatch(writeErr.Message)
	if len(matches) < 2 {
		return "", false
	}

	return matches[1], true
}
