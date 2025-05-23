import { ledger } from '@/schema/ledger'
import { template } from 'lodash'
import { destination } from 'pino'
import { z } from 'zod'

export const formSchema = z.object({
  id: z.string().optional(),
  description: z.string().optional(),
  template: z.string().optional(),
  status: z
    .object({
      code: z.string(),
      description: z.string().optional()
    })
    .optional(),
  amount: z.number().optional(),
  amountScale: z.number().optional(),
  assetCode: z.string().optional(),
  chartOfAccountsGroupName: z.string().optional(),
  source: z.array(z.string()).optional(),
  destination: z.array(z.string()).optional(),
  ledgerId: z.string().optional(),
  organizationId: z.string().optional(),
  operations: z.array(
    z.object({
      id: z.string().optional(),
      transactionId: z.string().optional(),
      description: z.string().optional(),
      type: z.string().optional(),
      assetCode: z.string().optional(),
      chartOfAccounts: z.string().optional(),
      amount: z
        .object({
          amount: z.number(),
          scale: z.number()
        })
        .optional(),
      balance: z
        .object({
          available: z.number(),
          onHold: z.number(),
          scale: z.number()
        })
        .optional(),
      balanceAfter: z
        .object({
          available: z.number(),
          onHold: z.number(),
          scale: z.number()
        })
        .optional(),
      status: z
        .object({
          code: z.string(),
          description: z.string().optional()
        })
        .optional(),
      accountId: z.string().optional(),
      accountAlias: z.string().optional(),
      portfolioId: z.string().optional(),
      organizationId: z.string().optional(),
      ledgerId: z.string().optional(),
      createdAt: z.string().optional(),
      updatedAt: z.string().optional(),
      deletedAt: z.string().optional(),
      metadata: z.record(z.string(), z.unknown()).optional()
    })
  ),
  metadata: z.record(z.string(), z.unknown()).optional(),
  createdAt: z.string().optional(),
  updatedAt: z.string().optional(),
  deletedAt: z.string().optional()
})
