// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
)

// ExpectedBackupStatusForCleanup returns the status that should be used when
// conditionally removing entries from backup_queue.
//
// CREATED transactions are promoted to APPROVED before async persistence, but
// the backup_queue entry keeps TransactionStatus=CREATED (written earlier in
// the HTTP flow). In this specific case, using APPROVED as expected status
// would skip cleanup and leak entries.
func ExpectedBackupStatusForCleanup(transactionStatus string, validate *mtransaction.Responses) string {
	if transactionStatus == constant.APPROVED && validate != nil && !validate.Pending {
		return constant.CREATED
	}

	return transactionStatus
}
