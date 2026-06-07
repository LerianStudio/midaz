// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

const (
	ProcessingStatus        = "Processing"
	FinishedStatus          = "Finished"
	ErrorStatus             = "Error"
	PendingExtractionStatus = "PendingExtraction"
	// PartialStatus marks a report where at least one data section succeeded and at
	// least one failed. Per-section canonical error codes are carried in metadata.
	PartialStatus = "Partial"
)
