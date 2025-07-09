import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import { omitBy } from 'lodash'

export interface MidazFeeCreateTransactionDto {
  segmentId: string | null
  ledgerId: string
  transaction: {
    chartOfAccountsGroupName?: string
    route: string
    description?: string
    send: {
      asset: string
      value: string
      source: {
        from: {
          accountAlias: string
          amount: {
            asset: string
            value: string
          }
          route: 'DEBIT'
          description?: string
          chartOfAccounts?: string
          metadata?: Record<string, any>
        }[]
      }
      distribute: {
        to: {
          accountAlias: string
          amount: {
            asset: string
            value: string
          }
          route: 'CREDIT'
          description?: string
          chartOfAccounts?: string
          metadata?: Record<string, any>
        }[]
      }
    }
    metadata?: Record<string, any>
  }
}

export class MidazFeeTransactionMapper {
  private static cleanObject<T extends Record<string, any>>(
    objectToClean: T
  ): T {
    const cleaned: Record<string, any> = {}
    Object.entries(objectToClean).forEach(([key, value]) => {
      if (value === '' || value === undefined || value === null) {
        return
      }
      if (Array.isArray(value)) {
        const cleanedArray = value
          .map((item) =>
            typeof item === 'object' ? this.cleanObject(item) : item
          )
          .filter((v) =>
            typeof v === 'object' ? Object.keys(v).length > 0 : true
          )
        if (cleanedArray.length > 0) cleaned[key] = cleanedArray
      } else if (typeof value === 'object') {
        const cleanedObj = this.cleanObject(value)
        if (Object.keys(cleanedObj).length > 0) cleaned[key] = cleanedObj
      } else {
        cleaned[key] = value
      }
    })
    return cleaned as T
  }

  /**
   * Convert fee API response back to TransactionEntity format
   * @param feeResponse Response from the fees API
   */
  public static toEntity(feeResponse: any): TransactionEntity {
    return feeResponse
  }

  /**
   * Convert TransactionEntity into payload accepted by Fee Engine /v1/fees endpoint
   * @param transaction TransactionEntity domain object
   * @param ledgerId    Ledger ID where the transaction will be created
   * @param route       Business route identifier (defaults to "international_transfer")
   * @param segmentId   Optional segmentId (defaults to null)
   */
  public static toCreateDto(
    transaction: TransactionEntity,
    ledgerId: string,
    route: string = 'international_transfer',
    segmentId: string | null = null
  ): MidazFeeCreateTransactionDto {
    const transactionBody: any = {
      chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
      route,
      description: transaction.description,
      send: {
        asset: transaction.asset,
        value: transaction.amount,
        source: {
          from: transaction.source.map((source) =>
            omitBy(
              {
                accountAlias: source.accountAlias,
                amount: {
                  asset: transaction.asset,
                  value: source.amount
                },
                route: 'DEBIT' as const,
                description: source.description,
                chartOfAccounts: source.chartOfAccounts,
                metadata: source.metadata || undefined
              },
              (currentValue: any) =>
                currentValue === '' ||
                currentValue === undefined ||
                currentValue === null
            )
          )
        },
        distribute: {
          to: transaction.destination.map((destination) =>
            omitBy(
              {
                accountAlias: destination.accountAlias,
                amount: {
                  asset: transaction.asset,
                  value: destination.amount
                },
                route: 'CREDIT' as const,
                description: destination.description,
                chartOfAccounts: destination.chartOfAccounts,
                metadata: destination.metadata || undefined
              },
              (currentValue: any) =>
                currentValue === '' ||
                currentValue === undefined ||
                currentValue === null
            )
          )
        }
      }
    }

    if (transaction.metadata && Object.keys(transaction.metadata).length > 0) {
      transactionBody.metadata = transaction.metadata
    }

    // Clean empty props recursively
    const cleanedTransaction = this.cleanObject(transactionBody)

    return {
      segmentId,
      ledgerId,
      transaction: cleanedTransaction
    } as MidazFeeCreateTransactionDto
  }
}
