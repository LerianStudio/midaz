import { z } from 'zod'
import { transaction } from '@/schema/transactions'

// Using intersection type to add specific fields to the record
const extendedAccountMetadata = z
  .intersection(
    z.record(z.string(), z.any()),
    z.object({
      route: z.string().optional() // Route can be specified per account
    })
  )
  .nullable()

const extendedTransactionMetadata = z
  .intersection(
    z.record(z.string(), z.any()),
    z.object({
      route: z.string().optional(), // Default route for the transaction
      segmentId: z.string().optional() // Segment ID can be passed via metadata
    })
  )
  .nullable()

export const transactionSourceFormSchema = z
  .array(
    z.object({
      accountAlias: transaction.source.accountAlias,
      value: transaction.value,
      description: transaction.description.optional(),
      chartOfAccounts: transaction.chartOfAccounts.optional(),
      metadata: extendedAccountMetadata
    })
  )
  .nonempty()
  .default([] as any)

export const transactionFormSchema = z.object({
  description: transaction.description.optional(),
  chartOfAccountsGroupName: transaction.chartOfAccounts.optional(),
  asset: transaction.asset,
  value: transaction.value,
  source: transactionSourceFormSchema,
  destination: transactionSourceFormSchema,
  metadata: extendedTransactionMetadata
})

export type TransactionSourceFormSchema = z.infer<
  typeof transactionSourceFormSchema
>

export type TransactionFormSchema = z.infer<typeof transactionFormSchema>

export const initialValues = {
  description: '',
  chartOfAccountsGroupName: '',
  value: '',
  asset: '',
  source: [],
  destination: [],
  metadata: {
    route: '', // Default transaction route
    segmentId: '' // Optional segment ID
  }
}

export const sourceInitialValues = {
  accountAlias: '',
  value: '',
  asset: '',
  description: '',
  chartOfAccounts: '',
  metadata: {
    route: '' // Optional account-specific route
  }
}
