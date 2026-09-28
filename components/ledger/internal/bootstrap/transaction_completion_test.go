// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"

func completionError(_ command.TransactionCompletionResult, err error) error {
	return err
}
