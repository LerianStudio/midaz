import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import {
  MidazCreateTransactionDto,
  MidazTransactionDto,
  MidazUpdateTransactionDto
} from '../dto/midaz-transaction-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationMapper } from './midaz-pagination-mapper'
import { omitBy } from 'lodash'

export class MidazTransactionMapper {
  public static toCreateDto(
    transaction: TransactionEntity
  ): MidazCreateTransactionDto {
    return omitBy(
      {
        description: transaction.description,
        chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
        send: {
          asset: transaction.asset,
          value: transaction.amount.value,
          scale: transaction.amount.scale,
          source: {
            from: transaction.source.map((source) =>
              omitBy(
                {
                  account: source.account,
                  amount: {
                    value: source.amount.value,
                    scale: source.amount.scale,
                    asset: transaction.asset
                  },
                  description: source.description,
                  chartOfAccounts: source.chartOfAccounts,
                  metadata: source.metadata
                },
                (v: any) => v === ''
              )
            )
          },
          distribute: {
            to: transaction.destination.map((destination) =>
              omitBy(
                {
                  account: destination.account,
                  amount: {
                    value: destination.amount.value,
                    scale: destination.amount.scale,
                    asset: transaction.asset
                  },
                  description: destination.description,
                  chartOfAccounts: destination.chartOfAccounts,
                  metadata: destination.metadata
                },
                (v: any) => v === ''
              )
            )
          }
        },
        metadata: transaction.metadata
      },
      (v) => v === ''
    ) as MidazCreateTransactionDto
  }

  public static toUpdateDto(
    transaction: Partial<TransactionEntity>
  ): MidazUpdateTransactionDto {
    return {
      description: transaction.description,
      metadata: transaction.metadata
    }
  }

  public static toEntity(transaction: MidazTransactionDto): TransactionEntity {
    const source =
      transaction.operations?.filter((t) => t.type === 'DEBIT') ?? []
    const destination =
      transaction.operations?.filter((t) => t.type === 'CREDIT') ?? []

    return {
      id: transaction.id,
      ledgerId: transaction.ledgerId,
      organizationId: transaction.organizationId,
      description: transaction.description,
      chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
      status: transaction.status,
      amount: {
        value: transaction.amount,
        scale: transaction.amountScale
      },
      asset: transaction.assetCode,
      source: source.map((source) => ({
        account: source.accountId,
        accountAlias: source.accountAlias,
        asset: source.assetCode,
        amount: {
          value: source.amount.amount,
          scale: source.amount.scale
        },
        description: source.description,
        chartOfAccounts: source.chartOfAccounts,
        metadata: source.metadata ?? {}
      })),
      destination: destination.map((destination) => ({
        account: destination.accountId,
        accountAlias: destination.accountAlias,
        asset: destination.assetCode,
        amount: {
          value: destination.amount.amount,
          scale: destination.amount.scale
        },
        description: destination.description,
        chartOfAccounts: destination.chartOfAccounts,
        metadata: destination.metadata ?? {}
      })),
      metadata: transaction.metadata ?? {},
      createdAt: transaction.createdAt,
      updatedAt: transaction.updatedAt
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazTransactionDto>
  ): PaginationEntity<TransactionEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazTransactionMapper.toEntity
    )
  }
}
