import { TransactionEntity } from '@/core/domain/entities/transaction-entity'

export interface MidazFeeCreateTransactionDto {
  segmentId: string | null
  ledgerId: string
  transaction: {
    route: string
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
          chartOfAccounts: string
          metadata: Record<string, any> | null
        }[]
      }
      distribute: {
        to: {
          accountAlias: string
          amount: {
            asset: string
            value: string
          }
          chartOfAccounts: string
          metadata: Record<string, any>
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
    const requiredFields = ['chartOfAccounts', 'accountAlias', 'asset', 'value']

    Object.entries(objectToClean).forEach(([key, value]) => {
      // Keep required fields even if they're empty strings
      if (requiredFields.includes(key)) {
        cleaned[key] = value
        return
      }

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
    route?: string,
    segmentId: string | null = null
  ): MidazFeeCreateTransactionDto {
    const transactionBody: any = {
      send: {
        asset: transaction.asset,
        value: transaction.amount,
        source: {
          from: transaction.source.map((source) => ({
            accountAlias: source.accountAlias,
            amount: {
              asset: transaction.asset,
              value: source.amount
            },
            chartOfAccounts: source.chartOfAccounts || 'assets'
          }))
        },
        distribute: {
          to: transaction.destination.map((destination) => ({
            accountAlias: destination.accountAlias,
            amount: {
              asset: transaction.asset,
              value: destination.amount
            },
            chartOfAccounts: destination.chartOfAccounts || 'assets'
          }))
        }
      }
    }

    const result = {
      ledgerId,
      transaction: transactionBody
    } as MidazFeeCreateTransactionDto

    const cleanedResult = this.cleanObject(result)

    return cleanedResult
  }
}
