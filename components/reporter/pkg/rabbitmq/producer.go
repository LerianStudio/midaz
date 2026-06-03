// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"

	"github.com/LerianStudio/reporter/pkg/model"
)

// ProducerRepository provides an interface for Producer related to rabbitmq.
//
//go:generate mockgen --destination=producer.mock.go --package=rabbitmq --copyright_file=../../COPYRIGHT . ProducerRepository
type ProducerRepository interface {
	ProducerDefault(ctx context.Context, exchange, key string, message model.ReportMessage) (*string, error)
}
