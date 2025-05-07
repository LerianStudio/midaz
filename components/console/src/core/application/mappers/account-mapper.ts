import { AccountEntity } from '@/core/domain/entities/account-entity'
import {
  AccountResponseDto,
  CreateAccountDto,
  UpdateAccountDto
} from '../dto/account-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'
import { BalanceEntity } from '@/core/domain/entities/balance-entity'

export class AccountMapper {
  public static toDto(
    account: AccountEntity & Partial<BalanceEntity>
  ): AccountResponseDto {
    return {
      id: account.id!,
      entityId: account.entityId!,
      ledgerId: account.ledgerId!,
      organizationId: account.organizationId!,
      name: account.name,
      type: account.type,
      metadata: account.metadata ?? {},
      createdAt: account.createdAt!,
      updatedAt: account.updatedAt!,
      deletedAt: account.deletedAt ?? null,
      alias: account.alias!,
      assetCode: account.assetCode,
      parentAccountId: account.parentAccountId!,
      segmentId: account.segmentId!,
      portfolioId: account.portfolioId,
      allowSending: account.allowSending,
      allowReceiving: account.allowReceiving
    }
  }

  public static toDomain(
    dto: CreateAccountDto | UpdateAccountDto
  ): AccountEntity {
    return {
      entityId: dto.entityId!,
      alias: dto.alias!,
      name: dto.name!,
      type: dto.type!,
      assetCode: dto.assetCode!,
      parentAccountId: dto.parentAccountId!,
      segmentId: dto.segmentId,
      portfolioId: dto.portfolioId!,
      metadata: dto.metadata ?? {}
    }
  }

  static toPaginationResponseDto(
    result: PaginationEntity<AccountEntity>
  ): PaginationEntity<AccountResponseDto> {
    return PaginationMapper.toResponseDto(result, AccountMapper.toDto)
  }
}
