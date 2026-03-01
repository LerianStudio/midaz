// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import "errors"

// Test sentinel errors used across query test files.
var (
	errAccountTypeRepoError          = errors.New("account type repo error")
	errDatabaseConnectionError       = errors.New("database connection error")
	errDatabaseConnectionTimeout     = errors.New("database connection timeout")
	errDatabaseError                 = errors.New("database error")
	errErrorMetadataNoFound          = errors.New("error metadata no found")
	errErrorNoMetadataFound          = errors.New("error no metadata found")
	errFailedToRetrieveAccounts      = errors.New("failed to retrieve accounts")
	errFailedToRetrieveAssets        = errors.New("failed to retrieve assets")
	errFailedToRetrieveLedgers       = errors.New("failed to retrieve ledgers")
	errFailedToRetrieveMetadata      = errors.New("failed to retrieve metadata")
	errMetadataRepoError             = errors.New("metadata repo error")
	errMetadataRetrievalError        = errors.New("metadata retrieval error")
	errMetadataServiceError          = errors.New("metadata service error")
	errMongodbConnectionFailed       = errors.New("mongodb connection failed")
	errNoAccountsFound               = errors.New("No accounts were found in the search. Please review the search criteria and try again.")                                 //nolint:revive,staticcheck
	errNoAssetsFound                 = errors.New("No assets were found in the search. Please review the search criteria and try again.")                                   //nolint:revive,staticcheck
	errNoLedgersFound                = errors.New("No ledgers were found in the search. Please review the search criteria and try again.")                                  //nolint:revive,staticcheck
	errAccountsNotRetrievedByAliases = errors.New("The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again.") //nolint:revive,staticcheck
)
