// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import "time"

// MongoDB collection names.
const (
	MongoCollectionDeadline          = "deadline"
	MongoCollectionReport            = "report"
	MongoCollectionTemplate          = "template"
	MongoCollectionExtractionMapping = "extraction_mapping"
)

// MongoDB sampling and collection size thresholds for schema discovery.
const (
	// MongoLargeCollectionThreshold is the document count above which sampling is used instead of full aggregation.
	MongoLargeCollectionThreshold = 10000

	// MongoSmallCollectionLimit is the max documents to process for small collections during aggregation.
	MongoSmallCollectionLimit = 1000

	// MongoDefaultSampleSize is the sample size for collections with 1,001-10,000 documents.
	MongoDefaultSampleSize = 1000

	// MongoMediumSampleSize is the sample size for collections with 10,001-100,000 documents.
	MongoMediumSampleSize = 2000

	// MongoLargeSampleSize is the sample size for collections with 100,001-1,000,000 documents.
	MongoLargeSampleSize = 5000

	// MongoMaxSampleSize is the sample size for collections with over 1,000,000 documents.
	MongoMaxSampleSize = 10000

	// MongoSmallCollectionDocLimit is the threshold for small collections (up to 1,000 documents).
	MongoSmallCollectionDocLimit = 1000

	// MongoMediumCollectionDocLimit is the threshold for medium collections (up to 10,000 documents).
	MongoMediumCollectionDocLimit = 10000

	// MongoLargeCollectionDocLimit is the threshold for large collections (up to 100,000 documents).
	MongoLargeCollectionDocLimit = 100000

	// MongoVeryLargeCollectionDocLimit is the threshold for very large collections (up to 1,000,000 documents).
	MongoVeryLargeCollectionDocLimit = 1000000

	// MongoMinDocsForSampling is the minimum document count before random sampling is used.
	MongoMinDocsForSampling = 50

	// MongoMaxDocsForSmallSample is the document count above which random sampling is preferred over sequential scan.
	MongoMaxDocsForSmallSample = 1000

	// MongoUUIDByteLength is the byte length of a UUID value in MongoDB Binary format.
	MongoUUIDByteLength = 16

	// MongoTextSearchWeight is the default weight for text search indexes.
	MongoTextSearchWeight = 10
)

// MongoDB index operation timeouts.
const (
	// MongoIndexCreateTimeout is the maximum time allowed for creating indexes.
	MongoIndexCreateTimeout = 60 * time.Second

	// MongoIndexDropTimeout is the maximum time allowed for dropping indexes.
	MongoIndexDropTimeout = 30 * time.Second
)

// BSON type priority constants for schema inference.
// Higher values indicate more specific types that should be preferred during type resolution.
const (
	BSONPriorityObjectID  = 10
	BSONPriorityDate      = 9
	BSONPriorityTimestamp = 8
	BSONPriorityDecimal   = 7
	BSONPriorityBinData   = 6
	BSONPriorityRegex     = 5
	BSONPriorityMinMaxKey = 4
	BSONPriorityNumber    = 3
	BSONPriorityDefault   = 2
	BSONPriorityUnknown   = 1
)

// MongoDB pool configuration constant.
const (
	// MongoMaxPoolSizeExternal is the max connection pool size for external data source connections.
	MongoMaxPoolSizeExternal = 100

	// MongoMaxPoolSizeUpperBound is the maximum allowed value for MongoDB connection pool size configuration.
	MongoMaxPoolSizeUpperBound = 10000

	// MongoDefaultMaxPoolSize is the default connection pool size when MONGO_MAX_POOL_SIZE is not set or zero.
	MongoDefaultMaxPoolSize = 100
)

// Time-related constants.
const (
	// HoursPerDay is the number of hours in a single day, used for date range calculations.
	HoursPerDay = 24
)
