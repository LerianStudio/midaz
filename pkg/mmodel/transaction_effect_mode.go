// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"fmt"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const TransactionEffectModeVersion = 1

type TransactionEffectMode string

const (
	TransactionEffectBalanceMutation TransactionEffectMode = "BALANCE_MUTATION"
	TransactionEffectAnnotationOnly  TransactionEffectMode = "ANNOTATION_ONLY"
)

// ResolveTransactionEffectMode validates the explicit versioned effect
// discriminator written by new producers. Payloads without both fields are
// legacy: NOTED is the only legacy annotation identity; every other status is
// treated as a balance mutation and must still pass the complete movement
// proof. A partially upgraded or unknown discriminator fails closed.
func ResolveTransactionEffectMode(queue *TransactionRedisQueue) (TransactionEffectMode, error) {
	if queue == nil {
		return "", fmt.Errorf("transaction effect envelope is required")
	}
	if queue.EffectModeVersion == 0 && queue.EffectMode == "" {
		if err := validateOperationTypeOverride(queue.TransactionStatus, queue.OperationTypeOverride); err != nil {
			return "", err
		}
		if queue.TransactionStatus == constant.NOTED {
			return TransactionEffectAnnotationOnly, nil
		}

		return TransactionEffectBalanceMutation, nil
	}
	if queue.EffectModeVersion != TransactionEffectModeVersion {
		return "", fmt.Errorf("unsupported transaction effect mode version %d", queue.EffectModeVersion)
	}
	switch queue.EffectMode {
	case TransactionEffectBalanceMutation:
		if queue.TransactionStatus == constant.NOTED {
			return "", fmt.Errorf("NOTED transaction cannot declare a balance mutation")
		}
		if err := validateOperationTypeOverride(queue.TransactionStatus, queue.OperationTypeOverride); err != nil {
			return "", err
		}
	case TransactionEffectAnnotationOnly:
		if queue.TransactionStatus != constant.NOTED {
			return "", fmt.Errorf("annotation-only transaction must be NOTED")
		}
		if queue.OperationTypeOverride != "" {
			return "", fmt.Errorf("annotation-only transaction cannot override its operation type")
		}
	default:
		return "", fmt.Errorf("unsupported transaction effect mode %q", queue.EffectMode)
	}

	return queue.EffectMode, nil
}

func validateOperationTypeOverride(transactionStatus, operationTypeOverride string) error {
	if operationTypeOverride == "" {
		return nil
	}
	if transactionStatus == constant.NOTED ||
		(operationTypeOverride != constant.BLOCK && operationTypeOverride != constant.UNBLOCK) {
		return fmt.Errorf("unsupported transaction operation type override %q", operationTypeOverride)
	}

	return nil
}
