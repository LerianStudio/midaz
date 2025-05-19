import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import {
  MidazCreateTransactionDto,
  MidazTransactionDto,
  MidazUpdateTransactionDto
} from '../dto/midaz-transaction-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationMapper } from './midaz-pagination-mapper'
import { transactions as lib } from '@lerian/lib-commons-js'
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
          ...lib.findScale(transaction.asset, transaction.value, 0),
          source: {
            from: transaction.source.map((source) =>
              omitBy(
                {
                  account: source.account,
                  amount: lib.findScale(transaction.asset, source.value, 0),
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
                  amount: lib.findScale(
                    transaction.asset,
                    destination.value,
                    0
                  ),
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
      value: lib.undoScale(transaction.amount, -transaction.amountScale),
      asset: transaction.assetCode,
      source: source.map((source) => ({
        account: source.accountId,
        accountAlias: source.accountAlias,
        asset: source.assetCode,
        value: lib.undoScale(source.amount.amount, -source.amount.scale),
        description: source.description,
        chartOfAccounts: source.chartOfAccounts,
        metadata: source.metadata ?? {}
      })),
      destination: destination.map((destination) => ({
        account: destination.accountId,
        accountAlias: destination.accountAlias,
        asset: destination.assetCode,
        value: lib.undoScale(
          destination.amount.amount,
          -destination.amount.scale
        ),
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
