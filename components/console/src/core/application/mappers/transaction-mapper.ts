import {
  TransactionCreateEntity,
  TransactionEntity
} from '@/core/domain/entities/transaction-entity'
import {
  CreateTransactionDto,
  TransactionResponseDto
} from '../dto/transaction-dto'
import { isNumber } from 'lodash'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'
import { PaginationDto } from '../dto/pagination-dto'
export class TransactionMapper {
  static toDomain(transaction: CreateTransactionDto): TransactionCreateEntity {
    const transactionCreateEntity: TransactionCreateEntity = {
      ...(transaction.description && transaction.description.trim() !== ''
        ? { description: transaction.description }
        : {}),
      ...(transaction.chartOfAccountsGroupName &&
      transaction.chartOfAccountsGroupName.trim() !== ''
        ? { chartOfAccountsGroupName: transaction.chartOfAccountsGroupName }
        : {}),

      metadata:
        Object.keys(transaction.metadata).length !== 0
          ? transaction.metadata
          : null,

      send: {
        asset: transaction.asset,
        ...TransactionMapper.valueToAmount(transaction.value),
        source: {
          from: transaction.source.map((source) => ({
            account: source.account,
            amount: {
              asset: source.asset,
              ...TransactionMapper.valueToAmount(source.value)
            },
            ...(source.share
              ? {
                  share: {
                    percentage: source.share.percentage,
                    percentageOfPercentage: source.share.percentageOfPercentage
                  }
                }
              : {}),
            ...(source.chartOfAccounts && source.chartOfAccounts.trim() !== ''
              ? { chartOfAccounts: source.chartOfAccounts }
              : {}),

            ...(source.description && source.description.trim() !== ''
              ? { description: source.description }
              : {}),

            metadata:
              Object.keys(source.metadata).length !== 0 ? source.metadata : null
          }))
        },
        distribute: {
          to: transaction.destination.map((destination) => ({
            account: destination.account,
            amount: {
              asset: destination.asset,
              ...TransactionMapper.valueToAmount(destination.value)
            },
            ...(destination.share
              ? {
                  share: {
                    percentage: destination.share.percentage,
                    percentageOfPercentage:
                      destination.share.percentageOfPercentage
                  }
                }
              : {}),
            ...(destination.chartOfAccounts &&
            destination.chartOfAccounts.trim() !== ''
              ? { chartOfAccounts: destination.chartOfAccounts }
              : {}),
            ...(destination.description && destination.description.trim() !== ''
              ? { description: destination.description }
              : {}),
            metadata:
              Object.keys(destination.metadata).length !== 0
                ? destination.metadata
                : null
          }))
        }
      }
    }

    return transactionCreateEntity
  }

  static toDecimalValue(amount: number, amountScale: number): number {
    return amount / 10 ** amountScale
  }

  static toResponseDto(transaction: TransactionEntity): TransactionResponseDto {
    const transactionResponseDto: TransactionResponseDto = {
      id: transaction.id,
      description: transaction.description,
      template: transaction.template,
      status: {
        code: transaction.status.code,
        description: transaction.status.description ?? ''
      },
      amount: TransactionMapper.toDecimalValue(
        transaction.amount,
        transaction.amountScale
      ),
      amountScale: transaction.amountScale,
      assetCode: transaction.assetCode,
      chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
      source: !transaction.source ? [] : transaction.source,
      destination: !transaction.destination ? [] : transaction.destination,
      ledgerId: transaction.ledgerId,
      organizationId: transaction.organizationId,
      operations: !transaction.operations
        ? []
        : transaction.operations.map((operation) => ({
            id: operation.id,
            transactionId: operation.transactionId,
            description: operation.description,
            type: operation.type,
            assetCode: operation.assetCode,
            chartOfAccounts: operation.chartOfAccounts,
            amount: {
              amount: TransactionMapper.toDecimalValue(
                operation.amount.amount,
                operation.amount.scale
              ),
              scale: operation.amount.scale
            },
            balance: {
              available: TransactionMapper.toDecimalValue(
                operation.balance.available,
                operation.balance.scale
              ),
              onHold: TransactionMapper.toDecimalValue(
                operation.balance.onHold,
                operation.balance.scale
              ),
              scale: operation.balance.scale
            },
            balanceAfter: {
              available: TransactionMapper.toDecimalValue(
                operation.balanceAfter.available,
                operation.balanceAfter.scale
              ),
              onHold: TransactionMapper.toDecimalValue(
                operation.balanceAfter.onHold,
                operation.balanceAfter.scale
              ),
              scale: operation.balanceAfter.scale
            },
            status: {
              code: operation.status.code,
              description: operation.status.description ?? ''
            },
            accountId: operation.accountId,
            accountAlias: operation.accountAlias,
            organizationId: operation.organizationId,
            ledgerId: operation.ledgerId,
            portfolioId: operation.portfolioId,
            createdAt: operation.createdAt,
            updatedAt: operation.updatedAt,
            deletedAt: operation.deletedAt,
            metadata: operation.metadata ?? {}
          })),
      metadata: transaction.metadata ?? {},
      createdAt: transaction.createdAt,
      updatedAt: transaction.updatedAt,
      deletedAt: transaction.deletedAt
    }

    return transactionResponseDto
  }

  static valueToAmount(value: number) {
    if (!isNumber(value)) {
      throw new Error(
        `TransactionMapper.valueToAmount: value ${value} is not a number`
      )
    }

    let resultValue = value
    let scale = 0

    while (resultValue % 1 !== 0) {
      resultValue *= 10
      scale++
    }

    return { value: resultValue, scale }
  }

  static transactionMapperUpdate(
    description?: string,
    metadata?: Record<string, unknown>
  ) {
    return {
      description,
      metadata: metadata && Object.keys(metadata).length !== 0 ? metadata : {}
    }
  }

  static toPaginatedResponseDto(
    paginationEntity: PaginationEntity<TransactionEntity>
  ): PaginationDto<TransactionResponseDto> {
    return PaginationMapper.toResponseDto(
      paginationEntity,
      TransactionMapper.toResponseDto as (
        transaction: TransactionEntity
      ) => TransactionResponseDto
    )
  }
}
