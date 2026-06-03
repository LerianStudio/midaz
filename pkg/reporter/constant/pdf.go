// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import "time"

// PDF Generation Constants
const (
	PDFMinValidSizeBytes     = 1000
	PDFLargeHTMLThreshold    = 500 * 1024 // 500 KB
	PDFBytesPerKB            = 1024
	PDFRenderSettleDelay     = 500 * time.Millisecond
	PDFPaperWidthInches      = 8.5
	PDFPaperHeightInches     = 11.0
	PDFMarginInches          = 0.5
	PDFFilePermissions       = 0o600
	PDFChromeMaxOldSpaceSize = "512"
)
