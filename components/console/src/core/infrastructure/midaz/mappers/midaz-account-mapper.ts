import { AccountEntity } from '@/core/domain/entities/account-entity'
import {
  MidazAccountDto,
  MidazCreateAccountDto,
  MidazUpdateAccountDto
} from '../dto/midaz-account-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationMapper } from './midaz-pagination-mapper'

export class MidazAccountMapper {
  public static toCreateDto(account: AccountEntity): MidazCreateAccountDto {
    return {
      name: account.name,
      alias: account.alias,
      assetCode: account.assetCode,
      type: account.type,
      entityId: account.entityId,
      parentAccountId: account.parentAccountId,
      portfolioId: account.portfolioId,
      segmentId: account.segmentId,
      metadata: account.metadata
    }
  }

  public static toUpdateDto(
    account: Partial<AccountEntity>
  ): MidazUpdateAccountDto {
    return {
      name: account.name,
      alias: account.alias,
      assetCode: account.assetCode,
      type: account.type,
      entityId: account.entityId,
      parentAccountId: account.parentAccountId,
      portfolioId: account.portfolioId,
      segmentId: account.segmentId,
      metadata: account.metadata
    }
  }

  public static toEntity(account: MidazAccountDto): AccountEntity {
    return {
      id: account.id,
      organizationId: account.organizationId,
      ledgerId: account.ledgerId,
      name: account.name,
      type: account.type,
      alias: account.alias,
      assetCode: account.assetCode,
      entityId: account.entityId!,
      portfolioId: account.portfolioId!,
      segmentId: account.segmentId!,
      metadata: account.metadata ?? {},
      createdAt: account.createdAt,
      updatedAt: account.updatedAt,
      deletedAt: account.deletedAt
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazAccountDto>
  ): PaginationEntity<AccountEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazAccountMapper.toEntity
    )
  }
}
