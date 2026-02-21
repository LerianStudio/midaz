// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type migrateAccountShardInput struct {
	Alias       string `json:"alias" validate:"required,max=100,invalidaliascharacters,prohibitedexternalaccountprefix"`
	TargetShard *int   `json:"targetShard" validate:"required,min=0"`
}

func (handler *TransactionHandler) PauseShardRebalance(c *fiber.Ctx) error {
	if handler == nil || handler.Command == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "transaction command service is unavailable")
	}

	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.pause_shard_rebalance")
	defer span.End()

	if err := handler.Command.SetShardRebalancePaused(ctx, true); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to pause shard rebalancer", err)
		logger.Errorf("Failed to pause shard rebalancer actor=%s request_id=%s path=%s: %v", shardingActor(c), c.GetRespHeader("X-Request-Id"), c.Path(), err)

		return http.WithError(c, err)
	}

	logger.Infof("Sharding control action=rebalance_pause outcome=success actor=%s request_id=%s path=%s", shardingActor(c), c.GetRespHeader("X-Request-Id"), c.Path())

	return http.OK(c, fiber.Map{"paused": true})
}

func (handler *TransactionHandler) ResumeShardRebalance(c *fiber.Ctx) error {
	if handler == nil || handler.Command == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "transaction command service is unavailable")
	}

	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.resume_shard_rebalance")
	defer span.End()

	if err := handler.Command.SetShardRebalancePaused(ctx, false); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to resume shard rebalancer", err)
		logger.Errorf("Failed to resume shard rebalancer actor=%s request_id=%s path=%s: %v", shardingActor(c), c.GetRespHeader("X-Request-Id"), c.Path(), err)

		return http.WithError(c, err)
	}

	logger.Infof("Sharding control action=rebalance_resume outcome=success actor=%s request_id=%s path=%s", shardingActor(c), c.GetRespHeader("X-Request-Id"), c.Path())

	return http.OK(c, fiber.Map{"paused": false})
}

func (handler *TransactionHandler) GetShardRebalanceStatus(c *fiber.Ctx) error {
	if handler == nil || handler.Command == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "transaction command service is unavailable")
	}

	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_shard_rebalance_status")
	defer span.End()

	status, err := handler.Command.GetShardRebalanceStatus(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get shard rebalancer status", err)
		logger.Errorf("Failed to get shard rebalancer status actor=%s request_id=%s path=%s: %v", shardingActor(c), c.GetRespHeader("X-Request-Id"), c.Path(), err)

		return http.WithError(c, err)
	}

	logger.Infof("Sharding control action=rebalance_status outcome=success actor=%s request_id=%s path=%s", shardingActor(c), c.GetRespHeader("X-Request-Id"), c.Path())

	return http.OK(c, status)
}

func (handler *TransactionHandler) MigrateAccountShard(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.migrate_account_shard")
	defer span.End()

	organizationID, ok := c.Locals("organization_id").(uuid.UUID)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "organization_id is required")
	}

	ledgerID, ok := c.Locals("ledger_id").(uuid.UUID)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "ledger_id is required")
	}

	input, ok := p.(*migrateAccountShardInput)
	if !ok || input == nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid migration payload")
	}

	if input.TargetShard == nil {
		return fiber.NewError(fiber.StatusBadRequest, "targetShard is required")
	}

	if handler == nil || handler.Command == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "transaction command service is unavailable")
	}

	result, err := handler.Command.MigrateAccountShard(ctx, organizationID, ledgerID, input.Alias, *input.TargetShard)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to migrate account shard", err)
		logger.Errorf(
			"Failed to migrate account shard actor=%s request_id=%s alias=%s target_shard=%d: %v",
			shardingActor(c),
			c.GetRespHeader("X-Request-Id"),
			input.Alias,
			*input.TargetShard,
			err,
		)

		return http.WithError(c, err)
	}

	logger.Infof(
		"Sharding control action=migrate_account outcome=success actor=%s request_id=%s organization_id=%s ledger_id=%s alias=%s target_shard=%d path=%s",
		shardingActor(c),
		c.GetRespHeader("X-Request-Id"),
		organizationID.String(),
		ledgerID.String(),
		input.Alias,
		*input.TargetShard,
		c.Path(),
	)

	return http.OK(c, result)
}

func shardingActor(c *fiber.Ctx) string {
	if c == nil {
		return "unknown"
	}

	keys := []string{"subject", "sub", "user_id", "client_id"}
	for _, key := range keys {
		if value := c.Locals(key); value != nil {
			if text := fmt.Sprintf("%v", value); text != "" {
				return text
			}
		}
	}

	return "unknown"
}
